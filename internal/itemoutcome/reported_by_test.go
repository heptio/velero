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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/velero-io/velero/pkg/plugin/velero"
)

// An edge is recorded before the reportedItem is fetched or backed up, so the row it
// mints must not claim a success nothing has established. If the reportedItem is never
// resolved — the additional item is not found, or the reportedItem is filtered out —
// Named is the only record that anything named it at all.
func TestEdgeAloneMintsNamed(t *testing.T) {
	r := NewRecorder()
	owner := pod("ns1", "pod1")
	r.SetReportedBy(pvc("ns1", "data-disk"), owner)

	// The edge names the reportedItem; it does not invent a row for the reporter, so the
	// reporter stays a dangling reference until something processes it.
	result := r.Result()
	require.Len(t, result.Items, 1)
	reportedItem := result.Items[0]
	assert.Equal(t, OutcomeNamed, reportedItem.Outcome)
	require.Len(t, reportedItem.ReportedBy, 1)
	assert.Equal(t, *newItemRef(owner), reportedItem.ReportedBy[0].ItemRef)
	assert.Equal(t, ReportSourceAdditionalItem, reportedItem.ReportedBy[0].Source)
}

func TestReportedByRecordsEveryDistinctReporter(t *testing.T) {
	r := NewRecorder()
	reportedItem := pvc("ns1", "pvc1")
	fromOwnerRef := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "virtualmachines", Group: "kubevirt.io"}, Namespace: "ns1", Name: "vm1"}
	fromAction := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "virtualmachines", Group: "kubevirt.io"}, Namespace: "ns1", Name: "vm2"}

	r.SetOwnerReportedBy(reportedItem, fromOwnerRef)
	r.SetReportedBy(reportedItem, fromAction)

	result := r.Result()
	require.Len(t, result.Items, 1)
	require.Len(t, result.Items[0].ReportedBy, 2)
	assert.Equal(t, *newItemRef(fromAction), result.Items[0].ReportedBy[0].ItemRef,
		"additionalItem sorts first")
	assert.Equal(t, ReportSourceAdditionalItem, result.Items[0].ReportedBy[0].Source)
	assert.Equal(t, *newItemRef(fromOwnerRef), result.Items[0].ReportedBy[1].ItemRef)
	assert.Equal(t, ReportSourceOwnerReference, result.Items[0].ReportedBy[1].Source)
}

func TestSameReporterKeepsStrongerSource(t *testing.T) {
	r := NewRecorder()
	reportedItem := pvc("ns1", "pvc1")
	reporter := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "virtualmachines", Group: "kubevirt.io"}, Namespace: "ns1", Name: "vm1"}

	r.SetOwnerReportedBy(reportedItem, reporter)
	r.SetReportedBy(reportedItem, reporter)

	result := r.Result()
	require.Len(t, result.Items, 1)
	require.Len(t, result.Items[0].ReportedBy, 1)
	assert.Equal(t, *newItemRef(reporter), result.Items[0].ReportedBy[0].ItemRef)
	assert.Equal(t, ReportSourceAdditionalItem, result.Items[0].ReportedBy[0].Source)
}

func TestOwnerReferenceCannotDowngradeAdditionalItem(t *testing.T) {
	r := NewRecorder()
	reportedItem := pvc("ns1", "pvc1")
	reporter := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "virtualmachines", Group: "kubevirt.io"}, Namespace: "ns1", Name: "vm1"}

	r.SetReportedBy(reportedItem, reporter)
	r.SetOwnerReportedBy(reportedItem, reporter)

	result := r.Result()
	require.Len(t, result.Items, 1)
	require.Len(t, result.Items[0].ReportedBy, 1)
	assert.Equal(t, *newItemRef(reporter), result.Items[0].ReportedBy[0].ItemRef)
	assert.Equal(t, ReportSourceAdditionalItem, result.Items[0].ReportedBy[0].Source)
}

// Duplicate claims for the same reporter collapse to one edge.

