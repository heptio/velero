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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"

	api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov1client "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/typed/velero/v1"
	velerov1informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions/velero/v1"
	velerov1listers "github.com/vmware-tanzu/velero/pkg/generated/listers/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/metrics"
	"github.com/vmware-tanzu/velero/pkg/persistence"
	"github.com/vmware-tanzu/velero/pkg/plugin/clientmgmt"
	pkgrestore "github.com/vmware-tanzu/velero/pkg/restore"
	"github.com/vmware-tanzu/velero/pkg/util/collections"
	kubeutil "github.com/vmware-tanzu/velero/pkg/util/kube"
	"github.com/vmware-tanzu/velero/pkg/util/logging"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// nonRestorableResources is an exclusion list  for the restoration process. Any resources
// included here are explicitly excluded from the restoration process.
var nonRestorableResources = []string{
	"nodes",
	"events",
	"events.events.k8s.io",

	// Don't ever restore backups - if appropriate, they'll be synced in from object storage.
	// https://github.com/vmware-tanzu/velero/issues/622
	"backups.velero.io",

	// Restores are cluster-specific, and don't have value moving across clusters.
	// https://github.com/vmware-tanzu/velero/issues/622
	"restores.velero.io",

	// Restic repositories are automatically managed by Velero and will be automatically
	// created as needed if they don't exist.
	// https://github.com/vmware-tanzu/velero/issues/1113
	"resticrepositories.velero.io",
}

type restoreController struct {
	*genericController

	namespace              string
	restoreClient          velerov1client.RestoresGetter
	podVolumeBackupClient  velerov1client.PodVolumeBackupsGetter
	restorer               pkgrestore.Restorer
	backupLister           velerov1listers.BackupLister
	restoreLister          velerov1listers.RestoreLister
	kbClient               client.Client
	snapshotLocationLister velerov1listers.VolumeSnapshotLocationLister
	restoreLogLevel        logrus.Level
	metrics                *metrics.ServerMetrics
	logFormat              logging.Format
	clock                  clock.Clock

	newPluginManager  func(logger logrus.FieldLogger) clientmgmt.Manager
	backupStoreGetter persistence.ObjectBackupStoreGetter
}

func NewRestoreController(
	namespace string,
	restoreInformer velerov1informers.RestoreInformer,
	restoreClient velerov1client.RestoresGetter,
	podVolumeBackupClient velerov1client.PodVolumeBackupsGetter,
	restorer pkgrestore.Restorer,
	backupLister velerov1listers.BackupLister,
	kbClient client.Client,
	snapshotLocationLister velerov1listers.VolumeSnapshotLocationLister,
	logger logrus.FieldLogger,
	restoreLogLevel logrus.Level,
	newPluginManager func(logrus.FieldLogger) clientmgmt.Manager,
	backupStoreGetter persistence.ObjectBackupStoreGetter,
	metrics *metrics.ServerMetrics,
	logFormat logging.Format,
) Interface {
	c := &restoreController{
		genericController:      newGenericController(Restore, logger),
		namespace:              namespace,
		restoreClient:          restoreClient,
		podVolumeBackupClient:  podVolumeBackupClient,
		restorer:               restorer,
		backupLister:           backupLister,
		restoreLister:          restoreInformer.Lister(),
		kbClient:               kbClient,
		snapshotLocationLister: snapshotLocationLister,
		restoreLogLevel:        restoreLogLevel,
		metrics:                metrics,
		logFormat:              logFormat,
		clock:                  &clock.RealClock{},

		// use variables to refer to these functions so they can be
		// replaced with fakes for testing.
		newPluginManager:  newPluginManager,
		backupStoreGetter: backupStoreGetter,
	}

	c.syncHandler = c.processQueueItem
	c.resyncFunc = c.resync
	c.resyncPeriod = time.Minute

	restoreInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				restore := obj.(*api.Restore)

				switch restore.Status.Phase {
				case "", api.RestorePhaseNew:
					// only process new restores
				default:
					c.logger.WithFields(logrus.Fields{
						"restore": kubeutil.NamespaceAndName(restore),
						"phase":   restore.Status.Phase,
					}).Debug("Restore is not new, skipping")
					return
				}

				key, err := cache.MetaNamespaceKeyFunc(restore)
				if err != nil {
					c.logger.WithError(errors.WithStack(err)).WithField("restore", restore).Error("Error creating queue key, item not added to queue")
					return
				}
				c.queue.Add(key)
			},
		},
	)

	return c
}

