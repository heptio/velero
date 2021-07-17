/*
Copyright 2018 the Velero contributors.

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
	"context"
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/features"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

// Backupper can execute restic backups of volumes in a pod.
type Backupper interface {
	// BackupPodVolumes backs up all specified volumes in a pod.
	BackupPodVolumes(backup *velerov1api.Backup, pod *corev1api.Pod, volumesToBackup []string, log logrus.FieldLogger) ([]*velerov1api.PodVolumeBackup, []error)
}

type backupper struct {
	ctx         context.Context
	repoManager *repositoryManager
	repoEnsurer *repositoryEnsurer
	pvcClient   corev1client.PersistentVolumeClaimsGetter
	pvClient    corev1client.PersistentVolumesGetter

	results     map[string]chan *velerov1api.PodVolumeBackup
	resultsLock sync.Mutex
}

func newBackupper(
	ctx context.Context,
	repoManager *repositoryManager,
	repoEnsurer *repositoryEnsurer,
	podVolumeBackupInformer cache.SharedIndexInformer,
	pvcClient corev1client.PersistentVolumeClaimsGetter,
	pvClient corev1client.PersistentVolumesGetter,
	log logrus.FieldLogger,
) *backupper {
	b := &backupper{
		ctx:         ctx,
		repoManager: repoManager,
		repoEnsurer: repoEnsurer,
		pvcClient:   pvcClient,
		pvClient:    pvClient,

		results: make(map[string]chan *velerov1api.PodVolumeBackup),
	}

	podVolumeBackupInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(_, obj interface{}) {
				pvb := obj.(*velerov1api.PodVolumeBackup)

				if pvb.Status.Phase == velerov1api.PodVolumeBackupPhaseCompleted || pvb.Status.Phase == velerov1api.PodVolumeBackupPhaseFailed {
					b.resultsLock.Lock()
					defer b.resultsLock.Unlock()

					resChan, ok := b.results[resultsKey(pvb.Spec.Pod.Namespace, pvb.Spec.Pod.Name)]
					if !ok {
						log.Errorf("No results channel found for pod %s/%s to send pod volume backup %s/%s on", pvb.Spec.Pod.Namespace, pvb.Spec.Pod.Name, pvb.Namespace, pvb.Name)
						return
					}
					resChan <- pvb
				}
			},
		},
	)

	return b
}

func resultsKey(ns, name string) string {
	return fmt.Sprintf("%s/%s", ns, name)
}

func (b *backupper) BackupPodVolumes(backup *velerov1api.Backup, pod *corev1api.Pod, volumesToBackup []string, log logrus.FieldLogger) ([]*velerov1api.PodVolumeBackup, []error) {
	if len(volumesToBackup) == 0 {
		return nil, nil
	}

	repo, err := b.repoEnsurer.EnsureRepo(b.ctx, backup.Namespace, pod.Namespace, backup.Spec.StorageLocation)
	if err != nil {
		return nil, []error{err}
	}

	// get a single non-exclusive lock since we'll wait for all individual
	// backups to be complete before releasing it.
	b.repoManager.repoLocker.Lock(repo.Name)
	defer b.repoManager.repoLocker.Unlock(repo.Name)

	resultsChan := make(chan *velerov1api.PodVolumeBackup)

	b.resultsLock.Lock()
	b.results[resultsKey(pod.Namespace, pod.Name)] = resultsChan
	b.resultsLock.Unlock()

	var (
		errs             []error
		podVolumeBackups []*velerov1api.PodVolumeBackup
		podVolumes       = make(map[string]corev1api.Volume)
	)

	// put the pod's volumes in a map for efficient lookup below
	for _, podVolume := range pod.Spec.Volumes {
		podVolumes[podVolume.Name] = podVolume
	}

	var numVolumeSnapshots int
	for _, volumeName := range volumesToBackup {
		volume, ok := podVolumes[volumeName]
		if !ok {
			log.Warnf("No volume named %s found in pod %s/%s, skipping", volumeName, pod.Namespace, pod.Name)
			continue
		}

		var pvc *corev1api.PersistentVolumeClaim
		if volume.PersistentVolumeClaim != nil {
			pvc, err = b.pvcClient.PersistentVolumeClaims(pod.Namespace).Get(context.TODO(), volume.PersistentVolumeClaim.ClaimName, metav1.GetOptions{})
			if err != nil {
				errs = append(errs, errors.Wrap(err, "error getting persistent volume claim for volume"))
				continue
			}
		}

		// hostPath volumes are not supported because they're not mounted into /var/lib/kubelet/pods, so our
		// daemonset pod has no way to access their data.
		isHostPath, err := isHostPathVolume(&volume, pvc, b.pvClient.PersistentVolumes())
		if err != nil {
			errs = append(errs, errors.Wrap(err, "error checking if volume is a hostPath volume"))
			continue
		}
		if isHostPath {
			if features.IsEnabled(velerov1api.HostPathFlag) {
				log.Infof("Volume %s in pod %s/%s is a hostPath volume and will be backuped if available in the Restic DaemonSet", volumeName, pod.Namespace, pod.Name)
			} else {
				log.Warnf("Volume %s in pod %s/%s is a hostPath volume which is not supported for restic backup unless you activate the %s flag, skipping", volumeName, pod.Namespace, pod.Name, velerov1api.HostPathFlag)
				continue
			}
		}

		volumeBackup := newPodVolumeBackup(backup, pod, volume, repo.Spec.ResticIdentifier, pvc)
		if volumeBackup, err = b.repoManager.veleroClient.VeleroV1().PodVolumeBackups(volumeBackup.Namespace).Create(context.TODO(), volumeBackup, metav1.CreateOptions{}); err != nil {
			errs = append(errs, err)
			continue
		}
		numVolumeSnapshots++
	}

ForEachVolume:
	for i, count := 0, numVolumeSnapshots; i < count; i++ {
		select {
		case <-b.ctx.Done():
			errs = append(errs, errors.New("timed out waiting for all PodVolumeBackups to complete"))
			break ForEachVolume
		case res := <-resultsChan:
			switch res.Status.Phase {
			case velerov1api.PodVolumeBackupPhaseCompleted:
				podVolumeBackups = append(podVolumeBackups, res)
			case velerov1api.PodVolumeBackupPhaseFailed:
				errs = append(errs, errors.Errorf("pod volume backup failed: %s", res.Status.Message))
				podVolumeBackups = append(podVolumeBackups, res)
			}
		}
	}

	b.resultsLock.Lock()
	delete(b.results, resultsKey(pod.Namespace, pod.Name))
	b.resultsLock.Unlock()

	return podVolumeBackups, errs
}

type pvcGetter interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1api.PersistentVolumeClaim, error)
}

type pvGetter interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1api.PersistentVolume, error)
}

// isHostPathVolume returns true if the volume is either a hostPath pod volume or a persistent
// volume claim on a hostPath persistent volume, or false otherwise.
func isHostPathVolume(volume *corev1api.Volume, pvc *corev1api.PersistentVolumeClaim, pvGetter pvGetter) (bool, error) {
	if volume.HostPath != nil {
		return true, nil
	}

	if pvc == nil || pvc.Spec.VolumeName == "" {
		return false, nil
	}

	pv, err := pvGetter.Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return false, errors.WithStack(err)
	}

	return pv.Spec.HostPath != nil, nil
}

func newPodVolumeBackup(backup *velerov1api.Backup, pod *corev1api.Pod, volume corev1api.Volume, repoIdentifier string, pvc *corev1api.PersistentVolumeClaim) *velerov1api.PodVolumeBackup {
	pvb := &velerov1api.PodVolumeBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    backup.Namespace,
			GenerateName: backup.Name + "-",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: velerov1api.SchemeGroupVersion.String(),
					Kind:       "Backup",
					Name:       backup.Name,
					UID:        backup.UID,
					Controller: boolptr.True(),
				},
			},
			Labels: map[string]string{
				velerov1api.BackupNameLabel: label.GetValidName(backup.Name),
				velerov1api.BackupUIDLabel:  string(backup.UID),
			},
		},
		Spec: velerov1api.PodVolumeBackupSpec{
			Node: pod.Spec.NodeName,
			Pod: corev1api.ObjectReference{
				Kind:      "Pod",
				Namespace: pod.Namespace,
				Name:      pod.Name,
				UID:       pod.UID,
			},
			Volume: volume.Name,
			Tags: map[string]string{
				"backup":     backup.Name,
				"backup-uid": string(backup.UID),
				"pod":        pod.Name,
				"pod-uid":    string(pod.UID),
				"ns":         pod.Namespace,
				"volume":     volume.Name,
			},
			BackupStorageLocation: backup.Spec.StorageLocation,
			RepoIdentifier:        repoIdentifier,
		},
	}

	if pvc != nil {
		// this annotation is used in pkg/restore to identify if a PVC
		// has a restic backup.
		pvb.Annotations = map[string]string{
			PVCNameAnnotation: pvc.Name,
		}

		// this label is used by the pod volume backup controller to tell
		// if a pod volume backup is for a PVC.
		pvb.Labels[velerov1api.PVCUIDLabel] = string(pvc.UID)

		// this tag is not used by velero, but useful for debugging.
		pvb.Spec.Tags["pvc-uid"] = string(pvc.UID)
	}

	return pvb
}

func errorOnly(_ interface{}, err error) error {
	return err
}