// Duplicate claims for the same reporter collapse to one edge.
func TestDuplicateReporterClaimsCollapseToOne(t *testing.T) {
	reportedItem := pvc("ns1", "pvc1")
	reporter := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "datavolumes", Group: "cdi.kubevirt.io"}, Namespace: "ns1", Name: "dv1"}

	r := NewRecorder()
	r.SetOwnerReportedBy(reportedItem, reporter)
	r.SetOwnerReportedBy(reportedItem, reporter)

	result := r.Result()
	require.Len(t, result.Items, 1)
	require.Len(t, result.Items[0].ReportedBy, 1)
	assert.Equal(t, *newItemRef(reporter), result.Items[0].ReportedBy[0].ItemRef)
}

// Distinct equal-strength claims are all recorded, deterministically ordered.

// Distinct equal-strength claims are all recorded, deterministically ordered.
func TestEqualStrengthDistinctReportersAreBothRecorded(t *testing.T) {
	reportedItem := pvc("ns1", "pvc1")
	first := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "datavolumes", Group: "cdi.kubevirt.io"}, Namespace: "ns1", Name: "dv1"}
	second := velero.ResourceIdentifier{GroupResource: schema.GroupResource{Resource: "datavolumes", Group: "cdi.kubevirt.io"}, Namespace: "ns1", Name: "dv2"}

	r := NewRecorder()
	r.SetOwnerReportedBy(reportedItem, first)
	r.SetOwnerReportedBy(reportedItem, second)

	result := r.Result()
	require.Len(t, result.Items, 1)
	require.Len(t, result.Items[0].ReportedBy, 2)
	assert.Equal(t, "dv1", result.Items[0].ReportedBy[0].Name)
	assert.Equal(t, "dv2", result.Items[0].ReportedBy[1].Name)
}

func TestReportedByEmptyWithoutReporter(t *testing.T) {
	r := NewRecorder()
	r.Observe(pod("ns1", "pod1"))

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Empty(t, result.Items[0].ReportedBy)
}

func TestReportedByIsIndependentOfOutcome(t *testing.T) {
	r := NewRecorder()
	reporter := pod("ns1", "vm1")
	reportedItem := pvc("ns1", "pvc1")

	r.Observe(reporter)
	r.SetReportedBy(reportedItem, reporter)
	r.Fail(reportedItem, "disk full")

	result := r.Result()
	byName := map[string]ItemOutcome{}
	for _, item := range result.Items {
		byName[item.Name] = item
	}

	assert.Equal(t, OutcomeSucceeded, byName["vm1"].Outcome)
	assert.Empty(t, byName["vm1"].ReportedBy)
	assert.Equal(t, OutcomeFailed, byName["pvc1"].Outcome)
	require.Len(t, byName["pvc1"].ReportedBy, 1)
	assert.Equal(t, *newItemRef(reporter), byName["pvc1"].ReportedBy[0].ItemRef)
}

// An edge is recorded before the reportedItem is fetched, so processing the reportedItem must
// promote the edge-only row into a real verdict.
func TestObservePromotesNamedToBackedUp(t *testing.T) {
	r := NewRecorder()
	reportedItem := pvc("ns1", "data-disk")
	r.SetReportedBy(reportedItem, pod("ns1", "pod1"))
	r.Observe(reportedItem)

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeSucceeded, result.Items[0].Outcome)
}

// A verdict recorded before the edge arrives must not be lowered to Named.

// A verdict recorded before the edge arrives must not be lowered to Named.
func TestVerdictSurvivesEdgeArrivingLater(t *testing.T) {
	r := NewRecorder()
	reportedItem := pvc("ns1", "data-disk")
	r.Fail(reportedItem, "snapshot denied")
	r.SetReportedBy(reportedItem, pod("ns1", "pod1"))

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeFailed, result.Items[0].Outcome)
}

func TestSetReportedByRefusesSelfEdge(t *testing.T) {
	r := NewRecorder()
	id := pod("ns", "pod")

	r.SetReportedBy(id, id)

	assert.Empty(t, r.Result().Items)
}