func (c *restoreController) resync() {
	restores, err := c.restoreLister.List(labels.Everything())
	if err != nil {
		c.logger.Error(err, "Error computing restore_total metric")
	} else {
		c.metrics.SetRestoreTotal(int64(len(restores)))
	}
}

func (c *restoreController) processQueueItem(key string) error {
	log := c.logger.WithField("key", key)

	log.Debug("Running processQueueItem")
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("unable to process queue item: error splitting queue key")
		// Return nil here so we don't try to process the key any more
		return nil
	}

	log.Debug("Getting Restore")
	restore, err := c.restoreLister.Restores(ns).Get(name)
	if err != nil {
		return errors.Wrap(err, "error getting Restore")
	}

	// TODO I think this is now unnecessary. We only initially place
	// item with Phase = ("" | New) into the queue. Items will only get
	// re-queued if syncHandler returns an error, which will only
	// happen if there's an error updating Phase from its initial
	// state to something else. So any time it's re-queued it will
	// still have its initial state, which we've already confirmed
	// is ("" | New)
	switch restore.Status.Phase {
	case "", api.RestorePhaseNew:
		// only process new restores
	default:
		return nil
	}

	// Deep-copy the restore so the copy from the lister is not modified.
	// Any errors returned by processRestore will be bubbled up, meaning
	// the key will be re-enqueued by the controller.
	return c.processRestore(restore.DeepCopy())
}

func (c *restoreController) processRestore(restore *api.Restore) error {
	// Developer note: any error returned by this method will
	// cause the restore to be re-enqueued and re-processed by
	// the controller.

	// store a copy of the original restore for creating patch
	original := restore.DeepCopy()

	// Validate the restore and fetch the backup. Note that the plugin
	// manager used here is not the same one used by c.runValidatedRestore,
	// since within that function we want the plugin manager to log to
	// our per-restore log (which is instantiated within c.runValidatedRestore).
	pluginManager := c.newPluginManager(c.logger)
	defer pluginManager.CleanupClients()
	info := c.validateAndComplete(restore, pluginManager)

	// Register attempts after validation so we don't have to fetch the backup multiple times
	backupScheduleName := restore.Spec.ScheduleName
	c.metrics.RegisterRestoreAttempt(backupScheduleName)

	if len(restore.Status.ValidationErrors) > 0 {
		restore.Status.Phase = api.RestorePhaseFailedValidation
		c.metrics.RegisterRestoreValidationFailed(backupScheduleName)
	} else {
		restore.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
		restore.Status.Phase = api.RestorePhaseInProgress
	}

	// patch to update status and persist to API
	updatedRestore, err := patchRestore(original, restore, c.restoreClient)
	if err != nil {
		// return the error so the restore can be re-processed; it's currently
		// still in phase = New.
		return errors.Wrapf(err, "error updating Restore phase to %s", restore.Status.Phase)
	}
	// store ref to just-updated item for creating patch
	original = updatedRestore
	restore = updatedRestore.DeepCopy()

	if restore.Status.Phase == api.RestorePhaseFailedValidation {
		return nil
	}

	if err := c.runValidatedRestore(restore, info); err != nil {
		c.logger.WithError(err).Debug("Restore failed")
		restore.Status.Phase = api.RestorePhaseFailed
		restore.Status.FailureReason = err.Error()
		c.metrics.RegisterRestoreFailed(backupScheduleName)
	} else if restore.Status.Errors > 0 {
		c.logger.Debug("Restore partially failed")
		restore.Status.Phase = api.RestorePhasePartiallyFailed
		c.metrics.RegisterRestorePartialFailure(backupScheduleName)
	} else {
		c.logger.Debug("Restore completed")
		restore.Status.Phase = api.RestorePhaseCompleted
		c.metrics.RegisterRestoreSuccess(backupScheduleName)
	}

	restore.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
	c.logger.Debug("Updating restore's final status")
	if _, err = patchRestore(original, restore, c.restoreClient); err != nil {
		c.logger.WithError(errors.WithStack(err)).Info("Error updating restore's final status")
	}

	return nil
}

