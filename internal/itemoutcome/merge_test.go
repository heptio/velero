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
)

func TestMergeAddsFinalizePassRows(t *testing.T) {
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Kind:          KindBackup,
		Finalized:     true,
		Items:         []ItemOutcome{row(pvcGR, "ns", "a", OutcomeSucceeded, nil)},
	}
	src := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Kind:          KindBackup,
		Items:         []ItemOutcome{row("pods", "ns", "z", OutcomeSucceeded, nil)},
	}

	Merge(dst, src)

	require.Len(t, dst.Items, 2)
	assert.Equal(t, "a", dst.Items[0].Name)
	assert.Equal(t, "z", dst.Items[1].Name)
	assert.True(t, dst.Finalized)
	assert.Equal(t, KindBackup, dst.Kind)
}

func TestMergeRaisesOutcomeAndKeepsMessages(t *testing.T) {
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			Messages:      []string{"capture"},
		}},
	}
	src := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeFailed,
			Messages:      []string{"finalize"},
		}},
	}

	Merge(dst, src)

	require.Len(t, dst.Items, 1)
	assert.Equal(t, OutcomeFailed, dst.Items[0].Outcome)
	assert.Equal(t, []string{"capture", "finalize"}, dst.Items[0].Messages)
}
func TestMergePreservesSkippedByVolume(t *testing.T) {
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
		}},
	}
	src := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource:   pvcGR,
			Namespace:       "ns",
			Name:            "a",
			Outcome:         OutcomeSkipped,
			SkippedByVolume: true,
		}},
	}

	Merge(dst, src)

	require.Len(t, dst.Items, 1)
	assert.Equal(t, OutcomeSkipped, dst.Items[0].Outcome)
	assert.True(t, dst.Items[0].SkippedByVolume, "a finalize-pass row must keep the volume-skip provenance")
}

func TestMergePreservesEnvelope(t *testing.T) {
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Kind:          KindBackup,
		Finalized:     true,
	}
	src := &ItemOutcomes{
		SchemaVersion: "v2",
		Kind:          Kind("restore"),
		Finalized:     false,
		Items:         []ItemOutcome{row(pvcGR, "ns", "a", OutcomeSucceeded, nil)},
	}

	Merge(dst, src)

	assert.Equal(t, SchemaVersion, dst.SchemaVersion)
	assert.Equal(t, KindBackup, dst.Kind)
	assert.True(t, dst.Finalized)
	require.Len(t, dst.Items, 1)
}

func TestMergeMessagesKeepsHiddenCount(t *testing.T) {
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			Messages:      []string{"m1", "m2", "m3", "... and 2 more"},
		}},
	}
	src := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			Messages:      []string{"m4"},
		}},
	}

	Merge(dst, src)

	require.Len(t, dst.Items[0].Messages, MaxMessagesPerItem)
	assert.Equal(t, []string{"m1", "m2", "m3", "m4", "... and 2 more"}, dst.Items[0].Messages)
}

func TestMergeHandlesNilArtifacts(t *testing.T) {
	assert.NotPanics(t, func() {
		Merge(nil, nil)
		Merge(&ItemOutcomes{SchemaVersion: SchemaVersion}, nil)
		Merge(nil, &ItemOutcomes{SchemaVersion: SchemaVersion})
	})
}

func TestMergeUpgradesReportSourceForSameReporter(t *testing.T) {
	reporter := ref("pods", "ns", "plugin")
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			ReportedBy: []ReportedByRef{{
				ItemRef: *reporter,
				Source:  ReportSourceOwnerReference,
			}},
		}},
	}
	src := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			ReportedBy: []ReportedByRef{{
				ItemRef: *reporter,
				Source:  ReportSourceAdditionalItem,
			}},
		}},
	}

	Merge(dst, src)

	require.Len(t, dst.Items[0].ReportedBy, 1)
	assert.Equal(t, *reporter, dst.Items[0].ReportedBy[0].ItemRef)
	assert.Equal(t, ReportSourceAdditionalItem, dst.Items[0].ReportedBy[0].Source)
}

func TestMergeUnionsDistinctReporters(t *testing.T) {
	dst := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			ReportedBy: []ReportedByRef{{
				ItemRef: *ref("pods", "ns", "owner"),
				Source:  ReportSourceOwnerReference,
			}},
		}},
	}
	src := &ItemOutcomes{
		SchemaVersion: SchemaVersion,
		Items: []ItemOutcome{{
			GroupResource: pvcGR,
			Namespace:     "ns",
			Name:          "a",
			Outcome:       OutcomeSucceeded,
			ReportedBy: []ReportedByRef{{
				ItemRef: *ref("pods", "ns", "plugin"),
				Source:  ReportSourceAdditionalItem,
			}},
		}},
	}

	Merge(dst, src)

	require.Len(t, dst.Items[0].ReportedBy, 2)
	assert.Equal(t, "plugin", dst.Items[0].ReportedBy[0].Name, "additionalItem sorts first")
	assert.Equal(t, ReportSourceAdditionalItem, dst.Items[0].ReportedBy[0].Source)
	assert.Equal(t, "owner", dst.Items[0].ReportedBy[1].Name)
	assert.Equal(t, ReportSourceOwnerReference, dst.Items[0].ReportedBy[1].Source)
}
