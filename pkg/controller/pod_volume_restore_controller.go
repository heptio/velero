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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	k8scache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/velero/internal/credentials"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov1client "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/typed/velero/v1"
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions/velero/v1"
	listers "github.com/vmware-tanzu/velero/pkg/generated/listers/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

type podVolumeRestoreController struct {
	*genericController

	podVolumeRestoreClient velerov1client.PodVolumeRestoresGetter
	podVolumeRestoreLister listers.PodVolumeRestoreLister
	podLister              corev1listers.PodLister
	pvcLister              corev1listers.PersistentVolumeClaimLister
	pvLister               corev1listers.PersistentVolumeLister
	backupLocationInformer k8scache.Informer
	kbClient               client.Client
	nodeName               string
	credentialsFileStore   credentials.FileStore

	processRestoreFunc func(*velerov1api.PodVolumeRestore) error
	fileSystem         filesystem.Interface
	clock              clock.Clock
}

// NewPodVolumeRestoreController creates a new pod volume restore controller.
func NewPodVolumeRestoreController(
	logger logrus.FieldLogger,
	podVolumeRestoreInformer informers.PodVolumeRestoreInformer,
	podVolumeRestoreClient velerov1client.PodVolumeRestoresGetter,
	podInformer cache.SharedIndexInformer,
	pvcInformer corev1informers.PersistentVolumeClaimInformer,
	pvInformer corev1informers.PersistentVolumeInformer,
	kbClient client.Client,
	nodeName string,
	credentialsFileStore credentials.FileStore,
) Interface {
	c := &podVolumeRestoreController{
		genericController:      newGenericController(PodVolumeRestore, logger),
		podVolumeRestoreClient: podVolumeRestoreClient,
		podVolumeRestoreLister: podVolumeRestoreInformer.Lister(),
		podLister:              corev1listers.NewPodLister(podInformer.GetIndexer()),
		pvcLister:              pvcInformer.Lister(),
		pvLister:               pvInformer.Lister(),
		kbClient:               kbClient,
		nodeName:               nodeName,
		credentialsFileStore:   credentialsFileStore,

		fileSystem: filesystem.NewFileSystem(),
		clock:      &clock.RealClock{},
	}

	c.syncHandler = c.processQueueItem
	c.cacheSyncWaiters = append(
		c.cacheSyncWaiters,
		podVolumeRestoreInformer.Informer().HasSynced,
		podInformer.HasSynced,
		pvcInformer.Informer().HasSynced,
	)
	c.processRestoreFunc = c.processRestore

	podVolumeRestoreInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.pvrHandler,
			UpdateFunc: func(_, obj interface{}) {
				c.pvrHandler(obj)
			},
		},
	)

	podInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.podHandler,
			UpdateFunc: func(_, obj interface{}) {
				c.podHandler(obj)
			},
		},
	)

	return c
}

func (c *podVolumeRestoreController) pvrHandler(obj interface{}) {
	pvr := obj.(*velerov1api.PodVolumeRestore)
	log := loggerForPodVolumeRestore(c.logger, pvr)

	if !isPVRNew(pvr) {
		log.Debugf("Restore is not new, not enqueuing")
		return
	}

	pod, err := c.podLister.Pods(pvr.Spec.Pod.Namespace).Get(pvr.Spec.Pod.Name)
	if apierrors.IsNotFound(err) {
		log.WithError(err).Debugf("Restore's pod %s/%s not found, not enqueueing.", pvr.Spec.Pod.Namespace, pvr.Spec.Pod.Name)
		return
	}
	if err != nil {
		log.WithError(err).Errorf("Unable to get restore's pod %s/%s, not enqueueing.", pvr.Spec.Pod.Namespace, pvr.Spec.Pod.Name)
		return
	}

	if !isPodOnNode(pod, c.nodeName) {
		log.Debugf("Restore's pod is not on this node, not enqueuing")
		return
	}

	if !isResticInitContainerRunning(pod) {
		log.Debug("Restore's pod is not running restic-wait init container, not enqueuing")
		return
	}

	resticInitContainerIndex := getResticInitContainerIndex(pod)
	if resticInitContainerIndex > 0 {
		log.Warnf(`Init containers before the %s container may cause issues
		          if they interfere with volumes being restored: %s index %d`, restic.InitContainer, restic.InitContainer, resticInitContainerIndex)
	}

	log.Debug("Enqueueing")
	c.enqueue(obj)
}

