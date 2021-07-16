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
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubeerrs "k8s.io/apimachinery/pkg/util/errors"

	"github.com/vmware-tanzu/velero/internal/hook"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/discovery"
	velerov1client "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/typed/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/kuberesource"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"github.com/vmware-tanzu/velero/pkg/podexec"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/util/collections"
)

// BackupVersion is the current backup major version for Velero.
// Deprecated, use BackupFormatVersion
const BackupVersion = 1

// BackupFormatVersion is the current backup version for Velero, including major, minor, and patch.
const BackupFormatVersion = "1.1.0"

// Backupper performs backups.
type Backupper interface {
	// Backup takes a backup using the specification in the velerov1api.Backup and writes backup and log data
	// to the given writers.
	Backup(logger logrus.FieldLogger, backup *Request, backupFile io.Writer, actions []velero.BackupItemAction, volumeSnapshotterGetter VolumeSnapshotterGetter) error
}

// kubernetesBackupper implements Backupper.
type kubernetesBackupper struct {
	backupClient           velerov1client.BackupsGetter
	dynamicFactory         client.DynamicFactory
	discoveryHelper        discovery.Helper
	podCommandExecutor     podexec.PodCommandExecutor
	resticBackupperFactory restic.BackupperFactory
	resticTimeout          time.Duration
	defaultVolumesToRestic bool
	clientPageSize         int
}

type resolvedAction struct {
	velero.BackupItemAction

	resourceIncludesExcludes  *collections.IncludesExcludes
	namespaceIncludesExcludes *collections.IncludesExcludes
	selector                  labels.Selector
}

func (i *itemKey) String() string {
	return fmt.Sprintf("resource=%s,namespace=%s,name=%s", i.resource, i.namespace, i.name)
}

func cohabitatingResources() map[string]*cohabitatingResource {
	return map[string]*cohabitatingResource{
		"deployments":     newCohabitatingResource("deployments", "extensions", "apps"),
		"daemonsets":      newCohabitatingResource("daemonsets", "extensions", "apps"),
		"replicasets":     newCohabitatingResource("replicasets", "extensions", "apps"),
		"networkpolicies": newCohabitatingResource("networkpolicies", "extensions", "networking.k8s.io"),
		"events":          newCohabitatingResource("events", "", "events.k8s.io"),
	}
}

// NewKubernetesBackupper creates a new kubernetesBackupper.
func NewKubernetesBackupper(
	backupClient velerov1client.BackupsGetter,
	discoveryHelper discovery.Helper,
	dynamicFactory client.DynamicFactory,
	podCommandExecutor podexec.PodCommandExecutor,
	resticBackupperFactory restic.BackupperFactory,
	resticTimeout time.Duration,
	defaultVolumesToRestic bool,
	clientPageSize int,
) (Backupper, error) {
	return &kubernetesBackupper{
		backupClient:           backupClient,
		discoveryHelper:        discoveryHelper,
		dynamicFactory:         dynamicFactory,
		podCommandExecutor:     podCommandExecutor,
		resticBackupperFactory: resticBackupperFactory,
		resticTimeout:          resticTimeout,
		defaultVolumesToRestic: defaultVolumesToRestic,
		clientPageSize:         clientPageSize,
	}, nil
}

func resolveActions(actions []velero.BackupItemAction, helper discovery.Helper) ([]resolvedAction, error) {
	var resolved []resolvedAction

	for _, action := range actions {
		resourceSelector, err := action.AppliesTo()
		if err != nil {
			return nil, err
		}

		resources := collections.GetResourceIncludesExcludes(helper, resourceSelector.IncludedResources, resourceSelector.ExcludedResources)
		namespaces := collections.NewIncludesExcludes().Includes(resourceSelector.IncludedNamespaces...).Excludes(resourceSelector.ExcludedNamespaces...)

		selector := labels.Everything()
		if resourceSelector.LabelSelector != "" {
			if selector, err = labels.Parse(resourceSelector.LabelSelector); err != nil {
				return nil, err
			}
		}

		res := resolvedAction{
			BackupItemAction:          action,
			resourceIncludesExcludes:  resources,
			namespaceIncludesExcludes: namespaces,
			selector:                  selector,
		}

		resolved = append(resolved, res)
	}

	return resolved, nil
}

// getNamespaceIncludesExcludes returns an IncludesExcludes list containing which namespaces to
// include and exclude from the backup.
func getNamespaceIncludesExcludes(backup *velerov1api.Backup) *collections.IncludesExcludes {
	return collections.NewIncludesExcludes().Includes(backup.Spec.IncludedNamespaces...).Excludes(backup.Spec.ExcludedNamespaces...)
}

