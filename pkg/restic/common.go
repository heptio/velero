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

package restic

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/vmware-tanzu/velero/internal/credentials"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/features"
	velerov1listers "github.com/vmware-tanzu/velero/pkg/generated/listers/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
)

const (
	// DaemonSet is the name of the Velero restic daemonset.
	DaemonSet = "restic"

	// InitContainer is the name of the init container added
	// to workload pods to help with restores.
	InitContainer = "restic-wait"

	// DefaultMaintenanceFrequency is the default time interval
	// at which restic prune is run.
	DefaultMaintenanceFrequency = 7 * 24 * time.Hour

	// DefaultVolumesToRestic specifies whether restic should be used, by default, to
	// take backup of all pod volumes.
	DefaultVolumesToRestic = false

	// PVCNameAnnotation is the key for the annotation added to
	// pod volume backups when they're for a PVC.
	PVCNameAnnotation = "velero.io/pvc-name"

	// VolumesToBackupAnnotation is the annotation on a pod whose mounted volumes
	// need to be backed up using restic.
	VolumesToBackupAnnotation = "backup.velero.io/backup-volumes"

	// VolumesToExcludeAnnotation is the annotation on a pod whose mounted volumes
	// should be excluded from restic backup.
	VolumesToExcludeAnnotation = "backup.velero.io/backup-volumes-excludes"

	// credentialsFileKey is the key within a BSL config that is checked to see if
	// the BSL is using its own credentials, rather than those in the environment
	credentialsFileKey = "credentialsFile"

	// Deprecated.
	//
	// TODO(2.0): remove
	podAnnotationPrefix = "snapshot.velero.io/"
)

// getPodSnapshotAnnotations returns a map, of volume name -> snapshot id,
// of all restic snapshots for this pod.
// TODO(2.0) to remove
// Deprecated: we will stop using pod annotations to record restic snapshot IDs after they're taken,
// therefore we won't need to check if these annotations exist.
func getPodSnapshotAnnotations(obj metav1.Object) map[string]string {
	var res map[string]string

	insertSafe := func(k, v string) {
		if res == nil {
			res = make(map[string]string)
		}
		res[k] = v
	}

	for k, v := range obj.GetAnnotations() {
		if strings.HasPrefix(k, podAnnotationPrefix) {
			insertSafe(k[len(podAnnotationPrefix):], v)
		}
	}

	return res
}

func isPVBMatchPod(pvb *velerov1api.PodVolumeBackup, podName string, namespace string) bool {
	return podName == pvb.Spec.Pod.Name && namespace == pvb.Spec.Pod.Namespace
}

// volumeIsProjected checks if the given volume exists in the list of podVolumes
// and returns true if the volume has a projected source
func volumeIsProjected(volumeName string, podVolumes []corev1api.Volume) bool {
	for _, volume := range podVolumes {
		if volume.Name == volumeName && volume.Projected != nil {
			return true
		}
	}
	return false
}

// GetVolumeBackupsForPod returns a map, of volume name -> snapshot id,
// of the PodVolumeBackups that exist for the provided pod.
func GetVolumeBackupsForPod(podVolumeBackups []*velerov1api.PodVolumeBackup, pod *corev1api.Pod, sourcePodNs string) map[string]string {
	volumes := make(map[string]string)

	for _, pvb := range podVolumeBackups {
		if !isPVBMatchPod(pvb, pod.GetName(), sourcePodNs) {
			continue
		}

		// skip PVBs without a snapshot ID since there's nothing
		// to restore (they could be failed, or for empty volumes).
		if pvb.Status.SnapshotID == "" {
			continue
		}

		// If the volume came from a projected source, skip its restore.
		// This allows backups affected by https://github.com/vmware-tanzu/velero/issues/3863
		// to be restored successfully.
		if volumeIsProjected(pvb.Spec.Volume, pod.Spec.Volumes) {
			continue
		}

		volumes[pvb.Spec.Volume] = pvb.Status.SnapshotID
	}

	if len(volumes) > 0 {
		return volumes
	}

	return getPodSnapshotAnnotations(pod)
}

// GetVolumesToBackup returns a list of volume names to backup for
// the provided pod.
// Deprecated: Use GetPodVolumesUsingRestic instead.
func GetVolumesToBackup(obj metav1.Object) []string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}

	backupsValue := annotations[VolumesToBackupAnnotation]
	if backupsValue == "" {
		return nil
	}

	return strings.Split(backupsValue, ",")
}

func getVolumesToExclude(obj metav1.Object) []string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}

	return strings.Split(annotations[VolumesToExcludeAnnotation], ",")
}

func contains(list []string, k string) bool {
	for _, i := range list {
		if i == k {
			return true
		}
	}
	return false
}

