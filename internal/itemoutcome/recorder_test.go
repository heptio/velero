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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/velero-io/velero/pkg/plugin/velero"
)

func pod(ns, name string) velero.ResourceIdentifier {
	return velero.ResourceIdentifier{
		GroupResource: schema.GroupResource{Resource: "pods"},
		Namespace:     ns,
		Name:          name,
	}
}

func pvc(ns, name string) velero.ResourceIdentifier {
	return velero.ResourceIdentifier{
		GroupResource: schema.GroupResource{Resource: "persistentvolumeclaims"},
		Namespace:     ns,
		Name:          name,
	}
}

const (
	vmGR  = "virtualmachines.kubevirt.io"
	pvcGR = "persistentvolumeclaims"
	pvGR  = "persistentvolumes"
	dvGR  = "datavolumes.cdi.kubevirt.io"
)

func row(gr, ns, name string, outcome Outcome, reporter *ItemRef) ItemOutcome {
	item := ItemOutcome{GroupResource: gr, Namespace: ns, Name: name, Outcome: outcome}
	if reporter != nil {
		item.ReportedBy = []ReportedByRef{{ItemRef: *reporter, Source: ReportSourceOwnerReference}}
	}
	return item
}

func ref(gr, ns, name string) *ItemRef {
	return &ItemRef{GroupResource: gr, Namespace: ns, Name: name}
}

func TestResultEmpty(t *testing.T) {
	r := NewRecorder()
	result := r.Result()
	assert.Equal(t, SchemaVersion, result.SchemaVersion)
	assert.Empty(t, result.Items)
}

func TestNilRecorderIsNoOp(t *testing.T) {
	var r *Recorder
	id := pod("ns1", "pod1")

	assert.NotPanics(t, func() {
		r.Observe(id)
		r.Warn(id, "warn")
		r.Fail(id, "fail")
		r.Skip(id, "skip")
	})

	result := r.Result()
	assert.Equal(t, SchemaVersion, result.SchemaVersion)
	assert.Empty(t, result.Items)
}

func TestObserveDefaultsToBackedUp(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	r.Observe(id)

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeSucceeded, result.Items[0].Outcome)
	assert.Equal(t, "ns1", result.Items[0].Namespace)
	assert.Equal(t, "pod1", result.Items[0].Name)
	assert.Equal(t, "pods", result.Items[0].GroupResource)
}
func TestWarnUpgradesFromBackedUp(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	r.Observe(id)
	r.Warn(id, "something odd")

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeWarning, result.Items[0].Outcome)
	assert.Equal(t, []string{"something odd"}, result.Items[0].Messages)
}

func TestFailUpgradesFromWarning(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	r.Warn(id, "warn1")
	r.Fail(id, "fail1")

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeFailed, result.Items[0].Outcome)
	assert.Equal(t, []string{"warn1", "fail1"}, result.Items[0].Messages)
}

func TestWarnDoesNotDowngradeFailed(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	r.Fail(id, "fail1")
	r.Warn(id, "warn1")

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeFailed, result.Items[0].Outcome)
}

func TestSkipPrecedenceBetweenWarningAndFailed(t *testing.T) {
	r := NewRecorder()

	warnThenSkip := pod("ns1", "warn-then-skip")
	r.Warn(warnThenSkip, "warn")
	r.Skip(warnThenSkip, "skip")

	failThenSkip := pod("ns1", "fail-then-skip")
	r.Fail(failThenSkip, "fail")
	r.Skip(failThenSkip, "skip")

	skipThenWarn := pod("ns1", "skip-then-warn")
	r.Skip(skipThenWarn, "skip")
	r.Warn(skipThenWarn, "warn")

	result := r.Result()
	byName := map[string]ItemOutcome{}
	for _, item := range result.Items {
		byName[item.Name] = item
	}

	assert.Equal(t, OutcomeSkipped, byName["warn-then-skip"].Outcome)
	assert.Equal(t, OutcomeFailed, byName["fail-then-skip"].Outcome)
	assert.Equal(t, OutcomeSkipped, byName["skip-then-warn"].Outcome)
}

func TestSkipVolumeMarksSkipProvenance(t *testing.T) {
	r := NewRecorder()

	volumeSkipped := pvc("ns", "disk")
	r.For(volumeSkipped).Observe()
	r.For(volumeSkipped).SkipVolume("podvolume skipped")

	itemSkipped := pvc("ns", "excluded")
	r.For(itemSkipped).Observe()
	r.For(itemSkipped).Skip("skipped by backup item action")

	result := r.Result()
	byName := map[string]ItemOutcome{}
	for _, item := range result.Items {
		byName[item.Name] = item
	}

	assert.True(t, byName["disk"].SkippedByVolume, "a volume-path skip must be rescuable at finalize")
	assert.False(t, byName["excluded"].SkippedByVolume, "an item-level skip must stay skipped")
}

