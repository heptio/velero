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
