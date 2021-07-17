/*
Copyright The Velero Contributors.

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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// PodVolumeBackupBuilder builds PodVolumeBackup objects
type PodVolumeBackupBuilder struct {
	object *velerov1api.PodVolumeBackup
}

// ForPodVolumeBackup is the constructor for a PodVolumeBackupBuilder.
func ForPodVolumeBackup(ns, name string) *PodVolumeBackupBuilder {
	return &PodVolumeBackupBuilder{
		object: &velerov1api.PodVolumeBackup{
			TypeMeta: metav1.TypeMeta{
				APIVersion: velerov1api.SchemeGroupVersion.String(),
				Kind:       "PodVolumeBackup",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		},
	}
}

// Result returns the built PodVolumeBackup.
func (b *PodVolumeBackupBuilder) Result() *velerov1api.PodVolumeBackup {
	return b.object
}

// ObjectMeta applies functional options to the PodVolumeBackup's ObjectMeta.
func (b *PodVolumeBackupBuilder) ObjectMeta(opts ...ObjectMetaOpt) *PodVolumeBackupBuilder {
	for _, opt := range opts {
		opt(b.object)
	}

	return b
}

// Phase sets the PodVolumeBackup's phase.
func (b *PodVolumeBackupBuilder) Phase(phase velerov1api.PodVolumeBackupPhase) *PodVolumeBackupBuilder {
	b.object.Status.Phase = phase
	return b
}

// Node sets the PodVolumeBackup's node name.
func (b *PodVolumeBackupBuilder) Node(name string) *PodVolumeBackupBuilder {
	b.object.Spec.Node = name
	return b
}

// BackupStorageLocation sets the PodVolumeBackup's backup storage location.
func (b *PodVolumeBackupBuilder) BackupStorageLocation(name string) *PodVolumeBackupBuilder {
	b.object.Spec.BackupStorageLocation = name
	return b
}

// SnapshotID sets the PodVolumeBackup's snapshot ID.
func (b *PodVolumeBackupBuilder) SnapshotID(snapshotID string) *PodVolumeBackupBuilder {
	b.object.Status.SnapshotID = snapshotID
	return b
}

// PodName sets the name of the pod associated with this PodVolumeBackup.
func (b *PodVolumeBackupBuilder) PodName(name string) *PodVolumeBackupBuilder {
	b.object.Spec.Pod.Name = name
	return b
}

// PodNamespace sets the name of the pod associated with this PodVolumeBackup.
func (b *PodVolumeBackupBuilder) PodNamespace(ns string) *PodVolumeBackupBuilder {
	b.object.Spec.Pod.Namespace = ns
	return b
}

// Volume sets the name of the volume associated with this PodVolumeBackup.
func (b *PodVolumeBackupBuilder) Volume(volume string) *PodVolumeBackupBuilder {
	b.object.Spec.Volume = volume
	return b
}