func (c *podVolumeRestoreController) podHandler(obj interface{}) {
	pod := obj.(*corev1api.Pod)
	log := c.logger.WithField("key", kube.NamespaceAndName(pod))

	// the pod should always be for this node since the podInformer is filtered
	// based on node, so this is just a failsafe.
	if !isPodOnNode(pod, c.nodeName) {
		return
	}

	if !isResticInitContainerRunning(pod) {
		log.Debug("Pod is not running restic-wait init container, not enqueuing restores for pod")
		return
	}

	resticInitContainerIndex := getResticInitContainerIndex(pod)
	if resticInitContainerIndex > 0 {
		log.Warnf(`Init containers before the %s container may cause issues
		          if they interfere with volumes being restored: %s index %d`, restic.InitContainer, restic.InitContainer, resticInitContainerIndex)
	}

	selector := labels.Set(map[string]string{
		velerov1api.PodUIDLabel: string(pod.UID),
	}).AsSelector()

	pvrs, err := c.podVolumeRestoreLister.List(selector)
	if err != nil {
		log.WithError(err).Error("Unable to list pod volume restores")
		return
	}

	if len(pvrs) == 0 {
		return
	}

	for _, pvr := range pvrs {
		log := loggerForPodVolumeRestore(log, pvr)
		if !isPVRNew(pvr) {
			log.Debug("Restore is not new, not enqueuing")
			continue
		}
		log.Debug("Enqueuing")
		c.enqueue(pvr)
	}
}

func isPVRNew(pvr *velerov1api.PodVolumeRestore) bool {
	return pvr.Status.Phase == "" || pvr.Status.Phase == velerov1api.PodVolumeRestorePhaseNew
}

func isPodOnNode(pod *corev1api.Pod, node string) bool {
	return pod.Spec.NodeName == node
}

func isResticInitContainerRunning(pod *corev1api.Pod) bool {
	// Restic wait container can be anywhere in the list of init containers, but must be running.
	i := getResticInitContainerIndex(pod)
	return i >= 0 &&
		len(pod.Status.InitContainerStatuses)-1 >= i &&
		pod.Status.InitContainerStatuses[i].State.Running != nil
}

func getResticInitContainerIndex(pod *corev1api.Pod) int {
	// Restic wait container can be anywhere in the list of init containers so locate it.
	for i, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == restic.InitContainer {
			return i
		}
	}

	return -1
}

func (c *podVolumeRestoreController) processQueueItem(key string) error {
	log := c.logger.WithField("key", key)
	log.Debug("Running processQueueItem")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(errors.WithStack(err)).Error("error splitting queue key")
		return nil
	}

	req, err := c.podVolumeRestoreLister.PodVolumeRestores(ns).Get(name)
	if apierrors.IsNotFound(err) {
		log.Debug("Unable to find PodVolumeRestore")
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "error getting PodVolumeRestore")
	}

	// Don't mutate the shared cache
	reqCopy := req.DeepCopy()
	return c.processRestoreFunc(reqCopy)
}

func loggerForPodVolumeRestore(baseLogger logrus.FieldLogger, req *velerov1api.PodVolumeRestore) logrus.FieldLogger {
	log := baseLogger.WithFields(logrus.Fields{
		"namespace": req.Namespace,
		"name":      req.Name,
	})

	if len(req.OwnerReferences) == 1 {
		log = log.WithField("restore", fmt.Sprintf("%s/%s", req.Namespace, req.OwnerReferences[0].Name))
	}

	return log
}

