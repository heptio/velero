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

// Merge folds the rows recorded during the backup finalize pass into the
// stored artifact.
//
// The first write already holds the capture-time rows. The finalize pass
// re-processes post-operation items, so its recorder may hold rows and messages
// that the stored artifact does not. Dropping them would lose a finalize-time
// warning or failure while the artifact still claimed to be finalized.
//
// The envelope belongs to the stored artifact: Merge never changes
// SchemaVersion, Kind, or Finalized.
func Merge(dst, src *ItemOutcomes) {
	if dst == nil || src == nil || len(src.Items) == 0 {
		return
	}

	index := make(map[Key]int, len(dst.Items))
	for i := range dst.Items {
		index[KeyOf(dst.Items[i])] = i
	}

	for _, item := range src.Items {
		key := KeyOf(item)
		idx, ok := index[key]
		if !ok {
			dst.Items = append(dst.Items, item)
			index[key] = len(dst.Items) - 1
			continue
		}

		row := &dst.Items[idx]
		if precedence[item.Outcome] > precedence[row.Outcome] {
			row.Outcome = item.Outcome
		}
		row.Messages = mergeMessages(row.Messages, item.Messages)
		row.SkippedByVolume = row.SkippedByVolume || item.SkippedByVolume
	}

	sortItems(dst.Items)
}

// mergeMessages combines two capped message lists without losing the count of
// messages already represented by a marker.
func mergeMessages(groups ...[]string) []string {
	seen := make(map[string]bool)
	visible := make([]string, 0, MaxMessagesPerItem)
	hidden := 0

	for _, msgs := range groups {
		for _, msg := range msgs {
			if n, ok := moreMarkerCount(msg); ok {
				hidden += n
				continue
			}
			if seen[msg] {
				continue
			}
			seen[msg] = true
			visible = append(visible, msg)
		}
	}

	return capMessagesWithHidden(visible, hidden)
}
