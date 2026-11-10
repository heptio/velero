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

	"github.com/velero-io/velero/pkg/plugin/velero"
)

// Recorder accumulates per-item outcomes for one backup run, keyed by
// velero.ResourceIdentifier so repeated writes for one item collapse into a
// single row.
//
// Recorder invariants:
//   - A row is minted at OutcomeSucceeded because Velero treats an item as
//     successful unless a failure, warning or skip is recorded.
//   - Every later write only raises severity (Succeeded < Warning < Skipped <
//     Failed), so a verdict cannot be lost to a later event.
//   - A path that stops processing after Observe must record Fail, Warn or Skip;
//     otherwise the row stays Succeeded.
type Recorder struct {
	mu   sync.Mutex
	kind Kind
	rows map[velero.ResourceIdentifier]*itemOutcomeState
}

type itemOutcomeState struct {
	outcome         Outcome
	messages        []string
	skippedByVolume bool
}

// NewRecorder returns an initialized, empty backup Recorder.
func NewRecorder() *Recorder {
	return NewRecorderFor(KindBackup)
}

// NewRecorderFor returns an initialized, empty Recorder for kind.
func NewRecorderFor(kind Kind) *Recorder {
	return &Recorder{
		kind: kind,
		rows: make(map[velero.ResourceIdentifier]*itemOutcomeState),
	}
}

// rowFor returns the row for id, minting it at OutcomeSucceeded if it has none.
func (r *Recorder) rowFor(id velero.ResourceIdentifier) *itemOutcomeState {
	row, ok := r.rows[id]
	if !ok {
		row = &itemOutcomeState{outcome: OutcomeSucceeded}
		r.rows[id] = row
	}
	return row
}

// upgrade moves a row's outcome to the given outcome if it is more
// significant than the row's current outcome, per the precedence order
// Failed > Skipped > Warning > Succeeded > Named.
func (row *itemOutcomeState) upgrade(outcome Outcome) {
	if precedence[outcome] > precedence[row.outcome] {
		row.outcome = outcome
	}
}

// Observe records that an item began processing and mints its row at
// OutcomeSucceeded if it has none.
func (r *Recorder) Observe(id velero.ResourceIdentifier) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rowFor(id)
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

// rekey moves a row from old to new, merging into an existing row at the
// destination. It backs Item.Rekey, which follows a plugin-driven rename.
func (r *Recorder) rekey(old, new velero.ResourceIdentifier) {
	if r == nil || old == new {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row, ok := r.rows[old]
	if !ok {
		return
	}
	delete(r.rows, old)

	if existing, ok := r.rows[new]; ok {
		mergeRow(existing, row)
		return
	}
	r.rows[new] = row
}

// mergeRow folds src into dst, keeping the more significant outcome and
// appending messages. It is only used when a rename lands on a row that already
// exists.
func mergeRow(dst, src *itemOutcomeState) {
	if precedence[src.outcome] > precedence[dst.outcome] {
		dst.outcome = src.outcome
	}
	dst.messages = mergeMessages(dst.messages, src.messages)
	dst.skippedByVolume = dst.skippedByVolume || src.skippedByVolume
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

	if r.kind != "" {
		result.Kind = r.kind
	}

	for id, row := range r.rows {
		result.Items = append(result.Items, ItemOutcome{
			GroupResource:   id.GroupResource.String(),
			Namespace:       id.Namespace,
			Name:            id.Name,
			Outcome:         row.outcome,
			Messages:        capMessages(row.messages),
			SkippedByVolume: row.skippedByVolume,
		})
	}

	sortItems(result.Items)

	return result
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