func (c *podVolumeRestoreController) processRestore(req *velerov1api.PodVolumeRestore) error {
	log := loggerForPodVolumeRestore(c.logger, req)

	log.Info("Restore starting")

	var err error

	// update status to InProgress
	req, err = c.patchPodVolumeRestore(req, func(r *velerov1api.PodVolumeRestore) {
		r.Status.Phase = velerov1api.PodVolumeRestorePhaseInProgress
		r.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
	})
	if err != nil {
		log.WithError(err).Error("Error setting PodVolumeRestore startTimestamp and phase to InProgress")
		return errors.WithStack(err)
	}

	pod, err := c.podLister.Pods(req.Spec.Pod.Namespace).Get(req.Spec.Pod.Name)
	if err != nil {
		log.WithError(err).Errorf("Error getting pod %s/%s", req.Spec.Pod.Namespace, req.Spec.Pod.Name)
		return c.failRestore(req, errors.Wrap(err, "error getting pod").Error(), log)
	}

	volumeDir, err := kube.GetVolumeDirectory(pod, req.Spec.Volume, c.pvcLister, c.pvLister)
	if err != nil {
		log.WithError(err).Error("Error getting volume directory name")
		return c.failRestore(req, errors.Wrap(err, "error getting volume directory name").Error(), log)
	}

	// execute the restore process
	if err := c.restorePodVolume(req, volumeDir, log); err != nil {
		log.WithError(err).Error("Error restoring volume")
		return c.failRestore(req, errors.Wrap(err, "error restoring volume").Error(), log)
	}

	// update status to Completed
	if _, err = c.patchPodVolumeRestore(req, func(r *velerov1api.PodVolumeRestore) {
		r.Status.Phase = velerov1api.PodVolumeRestorePhaseCompleted
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
	}); err != nil {
		log.WithError(err).Error("Error setting PodVolumeRestore completionTimestamp and phase to Completed")
		return err
	}

	log.Info("Restore completed")

	return nil
}

func singlePathMatch(path string) (string, error) {
	matches, err := filepath.Glob(path)
	if err != nil {
		return "", errors.WithStack(err)
	}

	if len(matches) != 1 {
		return "", errors.Errorf("expected one matching path, got %d", len(matches))
	}

	return matches[0], nil
}