// GetPodVolumesUsingRestic returns a list of volume names to backup for the provided pod.
func GetPodVolumesUsingRestic(pod *corev1api.Pod, defaultVolumesToRestic bool) []string {
	if !defaultVolumesToRestic {
		return GetVolumesToBackup(pod)
	}

	volsToExclude := getVolumesToExclude(pod)
	podVolumes := []string{}
	for _, pv := range pod.Spec.Volumes {
		if pv.HostPath != nil && !features.IsEnabled(velerov1api.HostPathFlag) {
			// cannot backup hostpath volumes as they are not mounted into /var/lib/kubelet/pods
			// and therefore not accessible to the restic daemon set.
			continue
		}
		// don't backup volumes mounting secrets. Secrets will be backed up separately.
		if pv.Secret != nil {
			continue
		}
		// don't backup volumes mounting config maps. Config maps will be backed up separately.
		if pv.ConfigMap != nil {
			continue
		}
		// don't backup volumes mounted as projected volumes, all data in those come from kube state.
		if pv.Projected != nil {
			continue
		}
		// don't backup volumes that are included in the exclude list.
		if contains(volsToExclude, pv.Name) {
			continue
		}
		// don't include volumes that mount the default service account token.
		if strings.HasPrefix(pv.Name, "default-token") {
			continue
		}
		podVolumes = append(podVolumes, pv.Name)
	}
	return podVolumes
}

// SnapshotIdentifier uniquely identifies a restic snapshot
// taken by Velero.
type SnapshotIdentifier struct {
	// VolumeNamespace is the namespace of the pod/volume that
	// the restic snapshot is for.
	VolumeNamespace string

	// BackupStorageLocation is the backup's storage location
	// name.
	BackupStorageLocation string

	// SnapshotID is the short ID of the restic snapshot.
	SnapshotID string
}

// GetSnapshotsInBackup returns a list of all restic snapshot ids associated with
// a given Velero backup.
func GetSnapshotsInBackup(backup *velerov1api.Backup, podVolumeBackupLister velerov1listers.PodVolumeBackupLister) ([]SnapshotIdentifier, error) {
	selector := labels.Set(map[string]string{
		velerov1api.BackupNameLabel: label.GetValidName(backup.Name),
	}).AsSelector()

	podVolumeBackups, err := podVolumeBackupLister.List(selector)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var res []SnapshotIdentifier
	for _, item := range podVolumeBackups {
		if item.Status.SnapshotID == "" {
			continue
		}
		res = append(res, SnapshotIdentifier{
			VolumeNamespace:       item.Spec.Pod.Namespace,
			BackupStorageLocation: backup.Spec.StorageLocation,
			SnapshotID:            item.Status.SnapshotID,
		})
	}

	return res, nil
}

// TempCACertFile creates a temp file containing a CA bundle
// and returns its path. The caller should generally call os.Remove()
// to remove the file when done with it.
func TempCACertFile(caCert []byte, bsl string, fs filesystem.Interface) (string, error) {
	file, err := fs.TempFile("", fmt.Sprintf("cacert-%s", bsl))
	if err != nil {
		return "", errors.WithStack(err)
	}

	if _, err := file.Write(caCert); err != nil {
		// nothing we can do about an error closing the file here, and we're
		// already returning an error about the write failing.
		file.Close()
		return "", errors.WithStack(err)
	}

	name := file.Name()

	if err := file.Close(); err != nil {
		return "", errors.WithStack(err)
	}

	return name, nil
}

// NewPodVolumeRestoreListOptions creates a ListOptions with a label selector configured to
// find PodVolumeRestores for the restore identified by name.
func NewPodVolumeRestoreListOptions(name string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", velerov1api.RestoreNameLabel, label.GetValidName(name)),
	}
}

// CmdEnv returns a list of environment variables (in the format var=val) that
// should be used when running a restic command for a particular backend provider.
// This list is the current environment, plus any provider-specific variables restic needs.
func CmdEnv(backupLocation *velerov1api.BackupStorageLocation, credentialFileStore credentials.FileStore) ([]string, error) {
	env := os.Environ()
	customEnv := map[string]string{}
	var err error

	config := backupLocation.Spec.Config
	if config == nil {
		config = map[string]string{}
	}

	if backupLocation.Spec.Credential != nil {
		credsFile, err := credentialFileStore.Path(backupLocation.Spec.Credential)
		if err != nil {
			return []string{}, errors.WithStack(err)
		}
		config[credentialsFileKey] = credsFile
	}

	backendType := getBackendType(backupLocation.Spec.Provider)

	switch backendType {
	case AWSBackend:
		customEnv, err = getS3ResticEnvVars(config)
		if err != nil {
			return []string{}, err
		}
	case AzureBackend:
		customEnv, err = getAzureResticEnvVars(config)
		if err != nil {
			return []string{}, err
		}
	case GCPBackend:
		customEnv, err = getGCPResticEnvVars(config)
		if err != nil {
			return []string{}, err
		}
	}

	for k, v := range customEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env, nil
}