func TestRepeatedObserveCollapsesToOneRow(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	r.Observe(id)
	r.Observe(id)
	r.Warn(id, "warn")
	r.Observe(id)

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, OutcomeWarning, result.Items[0].Outcome)
}
func TestResultIsSortedDeterministically(t *testing.T) {
	r := NewRecorder()
	r.Observe(pod("ns2", "a"))
	r.Observe(pod("ns1", "b"))
	r.Observe(pod("ns1", "a"))
	r.Observe(pvc("ns1", "a"))

	result1 := r.Result()
	result2 := r.Result()
	assert.Equal(t, result1, result2)

	require.Len(t, result1.Items, 4)
	assert.Equal(t, "persistentvolumeclaims", result1.Items[0].GroupResource)
	assert.Equal(t, "pods", result1.Items[1].GroupResource)
	assert.Equal(t, "ns1", result1.Items[1].Namespace)
	assert.Equal(t, "a", result1.Items[1].Name)
	assert.Equal(t, "ns1", result1.Items[2].Namespace)
	assert.Equal(t, "b", result1.Items[2].Name)
	assert.Equal(t, "ns2", result1.Items[3].Namespace)
}

// A row keeps the first messages, in the order they were recorded, and replaces
// the rest with one counted marker. An item can produce hundreds of errors; a
// row carrying all of them makes the artifact unreadable without telling the
// reader anything the first few did not.
func TestResultCapsMessages(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	for i := 0; i < 12; i++ {
		r.Fail(id, fmt.Sprintf("error %d", i))
	}

	msgs := r.Result().Items[0].Messages

	require.Len(t, msgs, MaxMessagesPerItem, "the cap includes the marker itself")
	assert.Equal(t, []string{
		"error 0", "error 1", "error 2", "error 3",
		"... and 8 more",
	}, msgs)
}

// A row at or under the cap is untouched: no marker, nothing reordered.
func TestResultDoesNotTruncateAtOrUnderCap(t *testing.T) {
	r := NewRecorder()
	id := pod("ns1", "pod1")
	for i := 0; i < MaxMessagesPerItem; i++ {
		r.Fail(id, fmt.Sprintf("error %d", i))
	}

	msgs := r.Result().Items[0].Messages
	require.Len(t, msgs, MaxMessagesPerItem)
	assert.NotContains(t, strings.Join(msgs, "\n"), "more")
}

// The cap counts what was recorded, not what fits, so the marker's number is the
// number of messages the reader cannot see.
func TestCapMessagesCountsHiddenMessages(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nothing to hide",
			in:   []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "exactly at the cap",
			in:   []string{"a", "b", "c", "d", "e"},
			want: []string{"a", "b", "c", "d", "e"},
		},
		{
			name: "one over the cap hides one",
			in:   []string{"a", "b", "c", "d", "e", "f"},
			want: []string{"a", "b", "c", "d", "... and 2 more"},
		},
		{
			name: "empty stays empty",
			in:   nil,
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, capMessages(tc.in))
		})
	}
}

func TestRecorderKind(t *testing.T) {
	assert.Equal(t, KindBackup, NewRecorder().Result().Kind)
}

func TestRekeyMergesWithoutDuplicatingMessages(t *testing.T) {
	r := NewRecorder()
	old := pvc("ns", "disk")
	newID := pvc("ns", "disk-generated")

	r.For(old).Observe()
	r.For(old).Warn("same")
	r.For(newID).Observe()
	r.For(newID).Warn("same")
	r.For(newID).Warn("new")
	r.For(old).Rekey(newID)

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.Equal(t, []string{"same", "new"}, result.Items[0].Messages)
}

func TestRekeyPreservesSkippedByVolume(t *testing.T) {
	r := NewRecorder()
	old := pvc("ns", "disk")
	newID := pvc("ns", "disk-generated")

	r.For(old).Observe()
	r.For(old).SkipVolume("podvolume skipped")
	r.For(newID).Observe()
	r.For(old).Rekey(newID)

	result := r.Result()
	require.Len(t, result.Items, 1)
	assert.True(t, result.Items[0].SkippedByVolume)
	assert.Equal(t, OutcomeSkipped, result.Items[0].Outcome)
}