func (c *podVolumeRestoreController) restorePodVolume(req *velerov1api.PodVolumeRestore, volumeDir string, log logrus.FieldLogger) error {

	// Get the full path of the new volume's directory as mounted in the daemonset pod, which
	// will look like: /host_pods/<new-pod-uid>/volumes/<volume-plugin-name>/<volume-dir>
	volumePath, err := singlePathMatch(fmt.Sprintf("/host_pods/%s/volumes/*/%s", string(req.Spec.Pod.UID), volumeDir))
	if err != nil {
		return errors.Wrap(err, "error identifying path of volume")
	}

	credsFile, err := c.credentialsFileStore.Path(restic.RepoKeySelector())
	if err != nil {
		log.WithError(err).Error("Error creating temp restic credentials file")
		return c.failRestore(req, errors.Wrap(err, "error creating temp restic credentials file").Error(), log)
	}
	// ignore error since there's nothing we can do and it's a temp file.
	defer os.Remove(credsFile)

	resticCmd := restic.RestoreCommand(
		req.Spec.RepoIdentifier,
		credsFile,
		req.Spec.SnapshotID,
		volumePath,
	)

	backupLocation := &velerov1api.BackupStorageLocation{}
	if err := c.kbClient.Get(context.Background(), client.ObjectKey{
		Namespace: req.Namespace,
		Name:      req.Spec.BackupStorageLocation,
	}, backupLocation); err != nil {
		return c.failRestore(req, errors.Wrap(err, "error getting backup storage location").Error(), log)
	}

	// if there's a caCert on the ObjectStorage, write it to disk so that it can be passed to restic
	var caCertFile string
	if backupLocation.Spec.ObjectStorage != nil && backupLocation.Spec.ObjectStorage.CACert != nil {
		caCertFile, err = restic.TempCACertFile(backupLocation.Spec.ObjectStorage.CACert, req.Spec.BackupStorageLocation, c.fileSystem)
		if err != nil {
			log.WithError(err).Error("Error creating temp cacert file")
		}
		// ignore error since there's nothing we can do and it's a temp file.
		defer os.Remove(caCertFile)
	}
	resticCmd.CACertFile = caCertFile

	env, err := restic.CmdEnv(backupLocation, c.credentialsFileStore)
	if err != nil {
		return c.failRestore(req, errors.Wrap(err, "error setting restic cmd env").Error(), log)
	}
	resticCmd.Env = env

	var stdout, stderr string

	if stdout, stderr, err = restic.RunRestore(resticCmd, log, c.updateRestoreProgressFunc(req, log)); err != nil {
		return errors.Wrapf(err, "error running restic restore, cmd=%s, stdout=%s, stderr=%s", resticCmd.String(), stdout, stderr)
	}
	log.Debugf("Ran command=%s, stdout=%s, stderr=%s", resticCmd.String(), stdout, stderr)

	// Remove the .velero directory from the restored volume (it may contain done files from previous restores
	// of this volume, which we don't want to carry over). If this fails for any reason, log and continue, since
	// this is non-essential cleanup (the done files are named based on restore UID and the init container looks
	// for the one specific to the restore being executed).
	if err := os.RemoveAll(filepath.Join(volumePath, ".velero")); err != nil {
		log.WithError(err).Warnf("error removing .velero directory from directory %s", volumePath)
	}

	var restoreUID types.UID
	for _, owner := range req.OwnerReferences {
		if boolptr.IsSetToTrue(owner.Controller) {
			restoreUID = owner.UID
			break
		}
	}

	// Create the .velero directory within the volume dir so we can write a done file
	// for this restore.
	if err := os.MkdirAll(filepath.Join(volumePath, ".velero"), 0755); err != nil {
		return errors.Wrap(err, "error creating .velero directory for done file")
	}

	// Write a done file with name=<restore-uid> into the just-created .velero dir
	// within the volume. The velero restic init container on the pod is waiting
	// for this file to exist in each restored volume before completing.
	if err := ioutil.WriteFile(filepath.Join(volumePath, ".velero", string(restoreUID)), nil, 0644); err != nil {
		return errors.Wrap(err, "error writing done file")
	}

	return nil
}

func (c *podVolumeRestoreController) patchPodVolumeRestore(req *velerov1api.PodVolumeRestore, mutate func(*velerov1api.PodVolumeRestore)) (*velerov1api.PodVolumeRestore, error) {
	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling original PodVolumeRestore")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling updated PodVolumeRestore")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrap(err, "error creating json merge patch for PodVolumeRestore")
	}

	req, err = c.podVolumeRestoreClient.PodVolumeRestores(req.Namespace).Patch(context.TODO(), req.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error patching PodVolumeRestore")
	}

	return req, nil
}

func (c *podVolumeRestoreController) failRestore(req *velerov1api.PodVolumeRestore, msg string, log logrus.FieldLogger) error {
	if _, err := c.patchPodVolumeRestore(req, func(pvr *velerov1api.PodVolumeRestore) {
		pvr.Status.Phase = velerov1api.PodVolumeRestorePhaseFailed
		pvr.Status.Message = msg
		pvr.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
	}); err != nil {
		log.WithError(err).Error("Error setting PodVolumeRestore phase to Failed")
		return err
	}
	return nil
}

// updateRestoreProgressFunc returns a func that takes progress info and patches
// the PVR with the new progress
func (c *podVolumeRestoreController) updateRestoreProgressFunc(req *velerov1api.PodVolumeRestore, log logrus.FieldLogger) func(velerov1api.PodVolumeOperationProgress) {
	return func(progress velerov1api.PodVolumeOperationProgress) {
		if _, err := c.patchPodVolumeRestore(req, func(r *velerov1api.PodVolumeRestore) {
			r.Status.Progress = progress
		}); err != nil {
			log.WithError(err).Error("error updating PodVolumeRestore progress")
		}
	}
}
