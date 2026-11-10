/*
Copyright the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package itemoutcome

import (
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/velero-io/velero/pkg/plugin/velero"
)

// Recorder accumulates per-item outcomes for one backup run.
type Recorder struct {
	mu      sync.Mutex
	rows    map[velero.ResourceIdentifier]*itemOutcomeState
	aliases map[velero.ResourceIdentifier]velero.ResourceIdentifier
}

type itemOutcomeState struct {
	outcome         Outcome
	messages        []string
	skippedByVolume bool
	volumeAttempts  []VolumeAttempt
	reportedBy      map[Key]ReportedByRef
}

// maxAliasDepth bounds how far a rename chain is followed, so a pathological
// cycle cannot make alias resolution loop forever.
const maxAliasDepth = 32

// NewRecorder returns an initialized, empty backup Recorder.
func NewRecorder() *Recorder {
	return &Recorder{
		rows:    make(map[velero.ResourceIdentifier]*itemOutcomeState),
		aliases: make(map[velero.ResourceIdentifier]velero.ResourceIdentifier),
	}
}

// rowFor returns the row for id, minting it at OutcomeSucceeded if it has none.
// Callers that are about to record a verdict use this; callers that are only
// learning that the item exists use edgeRowFor.
func (r *Recorder) rowFor(id velero.ResourceIdentifier) *itemOutcomeState {
	return r.ensureRow(id, OutcomeSucceeded)
}

// edgeRowFor returns the row for id, minting it at OutcomeNamed if it has none.
// A row created by an ownership edge records that something named the item, not
// that anything processed it, so it must not start at OutcomeSucceeded.
func (r *Recorder) edgeRowFor(id velero.ResourceIdentifier) *itemOutcomeState {
	return r.ensureRow(id, OutcomeNamed)
}

func (r *Recorder) ensureRow(id velero.ResourceIdentifier, mint Outcome) *itemOutcomeState {
	id = r.resolveAlias(id)
	row, ok := r.rows[id]
	if !ok {
		row = &itemOutcomeState{outcome: mint}
		r.rows[id] = row
	}
	return row
}

// upgrade moves a row's outcome to the given outcome if it is more
// significant than the row's current outcome, per the precedence order
// Failed > Skipped > Warning > BackedUp > Named.
func (row *itemOutcomeState) upgrade(outcome Outcome) {
	if precedence[outcome] > precedence[row.outcome] {
		row.outcome = outcome
	}
}

// Observe records that an item began processing, promoting a row that an
// ownership edge minted at OutcomeNamed to OutcomeSucceeded and minting one at
// OutcomeSucceeded if there is no existing row.
func (r *Recorder) Observe(id velero.ResourceIdentifier) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// rowFor mints an absent row at BackedUp; the upgrade then promotes a row an
	// ownership edge already minted at Named. It is a no-op for every other
	// outcome, since upgrade only ever raises a severity.
	r.rowFor(id).upgrade(OutcomeSucceeded)
}

// Warn upgrades an item's outcome to OutcomeWarning, unless it is already
// OutcomeFailed, and appends msg to its messages.
func (r *Recorder) Warn(id velero.ResourceIdentifier, msg string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rowFor(id)
	row.upgrade(OutcomeWarning)
	row.messages = append(row.messages, msg)
}

// Fail upgrades an item's outcome to OutcomeFailed and appends msg to its
// messages.
func (r *Recorder) Fail(id velero.ResourceIdentifier, msg string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rowFor(id)
	row.upgrade(OutcomeFailed)
	row.messages = append(row.messages, msg)
}

// Skip upgrades an item's outcome to OutcomeSkipped and appends reason to its
// messages. The skip is item-level: the item itself was excluded or skipped, so
// a captured volume must not turn it into a success at finalize.
func (r *Recorder) Skip(id velero.ResourceIdentifier, reason string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rowFor(id)
	row.upgrade(OutcomeSkipped)
	row.skippedByVolume = false
	row.messages = append(row.messages, reason)
}

// SkipVolume records a skip from a volume path: one method declined the volume,
// but another method may still capture it. The finalize fold can therefore
// rescue the row when the volume verdict says the disk was captured.
func (r *Recorder) SkipVolume(id velero.ResourceIdentifier, reason string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rowFor(id)
	row.upgrade(OutcomeSkipped)
	row.skippedByVolume = true
	row.messages = append(row.messages, reason)
}

// RecordVolumeAttempt keeps a capture-time attempt on the row. The finalize
// fold merges it into Volume.Attempts, including a skip that volume info
// removed when another method claimed the volume.
func (r *Recorder) RecordVolumeAttempt(id velero.ResourceIdentifier, attempt VolumeAttempt) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rowFor(id)
	row.volumeAttempts = append(row.volumeAttempts, attempt)
}

// SetReportedBy records that a backup item action reported item as an additional
// item while backing up reporter. A plugin recording the item is a stronger
// claim than the item's own metadata.ownerReferences, so this overrides any
// owner-reference report set by SetOwnerReportedBy.
func (r *Recorder) SetReportedBy(item, reporter velero.ResourceIdentifier) {
	r.setReportedBy(item, reporter, ReportSourceAdditionalItem)
}

// SetOwnerReportedBy records that the item names the owner in its metadata.ownerReferences.
// It is a weaker claim than SetReportedBy and never displaces a report a backup
// item action has already named.
func (r *Recorder) SetOwnerReportedBy(item, owner velero.ResourceIdentifier) {
	r.setReportedBy(item, owner, ReportSourceOwnerReference)
}

// setReportedBy installs the item's reported-by entry if the claim is at least as strong as the
// one already recorded. Comparing on source rather than arrival order keeps the
// tree stable when two item blocks process an item and its reporter concurrently:
// whichever finishes first, the surviving edge is the same.
//
// The row is minted at OutcomeNamed rather than OutcomeSucceeded. An edge is set
// before the item is fetched or backed up, so at this point nothing has
// confirmed the item is in the backup; when the item is processed, Observe
// promotes the row. A row still reading Named in a finished artifact is one a
// plugin named and nothing resolved.
func (r *Recorder) setReportedBy(item, reporter velero.ResourceIdentifier, source ReportSource) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	item = r.resolveAlias(item)
	reporter = r.resolveAlias(reporter)
	if item == reporter {
		// A self-edge would make the item its own root and run every tree walk to
		// its depth limit. Refuse it regardless of which source produced it.
		return
	}

	row := r.edgeRowFor(item)
	if row.reportedBy == nil {
		row.reportedBy = make(map[Key]ReportedByRef)
	}
	key := Key{
		GroupResource: reporter.GroupResource.String(),
		Namespace:     reporter.Namespace,
		Name:          reporter.Name,
	}
	if existing, ok := row.reportedBy[key]; ok && reportSourcePrecedence[source] <= reportSourcePrecedence[existing.Source] {
		return
	}
	row.reportedBy[key] = *newReportedByRef(&reporter, source)
}

// rekey moves a row from old to new, merging into an existing row at the
// destination. It backs Item.Rekey, which follows a plugin-driven rename.
//
// The old identity is also recorded as an alias. A reported-by edge, a later write,
// or an operation result can still name the pre-rename identity; resolving it
// here is what keeps the row and its inbound references from drifting apart.
func (r *Recorder) rekey(old, new velero.ResourceIdentifier) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	old = r.resolveAlias(old)
	new = r.resolveAlias(new)
	if old == new {
		return
	}

	if r.aliases == nil {
		r.aliases = make(map[velero.ResourceIdentifier]velero.ResourceIdentifier)
	}
	r.aliases[old] = new

	row, ok := r.rows[old]
	if !ok {
		return
	}
	delete(r.rows, old)

	if existing, ok := r.rows[new]; ok {
		mergeRow(existing, row)
		r.dropSelfEdge(existing, new)
		return
	}
	r.dropSelfEdge(row, new)
	r.rows[new] = row
}

// dropSelfEdge removes reported-by edges that a rename has turned into
// self-edges. A reporter may still be recorded under an identity that aliases to id.
func (r *Recorder) dropSelfEdge(row *itemOutcomeState, id velero.ResourceIdentifier) {
	if row == nil || len(row.reportedBy) == 0 {
		return
	}
	idKey := Key{GroupResource: id.GroupResource.String(), Namespace: id.Namespace, Name: id.Name}
	for key, reporter := range row.reportedBy {
		if key == idKey || r.resolveAlias(reportedByIdentifier(reporter)) == id {
			delete(row.reportedBy, key)
		}
	}
}

// resolveAlias follows a rename chain to the final identity. The depth bound
// keeps a pathological cycle from looping; a cycle is not a supported shape.
func (r *Recorder) resolveAlias(id velero.ResourceIdentifier) velero.ResourceIdentifier {
	for depth := 0; depth < maxAliasDepth; depth++ {
		next, ok := r.aliases[id]
		if !ok || next == id {
			return id
		}
		id = next
	}
	return id
}

// mergeRow folds src into dst, keeping the more significant outcome and report
// source and appending messages. It is only used when a rename lands on a row
// that already exists.
func mergeRow(dst, src *itemOutcomeState) {
	mergeSkipProvenance(dst, src)
	if precedence[src.outcome] > precedence[dst.outcome] {
		dst.outcome = src.outcome
	}
	dst.messages = mergeMessages(dst.messages, src.messages)
	dst.volumeAttempts = dedupeVolumeAttempts(append(dst.volumeAttempts, src.volumeAttempts...))
	mergeReportedByMaps(dst, src)
}

func mergeReportedByMaps(dst, src *itemOutcomeState) {
	if len(src.reportedBy) == 0 {
		return
	}
	if dst.reportedBy == nil {
		dst.reportedBy = make(map[Key]ReportedByRef, len(src.reportedBy))
	}
	for key, reporter := range src.reportedBy {
		if existing, ok := dst.reportedBy[key]; ok && reportSourcePrecedence[reporter.Source] <= reportSourcePrecedence[existing.Source] {
			continue
		}
		dst.reportedBy[key] = reporter
	}
}

// mergeSkipProvenance folds src's skip provenance into dst. When both rows are
// skipped, an item-level skip wins, so a volume marker survives only if both
// rows carry it. When only one row is skipped, its marker is authoritative.
func mergeSkipProvenance(dst, src *itemOutcomeState) {
	dstSkipped := dst.outcome == OutcomeSkipped
	srcSkipped := src.outcome == OutcomeSkipped
	switch {
	case dstSkipped && srcSkipped:
		dst.skippedByVolume = dst.skippedByVolume && src.skippedByVolume
	case srcSkipped:
		dst.skippedByVolume = src.skippedByVolume
	case dstSkipped:
		// Keep dst's marker.
	default:
		dst.skippedByVolume = dst.skippedByVolume || src.skippedByVolume
	}
}

// Result returns the recorded outcomes as a sorted, deterministic
// ItemOutcomes artifact.
func (r *Recorder) Result() *ItemOutcomes {
	result := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Kind:          KindBackup,
		Items:         []ItemOutcome{},
	}

	if r == nil {
		return result
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, row := range r.rows {
		item := ItemOutcome{
			GroupResource:   id.GroupResource.String(),
			Namespace:       id.Namespace,
			Name:            id.Name,
			Outcome:         row.outcome,
			Messages:        capMessages(row.messages),
			SkippedByVolume: row.skippedByVolume,
			VolumeAttempts:  dedupeVolumeAttempts(row.volumeAttempts),
		}
		if len(row.reportedBy) > 0 {
			item.ReportedBy = r.reportedByRefs(row.reportedBy)
		}
		result.Items = append(result.Items, item)
	}

	sortItems(result.Items)

	return result
}

func (r *Recorder) reportedByRefs(reportedBy map[Key]ReportedByRef) []ReportedByRef {
	merged := make(map[Key]ReportedByRef, len(reportedBy))
	for _, reporter := range reportedBy {
		resolved := r.resolveAlias(reportedByIdentifier(reporter))
		key := Key{GroupResource: resolved.GroupResource.String(), Namespace: resolved.Namespace, Name: resolved.Name}
		ref := ReportedByRef{ItemRef: *newItemRef(resolved), Source: reporter.Source}
		if existing, ok := merged[key]; ok && reportSourcePrecedence[ref.Source] <= reportSourcePrecedence[existing.Source] {
			continue
		}
		merged[key] = ref
	}
	refs := make([]ReportedByRef, 0, len(merged))
	for _, reporter := range merged {
		refs = append(refs, reporter)
	}
	sortReportedBy(refs)
	return refs
}

func reportedByIdentifier(ref ReportedByRef) velero.ResourceIdentifier {
	return velero.ResourceIdentifier{
		GroupResource: schema.ParseGroupResource(ref.GroupResource),
		Namespace:     ref.Namespace,
		Name:          ref.Name,
	}
}

func sortReportedBy(reportedBy []ReportedByRef) {
	sort.Slice(reportedBy, func(i, j int) bool {
		ri, rj := reportSourcePrecedence[reportedBy[i].Source], reportSourcePrecedence[reportedBy[j].Source]
		if ri != rj {
			return ri > rj
		}
		if reportedBy[i].GroupResource != reportedBy[j].GroupResource {
			return reportedBy[i].GroupResource < reportedBy[j].GroupResource
		}
		if reportedBy[i].Namespace != reportedBy[j].Namespace {
			return reportedBy[i].Namespace < reportedBy[j].Namespace
		}
		return reportedBy[i].Name < reportedBy[j].Name
	})
}

// sortItems orders rows by group resource, namespace, and name so every writer
// produces the same artifact for the same input.
func sortItems(items []ItemOutcome) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.GroupResource != b.GroupResource {
			return a.GroupResource < b.GroupResource
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
}
