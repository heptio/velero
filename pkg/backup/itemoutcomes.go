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

package backup

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/velero-io/velero/internal/itemoutcome"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// recordArchiveFailure turns a tar-writer failure into a failed row. Observe
// has already minted the row at Succeeded, and the item is not in the archive,
// so returning without recording would leave a false success in the artifact.
func (ib *itemBackupper) recordArchiveFailure(obj runtime.Unstructured, groupResource schema.GroupResource, err error) {
	if metadata, mErr := meta.Accessor(obj); mErr == nil {
		ib.outcomeFor(metadata, groupResource).Fail(fmt.Sprintf("failed to write item to backup archive: %v", err))
	}
}

// outcomeFor returns the item-scoped recorder handle for obj. It is nil when
// collection is disabled, and every handle method is a no-op on nil.
func (ib *itemBackupper) outcomeFor(obj metav1.Object, groupResource schema.GroupResource) *itemoutcome.Item {
	if ib.backupRequest.ItemOutcomes == nil || obj == nil {
		return nil
	}
	return ib.backupRequest.ItemOutcomes.For(newResourceIdentifier(groupResource, obj.GetNamespace(), obj.GetName()))
}

// recordOwnerReportedBy derives the item's reported-by entry from its metadata.ownerReferences
// and records it as the floor under any edge a backup item action names. The
// derivation itself lives in itemoutcome so it is testable without a backupper.
func (ib *itemBackupper) recordOwnerReportedBy(metadata metav1.Object, item *itemoutcome.Item) {
	if item == nil {
		return
	}

	reporter, ok := itemoutcome.ReportedByFromOwnerReference(metadata, item.ID(), ib.discoveryHelper)
	if !ok {
		return
	}
	item.SetOwnerReportedBy(reporter)
}

func (ib *itemBackupper) trackSkippedVolumeOutcome(obj runtime.Unstructured, groupResource schema.GroupResource, approach string, reason string, byVolume bool, log logrus.FieldLogger) {
	pvName, pvcName, pvcNamespace, err := getVolumeTrackingInfo(obj, groupResource)
	if err == nil && (len(pvName) > 0 || len(pvcName) > 0) {
		ib.backupRequest.SkippedVolumeTracker.Track(pvName, pvcName, pvcNamespace, approach, reason)

		// The tracker is what feeds the volumeinfo artifact, but it carries no
		// link back to the item that owned the volume, so a reader chasing a
		// failed VM cannot tell where the skip came from. Record the same skip
		// against the item itself so the item-outcomes artifact explains it in
		// place. Skipped is a weaker outcome than Failed, so this never masks a
		// real failure recorded for the same item.
		if metadata, mErr := meta.Accessor(obj); mErr == nil {
			method := itemOutcomeVolumeMethod(approach)
			message := fmt.Sprintf("%s volume skipped (%s): %s", method, volumeRef(pvName, pvcName), reason)
			if method == "" {
				message = fmt.Sprintf("volume skipped (%s): %s", volumeRef(pvName, pvcName), reason)
			}
			item := ib.outcomeFor(metadata, groupResource)
			if byVolume {
				item.SkipVolume(message)
				item.RecordVolumeAttempt(itemoutcome.VolumeAttempt{
					Method:        method,
					Skipped:       true,
					SkippedReason: reason,
				})
			} else {
				item.Skip(message)
			}
		}
	} else if err != nil {
		log.WithError(err).Info("unable to get volume tracking info, skip tracking.")
	}
}

// volumeRef renders whichever of the PV/PVC identifiers is populated, so a
// skip message names the volume a reader would recognise.
func volumeRef(pvName, pvcName string) string {
	switch {
	case pvcName != "" && pvName != "":
		return fmt.Sprintf("pvc=%s pv=%s", pvcName, pvName)
	case pvcName != "":
		return fmt.Sprintf("pvc=%s", pvcName)
	case pvName != "":
		return fmt.Sprintf("pv=%s", pvName)
	default:
		return "unknown volume"
	}
}

// itemOutcomeVolumeMethod maps a skip-tracker approach to the artifact method name.
func itemOutcomeVolumeMethod(approach string) string {
	switch approach {
	case podVolumeApproach:
		return itemoutcome.VolumeMethodPodVolumeBackup
	case csiSnapshotApproach:
		return itemoutcome.VolumeMethodCSISnapshot
	case volumeSnapshotApproach, vsphereSnapshotApproach:
		return itemoutcome.VolumeMethodNativeSnapshot
	case anyApproach, "":
		return ""
	default:
		return approach
	}
}

// parseSkipReasons splits the legacy "approach: reason;" string into pairs.
func parseSkipReasons(serialized string) []Reason {
	if serialized == "" {
		return nil
	}

	var reasons []Reason
	for _, entry := range strings.Split(serialized, ";") {
		entry = strings.TrimLeft(entry, " ")
		if entry == "" {
			continue
		}

		approach, reason, ok := strings.Cut(entry, ": ")
		if !ok {
			// No prefix: keep the text as a methodless reason rather than
			// dropping the only explanation for the skip.
			reasons = append(reasons, Reason{Reason: entry})
			continue
		}
		reasons = append(reasons, Reason{Approach: approach, Reason: reason})
	}
	return reasons
}