type backupInfo struct {
	backup      *api.Backup
	location    *velerov1api.BackupStorageLocation
	backupStore persistence.BackupStore
}

func (c *restoreController) validateAndComplete(restore *api.Restore, pluginManager clientmgmt.Manager) backupInfo {
	// add non-restorable resources to restore's excluded resources
	excludedResources := sets.NewString(restore.Spec.ExcludedResources...)
	for _, nonrestorable := range nonRestorableResources {
		if !excludedResources.Has(nonrestorable) {
			restore.Spec.ExcludedResources = append(restore.Spec.ExcludedResources, nonrestorable)
		}
	}

	// validate that included resources don't contain any non-restorable resources
	includedResources := sets.NewString(restore.Spec.IncludedResources...)
	for _, nonRestorableResource := range nonRestorableResources {
		if includedResources.Has(nonRestorableResource) {
			restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, fmt.Sprintf("%v are non-restorable resources", nonRestorableResource))
		}
	}

	// validate included/excluded resources
	for _, err := range collections.ValidateIncludesExcludes(restore.Spec.IncludedResources, restore.Spec.ExcludedResources) {
		restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, fmt.Sprintf("Invalid included/excluded resource lists: %v", err))
	}

	// validate included/excluded namespaces
	for _, err := range collections.ValidateIncludesExcludes(restore.Spec.IncludedNamespaces, restore.Spec.ExcludedNamespaces) {
		restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, fmt.Sprintf("Invalid included/excluded namespace lists: %v", err))
	}

	// validate that exactly one of BackupName and ScheduleName have been specified
	if !backupXorScheduleProvided(restore) {
		restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, "Either a backup or schedule must be specified as a source for the restore, but not both")
		return backupInfo{}
	}

	// if ScheduleName is specified, fill in BackupName with the most recent successful backup from
	// the schedule
	if restore.Spec.ScheduleName != "" {
		selector := labels.SelectorFromSet(labels.Set(map[string]string{
			velerov1api.ScheduleNameLabel: restore.Spec.ScheduleName,
		}))

		backups, err := c.backupLister.Backups(c.namespace).List(selector)
		if err != nil {
			restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, "Unable to list backups for schedule")
			return backupInfo{}
		}
		if len(backups) == 0 {
			restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, "No backups found for schedule")
		}

		if backup := mostRecentCompletedBackup(backups); backup != nil {
			restore.Spec.BackupName = backup.Name
		} else {
			restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, "No completed backups found for schedule")
			return backupInfo{}
		}
	}

	info, err := c.fetchBackupInfo(restore.Spec.BackupName, pluginManager)
	if err != nil {
		restore.Status.ValidationErrors = append(restore.Status.ValidationErrors, fmt.Sprintf("Error retrieving backup: %v", err))
		return backupInfo{}
	}

	// Fill in the ScheduleName so it's easier to consume for metrics.
	if restore.Spec.ScheduleName == "" {
		restore.Spec.ScheduleName = info.backup.GetLabels()[velerov1api.ScheduleNameLabel]
	}

	return info
}

// backupXorScheduleProvided returns true if exactly one of BackupName and
// ScheduleName are non-empty for the restore, or false otherwise.
func backupXorScheduleProvided(restore *api.Restore) bool {
	if restore.Spec.BackupName != "" && restore.Spec.ScheduleName != "" {
		return false
	}

	if restore.Spec.BackupName == "" && restore.Spec.ScheduleName == "" {
		return false
	}

	return true
}

// mostRecentCompletedBackup returns the most recent backup that's
// completed from a list of backups.
func mostRecentCompletedBackup(backups []*api.Backup) *api.Backup {
	sort.Slice(backups, func(i, j int) bool {
		// Use .After() because we want descending sort.

		var iStartTime, jStartTime time.Time
		if backups[i].Status.StartTimestamp != nil {
			iStartTime = backups[i].Status.StartTimestamp.Time
		}
		if backups[j].Status.StartTimestamp != nil {
			jStartTime = backups[j].Status.StartTimestamp.Time
		}
		return iStartTime.After(jStartTime)
	})

	for _, backup := range backups {
		if backup.Status.Phase == api.BackupPhaseCompleted {
			return backup
		}
	}

	return nil
}