func TestRekeyReportedByFollowsReporterRename(t *testing.T) {
	r := NewRecorder()
	reporter := pod("ns", "reporter")
	renamedReporter := pod("ns", "reporter-renamed")
	reportedItem := pvc("ns", "disk")

	r.SetReportedBy(reportedItem, reporter)
	r.Observe(reportedItem)
	r.Observe(reporter)
	r.For(reporter).Rekey(renamedReporter)
	r.Observe(renamedReporter)

	result := r.Result()
	byName := map[string]ItemOutcome{}
	for _, item := range result.Items {
		byName[item.Name] = item
	}

	require.Contains(t, byName, "disk")
	require.Len(t, byName["disk"].ReportedBy, 1)
	assert.Equal(t, "reporter-renamed", byName["disk"].ReportedBy[0].Name)
	assert.NotContains(t, byName, "reporter")
}

func TestRekeyResolvesChildRenameForReportedBy(t *testing.T) {
	r := NewRecorder()
	reporter := pod("ns", "reporter")
	reportedItem := pvc("ns", "disk")
	renamedChild := pvc("ns", "disk-generated")

	r.SetReportedBy(reportedItem, reporter)
	r.Observe(reportedItem)
	r.For(reportedItem).Rekey(renamedChild)

	// A later edge still names the pre-rename reportedItem identity.
	r.SetReportedBy(reportedItem, reporter)
	r.Observe(renamedChild)

	result := r.Result()
	byName := map[string]ItemOutcome{}
	for _, item := range result.Items {
		byName[item.Name] = item
	}

	require.Contains(t, byName, "disk-generated")
	require.NotContains(t, byName, "disk")
	require.Len(t, byName["disk-generated"].ReportedBy, 1)
	assert.Equal(t, "reporter", byName["disk-generated"].ReportedBy[0].Name)
}

func TestRekeyBeforeReportedByKeepsEdgeOnFinalRow(t *testing.T) {
	r := NewRecorder()
	reporter := pod("ns", "reporter")
	reportedItem := pvc("ns", "disk")
	renamedChild := pvc("ns", "disk-generated")

	// The rename is known before the edge is written.
	r.For(reportedItem).Rekey(renamedChild)
	r.SetReportedBy(reportedItem, reporter)
	r.Observe(renamedChild)

	result := r.Result()
	byName := map[string]ItemOutcome{}
	for _, item := range result.Items {
		byName[item.Name] = item
	}

	require.Contains(t, byName, "disk-generated")
	require.NotContains(t, byName, "disk")
	require.Len(t, byName["disk-generated"].ReportedBy, 1)
	assert.Equal(t, "reporter", byName["disk-generated"].ReportedBy[0].Name)
}

// A rekey that merges a reportedItem into its reporter's identity must not leave the
// merged row pointing at itself.
func TestRekeyDropsSelfEdgeAfterMerge(t *testing.T) {
	t.Run("destination row exists", func(t *testing.T) {
		r := NewRecorder()
		reportedItem := pvc("ns", "reportedItem")
		reporter := pod("ns", "reporter")

		r.SetReportedBy(reportedItem, reporter)
		r.Observe(reporter)
		r.For(reportedItem).Rekey(reporter)

		result := r.Result()
		require.Len(t, result.Items, 1)
		assert.Equal(t, "reporter", result.Items[0].Name)
		assert.Empty(t, result.Items[0].ReportedBy)
	})

	t.Run("destination row is created by the move", func(t *testing.T) {
		r := NewRecorder()
		reportedItem := pvc("ns", "reportedItem")
		reporter := pod("ns", "reporter")

		r.SetReportedBy(reportedItem, reporter)
		r.For(reportedItem).Rekey(reporter)

		result := r.Result()
		require.Len(t, result.Items, 1)
		assert.Equal(t, "reporter", result.Items[0].Name)
		assert.Equal(t, OutcomeNamed, result.Items[0].Outcome)
		assert.Empty(t, result.Items[0].ReportedBy)
	})
}

// The backup name is the artifact's stable operation identity.