func getResourceHooks(hookSpecs []velerov1api.BackupResourceHookSpec, discoveryHelper discovery.Helper) ([]hook.ResourceHook, error) {
	resourceHooks := make([]hook.ResourceHook, 0, len(hookSpecs))

	for _, s := range hookSpecs {
		h, err := getResourceHook(s, discoveryHelper)
		if err != nil {
			return []hook.ResourceHook{}, err
		}

		resourceHooks = append(resourceHooks, h)
	}

	return resourceHooks, nil
}

func getResourceHook(hookSpec velerov1api.BackupResourceHookSpec, discoveryHelper discovery.Helper) (hook.ResourceHook, error) {
	h := hook.ResourceHook{
		Name: hookSpec.Name,
		Selector: hook.ResourceHookSelector{
			Namespaces: collections.NewIncludesExcludes().Includes(hookSpec.IncludedNamespaces...).Excludes(hookSpec.ExcludedNamespaces...),
			Resources:  collections.GetResourceIncludesExcludes(discoveryHelper, hookSpec.IncludedResources, hookSpec.ExcludedResources),
		},
		Pre:  hookSpec.PreHooks,
		Post: hookSpec.PostHooks,
	}

	if hookSpec.LabelSelector != nil {
		labelSelector, err := metav1.LabelSelectorAsSelector(hookSpec.LabelSelector)
		if err != nil {
			return hook.ResourceHook{}, errors.WithStack(err)
		}
		h.Selector.LabelSelector = labelSelector
	}

	return h, nil
}

type VolumeSnapshotterGetter interface {
	GetVolumeSnapshotter(name string) (velero.VolumeSnapshotter, error)
}