// fetchBackupInfo checks the backup lister for a backup that matches the given name. If it doesn't
// find it, it returns an error.
func (c *restoreController) fetchBackupInfo(backupName string, pluginManager clientmgmt.Manager) (backupInfo, error) {
	backup, err := c.backupLister.Backups(c.namespace).Get(backupName)
	if err != nil {
		return backupInfo{}, err
	}

	location := &velerov1api.BackupStorageLocation{}
	if err := c.kbClient.Get(context.Background(), client.ObjectKey{
		Namespace: c.namespace,
		Name:      backup.Spec.StorageLocation,
	}, location); err != nil {
		return backupInfo{}, errors.WithStack(err)
	}

	backupStore, err := c.backupStoreGetter.Get(location, pluginManager, c.logger)
	if err != nil {
		return backupInfo{}, err
	}

	return backupInfo{
		backup:      backup,
		location:    location,
		backupStore: backupStore,
	}, nil
}

// runValidatedRestore takes a validated restore API object and executes the restore process.
// The log and results files are uploaded to backup storage. Any error returned from this function
// means that the restore failed. This function updates the restore API object with warning and error
// counts, but *does not* update its phase or patch it via the API.
func (c *restoreController) runValidatedRestore(restore *api.Restore, info backupInfo) error {
	// instantiate the per-restore logger that will output both to a temp file
	// (for upload to object storage) and to stdout.
	restoreLog, err := newRestoreLogger(restore, c.logger, c.restoreLogLevel, c.logFormat)
	if err != nil {
		return err
	}
	defer restoreLog.closeAndRemove(c.logger)

	pluginManager := c.newPluginManager(restoreLog)
	defer pluginManager.CleanupClients()

	actions, err := pluginManager.GetRestoreItemActions()
	if err != nil {
		return errors.Wrap(err, "error getting restore item actions")
	}

	backupFile, err := downloadToTempFile(restore.Spec.BackupName, info.backupStore, restoreLog)
	if err != nil {
		return errors.Wrap(err, "error downloading backup")
	}
	defer closeAndRemoveFile(backupFile, c.logger)

	opts := label.NewListOptionsForBackup(restore.Spec.BackupName)

	podVolumeBackupList, err := c.podVolumeBackupClient.PodVolumeBackups(c.namespace).List(context.TODO(), opts)
	if err != nil {
		return errors.WithStack(err)
	}

	volumeSnapshots, err := info.backupStore.GetBackupVolumeSnapshots(restore.Spec.BackupName)
	if err != nil {
		return errors.Wrap(err, "error fetching volume snapshots metadata")
	}

	restoreLog.Info("starting restore")

	var podVolumeBackups []*velerov1api.PodVolumeBackup
	for i := range podVolumeBackupList.Items {
		podVolumeBackups = append(podVolumeBackups, &podVolumeBackupList.Items[i])
	}
	restoreReq := pkgrestore.Request{
		Log:              restoreLog,
		Restore:          restore,
		Backup:           info.backup,
		PodVolumeBackups: podVolumeBackups,
		VolumeSnapshots:  volumeSnapshots,
		BackupReader:     backupFile,
	}
	restoreWarnings, restoreErrors := c.restorer.Restore(restoreReq, actions, c.snapshotLocationLister, pluginManager)
	restoreLog.Info("restore completed")

	// re-instantiate the backup store because credentials could have changed since the original
	// instantiation, if this was a long-running restore
	info.backupStore, err = c.backupStoreGetter.Get(info.location, pluginManager, c.logger)
	if err != nil {
		return errors.Wrap(err, "error setting up backup store to persist log and results files")
	}

	if logReader, err := restoreLog.done(c.logger); err != nil {
		restoreErrors.Velero = append(restoreErrors.Velero, fmt.Sprintf("error getting restore log reader: %v", err))
	} else {
		if err := info.backupStore.PutRestoreLog(restore.Spec.BackupName, restore.Name, logReader); err != nil {
			restoreErrors.Velero = append(restoreErrors.Velero, fmt.Sprintf("error uploading log file to backup storage: %v", err))
		}
	}

	// At this point, no further logs should be written to restoreLog since it's been uploaded
	// to object storage.

	restore.Status.Warnings = len(restoreWarnings.Velero) + len(restoreWarnings.Cluster)
	for _, w := range restoreWarnings.Namespaces {
		restore.Status.Warnings += len(w)
	}

	restore.Status.Errors = len(restoreErrors.Velero) + len(restoreErrors.Cluster)
	for _, e := range restoreErrors.Namespaces {
		restore.Status.Errors += len(e)
	}

	m := map[string]pkgrestore.Result{
		"warnings": restoreWarnings,
		"errors":   restoreErrors,
	}

	if err := putResults(restore, m, info.backupStore, c.logger); err != nil {
		c.logger.WithError(err).Error("Error uploading restore results to backup storage")
	}

	return nil
}

