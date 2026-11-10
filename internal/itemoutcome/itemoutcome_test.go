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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportedByJSONShape(t *testing.T) {
	item := ItemOutcome{
		GroupResource: "persistentvolumeclaims",
		Namespace:     "ns",
		Name:          "shared-pvc",
		Outcome:       OutcomeSucceeded,
		ReportedBy: []ReportedByRef{
			{
				ItemRef: ItemRef{GroupResource: "pods", Namespace: "ns", Name: "virt-launcher-a"},
				Source:  ReportSourceAdditionalItem,
			},
			{
				ItemRef: ItemRef{GroupResource: "pods", Namespace: "ns", Name: "virt-launcher-b"},
				Source:  ReportSourceAdditionalItem,
			},
		},
	}

	got, err := json.Marshal(item)
	require.NoError(t, err)

	want := `{
		"groupResource": "persistentvolumeclaims",
		"namespace": "ns",
		"name": "shared-pvc",
		"outcome": "Succeeded",
		"reportedBy": [
			{"groupResource": "pods", "namespace": "ns", "name": "virt-launcher-a", "source": "additionalItem"},
			{"groupResource": "pods", "namespace": "ns", "name": "virt-launcher-b", "source": "additionalItem"}
		]
	}`
	assert.JSONEq(t, want, string(got))
	assert.NotContains(t, string(got), `"parent"`)
}