// Backup backs up the items specified in the Backup, placing them in a gzip-compressed tar file
// written to backupFile. The finalized velerov1api.Backup is written to metadata. Any error that represents
// a complete backup failure is returned. Errors that constitute partial failures (i.e. failures to
// back up individual resources that don't prevent the backup from continuing to be processed) are logged
// to the backup log.
func (kb *kubernetesBackupper) Backup(log logrus.FieldLogger, backupRequest *Request, backupFile io.Writer, actions []velero.BackupItemAction, volumeSnapshotterGetter VolumeSnapshotterGetter) error {
	gzippedData := gzip.NewWriter(backupFile)
	defer gzippedData.Close()

	tw := tar.NewWriter(gzippedData)
	defer tw.Close()

	log.Info("Writing backup version file")
	if err := kb.writeBackupVersion(tw); err != nil {
		return errors.WithStack(err)
	}

	backupRequest.NamespaceIncludesExcludes = getNamespaceIncludesExcludes(backupRequest.Backup)
	log.Infof("Including namespaces: %s", backupRequest.NamespaceIncludesExcludes.IncludesString())
	log.Infof("Excluding namespaces: %s", backupRequest.NamespaceIncludesExcludes.ExcludesString())

	backupRequest.ResourceIncludesExcludes = collections.GetResourceIncludesExcludes(kb.discoveryHelper, backupRequest.Spec.IncludedResources, backupRequest.Spec.ExcludedResources)
	log.Infof("Including resources: %s", backupRequest.ResourceIncludesExcludes.IncludesString())
	log.Infof("Excluding resources: %s", backupRequest.ResourceIncludesExcludes.ExcludesString())
	log.Infof("Backing up all pod volumes using restic: %t", *backupRequest.Backup.Spec.DefaultVolumesToRestic)

	var err error
	backupRequest.ResourceHooks, err = getResourceHooks(backupRequest.Spec.Hooks.Resources, kb.discoveryHelper)
	if err != nil {
		return err
	}

	backupRequest.ResolvedActions, err = resolveActions(actions, kb.discoveryHelper)
	if err != nil {
		return err
	}

	backupRequest.BackedUpItems = map[itemKey]struct{}{}

	podVolumeTimeout := kb.resticTimeout
	if val := backupRequest.Annotations[velerov1api.PodVolumeOperationTimeoutAnnotation]; val != "" {
		parsed, err := time.ParseDuration(val)
		if err != nil {
			log.WithError(errors.WithStack(err)).Errorf("Unable to parse pod volume timeout annotation %s, using server value.", val)
		} else {
			podVolumeTimeout = parsed
		}
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), podVolumeTimeout)
	defer cancelFunc()

	var resticBackupper restic.Backupper
	if kb.resticBackupperFactory != nil {
		resticBackupper, err = kb.resticBackupperFactory.NewBackupper(ctx, backupRequest.Backup)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// set up a temp dir for the itemCollector to use to temporarily
	// store items as they're scraped from the API.
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return errors.Wrap(err, "error creating temp dir for backup")
	}
	defer os.RemoveAll(tempDir)

	collector := &itemCollector{
		log:                   log,
		backupRequest:         backupRequest,
		discoveryHelper:       kb.discoveryHelper,
		dynamicFactory:        kb.dynamicFactory,
		cohabitatingResources: cohabitatingResources(),
		dir:                   tempDir,
		pageSize:              kb.clientPageSize,
	}

	items := collector.getAllItems()
	log.WithField("progress", "").Infof("Collected %d items matching the backup spec from the Kubernetes API (actual number of items backed up may be more or less depending on velero.io/exclude-from-backup annotation, plugins returning additional related items to back up, etc.)", len(items))

	backupRequest.Status.Progress = &velerov1api.BackupProgress{TotalItems: len(items)}
	patch := fmt.Sprintf(`{"status":{"progress":{"totalItems":%d}}}`, len(items))
	if _, err := kb.backupClient.Backups(backupRequest.Namespace).Patch(context.TODO(), backupRequest.Name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		log.WithError(errors.WithStack((err))).Warn("Got error trying to update backup's status.progress.totalItems")
	}

	itemBackupper := &itemBackupper{
		backupRequest:           backupRequest,
		tarWriter:               tw,
		dynamicFactory:          kb.dynamicFactory,
		discoveryHelper:         kb.discoveryHelper,
		resticBackupper:         resticBackupper,
		resticSnapshotTracker:   newPVCSnapshotTracker(),
		volumeSnapshotterGetter: volumeSnapshotterGetter,
		itemHookHandler: &hook.DefaultItemHookHandler{
			PodCommandExecutor: kb.podCommandExecutor,
		},
	}

	// helper struct to send current progress between the main
	// backup loop and the gouroutine that periodically patches
	// the backup CR with progress updates
	type progressUpdate struct {
		totalItems, itemsBackedUp int
	}

	// the main backup process will send on this channel once
	// for every item it processes.
	update := make(chan progressUpdate)

	// the main backup process will send on this channel when
	// it's done sending progress updates
	quit := make(chan struct{})

	// This is the progress updater goroutine that receives
	// progress updates on the 'update' channel. It patches
	// the backup CR with progress updates at most every second,
	// but it will not issue a patch if it hasn't received a new
	// update since the previous patch. This goroutine exits
	// when it receives on the 'quit' channel.
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		var lastUpdate *progressUpdate
		for {
			select {
			case <-quit:
				ticker.Stop()
				return
			case val := <-update:
				lastUpdate = &val
			case <-ticker.C:
				if lastUpdate != nil {
					backupRequest.Status.Progress.TotalItems = lastUpdate.totalItems
					backupRequest.Status.Progress.ItemsBackedUp = lastUpdate.itemsBackedUp

					patch := fmt.Sprintf(`{"status":{"progress":{"totalItems":%d,"itemsBackedUp":%d}}}`, lastUpdate.totalItems, lastUpdate.itemsBackedUp)
					if _, err := kb.backupClient.Backups(backupRequest.Namespace).Patch(context.TODO(), backupRequest.Name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
						log.WithError(errors.WithStack((err))).Warn("Got error trying to update backup's status.progress")
					}
					lastUpdate = nil
				}
			}
		}
	}()

	backedUpGroupResources := map[schema.GroupResource]bool{}
	totalItems := len(items)

	for i, item := range items {
		log.WithFields(map[string]interface{}{
			"progress":  "",
			"resource":  item.groupResource.String(),
			"namespace": item.namespace,
			"name":      item.name,
		}).Infof("Processing item")

		// use an anonymous func so we can defer-close/remove the file
		// as soon as we're done with it
		func() {
			var unstructured unstructured.Unstructured

			f, err := os.Open(item.path)
			if err != nil {
				log.WithError(errors.WithStack(err)).Error("Error opening file containing item")
				return
			}
			defer f.Close()
			defer os.Remove(f.Name())

			if err := json.NewDecoder(f).Decode(&unstructured); err != nil {
				log.WithError(errors.WithStack(err)).Error("Error decoding JSON from file")
				return
			}

			if backedUp := kb.backupItem(log, item.groupResource, itemBackupper, &unstructured, item.preferredGVR); backedUp {
				backedUpGroupResources[item.groupResource] = true
			}
		}()

		// updated total is computed as "how many items we've backed up so far, plus
		// how many items we know of that are remaining"
		totalItems = len(backupRequest.BackedUpItems) + (len(items) - (i + 1))

		// send a progress update
		update <- progressUpdate{
			totalItems:    totalItems,
			itemsBackedUp: len(backupRequest.BackedUpItems),
		}

		log.WithFields(map[string]interface{}{
			"progress":  "",
			"resource":  item.groupResource.String(),
			"namespace": item.namespace,
			"name":      item.name,
		}).Infof("Backed up %d items out of an estimated total of %d (estimate will change throughout the backup)", len(backupRequest.BackedUpItems), totalItems)
	}

	// no more progress updates will be sent on the 'update' channel
	quit <- struct{}{}

	// back up CRD for resource if found. We should only need to do this if we've backed up at least
	// one item for the resource and IncludeClusterResources is nil. If IncludeClusterResources is false
	// we don't want to back it up, and if it's true it will already be included.
	if backupRequest.Spec.IncludeClusterResources == nil {
		for gr := range backedUpGroupResources {
			kb.backupCRD(log, gr, itemBackupper)
		}
	}

	// do a final update on progress since we may have just added some CRDs and may not have updated
	// for the last few processed items.
	backupRequest.Status.Progress.TotalItems = len(backupRequest.BackedUpItems)
	backupRequest.Status.Progress.ItemsBackedUp = len(backupRequest.BackedUpItems)

	patch = fmt.Sprintf(`{"status":{"progress":{"totalItems":%d,"itemsBackedUp":%d}}}`, len(backupRequest.BackedUpItems), len(backupRequest.BackedUpItems))
	if _, err := kb.backupClient.Backups(backupRequest.Namespace).Patch(context.TODO(), backupRequest.Name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		log.WithError(errors.WithStack((err))).Warn("Got error trying to update backup's status.progress")
	}

	log.WithField("progress", "").Infof("Backed up a total of %d items", len(backupRequest.BackedUpItems))

	return nil
}