func putResults(restore *api.Restore, results map[string]pkgrestore.Result, backupStore persistence.BackupStore, log logrus.FieldLogger) error {
	buf := new(bytes.Buffer)
	gzw := gzip.NewWriter(buf)
	defer gzw.Close()

	if err := json.NewEncoder(gzw).Encode(results); err != nil {
		return errors.Wrap(err, "error encoding restore results to JSON")
	}

	if err := gzw.Close(); err != nil {
		return errors.Wrap(err, "error closing gzip writer")
	}

	if err := backupStore.PutRestoreResults(restore.Spec.BackupName, restore.Name, buf); err != nil {
		return err
	}

	return nil
}

func downloadToTempFile(backupName string, backupStore persistence.BackupStore, logger logrus.FieldLogger) (*os.File, error) {
	readCloser, err := backupStore.GetBackupContents(backupName)
	if err != nil {
		return nil, err
	}
	defer readCloser.Close()

	file, err := ioutil.TempFile("", backupName)
	if err != nil {
		return nil, errors.Wrap(err, "error creating Backup temp file")
	}

	n, err := io.Copy(file, readCloser)
	if err != nil {
		//Temporary file has been created if we go here. And some problems occurs such as network interruption and
		//so on. So we close and remove temporary file first to prevent residual file.
		closeAndRemoveFile(file, logger)
		return nil, errors.Wrap(err, "error copying Backup to temp file")
	}

	log := logger.WithField("backup", backupName)

	log.WithFields(logrus.Fields{
		"fileName": file.Name(),
		"bytes":    n,
	}).Debug("Copied Backup to file")

	if _, err := file.Seek(0, 0); err != nil {
		closeAndRemoveFile(file, logger)
		return nil, errors.Wrap(err, "error resetting Backup file offset")
	}

	return file, nil
}

func patchRestore(original, updated *api.Restore, client velerov1client.RestoresGetter) (*api.Restore, error) {
	origBytes, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling original restore")
	}

	updatedBytes, err := json.Marshal(updated)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling updated restore")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(origBytes, updatedBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error creating json merge patch for restore")
	}

	res, err := client.Restores(original.Namespace).Patch(context.TODO(), original.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error patching restore")
	}

	return res, nil
}

type restoreLogger struct {
	logrus.FieldLogger
	file *os.File
	w    *gzip.Writer
}

func newRestoreLogger(restore *api.Restore, baseLogger logrus.FieldLogger, logLevel logrus.Level, logFormat logging.Format) (*restoreLogger, error) {
	file, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, errors.Wrap(err, "error creating temp file")
	}
	w := gzip.NewWriter(file)

	logger := logging.DefaultLogger(logLevel, logFormat)
	logger.Out = io.MultiWriter(os.Stdout, w)

	return &restoreLogger{
		FieldLogger: logger.WithField("restore", kubeutil.NamespaceAndName(restore)),
		file:        file,
		w:           w,
	}, nil
}

// done stops the restoreLogger from being able to be written to, and returns
// an io.Reader for getting the content of the logger. Any attempts to use
// restoreLogger to log after calling done will panic.
func (l *restoreLogger) done(log logrus.FieldLogger) (io.Reader, error) {
	l.FieldLogger = nil

	if err := l.w.Close(); err != nil {
		log.WithError(errors.WithStack(err)).Error("error closing gzip writer")
	}

	if _, err := l.file.Seek(0, 0); err != nil {
		return nil, errors.Wrap(err, "error resetting log file offset to 0")
	}

	return l.file, nil
}

// closeAndRemove removes the logger's underlying temporary storage. This
// method should be called when all logging and reading from the logger is
// complete.
func (l *restoreLogger) closeAndRemove(log logrus.FieldLogger) {
	closeAndRemoveFile(l.file, log)
}
