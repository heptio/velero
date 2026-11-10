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
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/velero-io/velero/internal/itemoutcome"
	"github.com/velero-io/velero/pkg/podvolume"
)

func TestBackupItemInternalRecordsFileArchiveFailure(t *testing.T) {
	gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
	const escaping = "../../../../../../escape"

	tests := []struct {
		name        string
		objectVer   string
		preferredV  string
		whichReturn string
	}{
		{
			name:        "preferred version matches, so the first call fails",
			objectVer:   "v1",
			preferredV:  "v1",
			whichReturn: "first",
		},
		{
			name:        "version differs, so only the second call runs and fails",
			objectVer:   "v1beta1",
			preferredV:  "v1",
			whichReturn: "second",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "apps/" + tc.objectVer,
				"kind":       "Deployment",
				"metadata": map[string]any{
					"namespace": "ns1",
					"name":      escaping,
				},
			}}
			// backupItemInternal reads the version off the object's kind, which
			// the unstructured literal above does not carry.
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: tc.objectVer, Kind: "Deployment"})

			ib := &itemBackupper{
				backupRequest: &Request{
					BackedUpItems:        NewBackedUpItemsMap(),
					ItemOutcomes:         itemoutcome.NewRecorder(),
					SkippedVolumeTracker: NewSkipVolumeTracker(),
				},
				podVolumeSnapshotTracker: podvolume.NewTracker(),
			}

			// mustInclude skips the inclusion filters, whose ShouldInclude needs a
			// discovery helper. The filters are not what this test is about.
			preferredGVR := schema.GroupVersionResource{Group: "apps", Version: tc.preferredV, Resource: "deployments"}
			backedUp, _, err := ib.backupItemInternal(logrus.New(), obj, gr, preferredGVR, true, false, nil)

			require.Error(t, err, "the archive path is rejected")
			assert.False(t, backedUp)

			var row *itemoutcome.ItemOutcome
			for _, item := range ib.backupRequest.ItemOutcomes.Result().Items {
				if item.GroupResource == gr.String() && item.Name == escaping {
					item := item
					row = &item
				}
			}
			require.NotNil(t, row, "the item must still have a row")
			assert.Equal(t, itemoutcome.OutcomeFailed, row.Outcome,
				"an item whose archive entry could not be built was not backed up")
			require.NotEmpty(t, row.Messages, "the reason must be attached to the row")
		})
	}
}