func (kb *kubernetesBackupper) backupItem(log logrus.FieldLogger, gr schema.GroupResource, itemBackupper *itemBackupper, unstructured *unstructured.Unstructured, preferredGVR schema.GroupVersionResource) bool {
	backedUpItem, err := itemBackupper.backupItem(log, unstructured, gr, preferredGVR)
	if aggregate, ok := err.(kubeerrs.Aggregate); ok {
		log.WithField("name", unstructured.GetName()).Infof("%d errors encountered backup up item", len(aggregate.Errors()))
		// log each error separately so we get error location info in the log, and an
		// accurate count of errors
		for _, err = range aggregate.Errors() {
			log.WithError(err).WithField("name", unstructured.GetName()).Error("Error backing up item")
		}

		return false
	}
	if err != nil {
		log.WithError(err).WithField("name", unstructured.GetName()).Error("Error backing up item")
		return false
	}
	return backedUpItem
}

// backupCRD checks if the resource is a custom resource, and if so, backs up the custom resource definition
// associated with it.
func (kb *kubernetesBackupper) backupCRD(log logrus.FieldLogger, gr schema.GroupResource, itemBackupper *itemBackupper) {
	crdGroupResource := kuberesource.CustomResourceDefinitions

	log.Debugf("Getting server preferred API version for %s", crdGroupResource)
	gvr, apiResource, err := kb.discoveryHelper.ResourceFor(crdGroupResource.WithVersion(""))
	if err != nil {
		log.WithError(errors.WithStack(err)).Errorf("Error getting resolved resource for %s", crdGroupResource)
		return
	}
	log.Debugf("Got server preferred API version %s for %s", gvr.Version, crdGroupResource)

	log.Debugf("Getting dynamic client for %s", gvr.String())
	crdClient, err := kb.dynamicFactory.ClientForGroupVersionResource(gvr.GroupVersion(), apiResource, "")
	if err != nil {
		log.WithError(errors.WithStack(err)).Errorf("Error getting dynamic client for %s", crdGroupResource)
		return
	}
	log.Debugf("Got dynamic client for %s", gvr.String())

	// try to get a CRD whose name matches the provided GroupResource
	unstructured, err := crdClient.Get(gr.String(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		// not found: this means the GroupResource provided was not a
		// custom resource, so there's no CRD to back up.
		log.Debugf("No CRD found for GroupResource %s", gr.String())
		return
	}
	if err != nil {
		log.WithError(errors.WithStack(err)).Errorf("Error getting CRD %s", gr.String())
		return
	}
	log.Infof("Found associated CRD %s to add to backup", gr.String())

	kb.backupItem(log, gvr.GroupResource(), itemBackupper, unstructured, gvr)
}

func (kb *kubernetesBackupper) writeBackupVersion(tw *tar.Writer) error {
	versionFile := filepath.Join(velerov1api.MetadataDir, "version")
	versionString := fmt.Sprintf("%s\n", BackupFormatVersion)

	hdr := &tar.Header{
		Name:     versionFile,
		Size:     int64(len(versionString)),
		Typeflag: tar.TypeReg,
		Mode:     0755,
		ModTime:  time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return errors.WithStack(err)
	}
	if _, err := tw.Write([]byte(versionString)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type tarWriter interface {
	io.Closer
	Write([]byte) (int, error)
	WriteHeader(*tar.Header) error
}
