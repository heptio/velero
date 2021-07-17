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

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1api "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	kubeerrs "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	snapshotv1beta1client "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	snapshotv1beta1informers "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions"
	snapshotv1beta1listers "github.com/kubernetes-csi/external-snapshotter/client/v4/listers/volumesnapshot/v1beta1"

	"github.com/vmware-tanzu/velero/internal/credentials"
	"github.com/vmware-tanzu/velero/pkg/backup"
	"github.com/vmware-tanzu/velero/pkg/buildinfo"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/signals"

	"github.com/vmware-tanzu/velero/pkg/controller"
	velerodiscovery "github.com/vmware-tanzu/velero/pkg/discovery"
	"github.com/vmware-tanzu/velero/pkg/features"
	clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero/pkg/metrics"
	"github.com/vmware-tanzu/velero/pkg/persistence"
	"github.com/vmware-tanzu/velero/pkg/plugin/clientmgmt"
	"github.com/vmware-tanzu/velero/pkg/podexec"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/restore"
	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
	"github.com/vmware-tanzu/velero/pkg/util/logging"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/velero/internal/storage"
	"github.com/vmware-tanzu/velero/internal/util/managercontroller"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

const (
	// the port where prometheus metrics are exposed
	defaultMetricsAddress = ":8085"

	defaultBackupSyncPeriod           = time.Minute
	defaultStoreValidationFrequency   = time.Minute
	defaultPodVolumeOperationTimeout  = 240 * time.Minute
	defaultResourceTerminatingTimeout = 10 * time.Minute

	// server's client default qps and burst
	defaultClientQPS   float32 = 20.0
	defaultClientBurst int     = 30

	defaultProfilerAddress = "localhost:6060"

	defaultControllerWorkers = 1
	// the default TTL for a backup
	defaultBackupTTL = 30 * 24 * time.Hour

	// defaultCredentialsDirectory is the path on disk where credential
	// files will be written to
	defaultCredentialsDirectory = "/tmp/credentials"
)

type serverConfig struct {
	// TODO(2.0) Deprecate defaultBackupLocation
	pluginDir, metricsAddress, defaultBackupLocation                        string
	backupSyncPeriod, podVolumeOperationTimeout, resourceTerminatingTimeout time.Duration
	defaultBackupTTL, storeValidationFrequency                              time.Duration
	restoreResourcePriorities                                               []string
	defaultVolumeSnapshotLocations                                          map[string]string
	restoreOnly                                                             bool
	disabledControllers                                                     []string
	clientQPS                                                               float32
	clientBurst                                                             int
	profilerAddress                                                         string
	formatFlag                                                              *logging.FormatFlag
	defaultResticMaintenanceFrequency                                       time.Duration
	defaultVolumesToRestic                                                  bool
}

type controllerRunInfo struct {
	controller controller.Interface
	numWorkers int
}

func NewCommand(f client.Factory) *cobra.Command {
	var (
		volumeSnapshotLocations = flag.NewMap().WithKeyValueDelimiter(":")
		logLevelFlag            = logging.LogLevelFlag(logrus.InfoLevel)
		config                  = serverConfig{
			pluginDir:                         "/plugins",
			metricsAddress:                    defaultMetricsAddress,
			defaultBackupLocation:             "default",
			defaultVolumeSnapshotLocations:    make(map[string]string),
			backupSyncPeriod:                  defaultBackupSyncPeriod,
			defaultBackupTTL:                  defaultBackupTTL,
			storeValidationFrequency:          defaultStoreValidationFrequency,
			podVolumeOperationTimeout:         defaultPodVolumeOperationTimeout,
			restoreResourcePriorities:         defaultRestorePriorities,
			clientQPS:                         defaultClientQPS,
			clientBurst:                       defaultClientBurst,
			profilerAddress:                   defaultProfilerAddress,
			resourceTerminatingTimeout:        defaultResourceTerminatingTimeout,
			formatFlag:                        logging.NewFormatFlag(),
			defaultResticMaintenanceFrequency: restic.DefaultMaintenanceFrequency,
			defaultVolumesToRestic:            restic.DefaultVolumesToRestic,
		}
	)

	var command = &cobra.Command{
		Use:    "server",
		Short:  "Run the velero server",
		Long:   "Run the velero server",
		Hidden: true,
		Run: func(c *cobra.Command, args []string) {
			// go-plugin uses log.Println to log when it's waiting for all plugin processes to complete so we need to
			// set its output to stdout.
			log.SetOutput(os.Stdout)

			logLevel := logLevelFlag.Parse()
			format := config.formatFlag.Parse()

			// Make sure we log to stdout so cloud log dashboards don't show this as an error.
			logrus.SetOutput(os.Stdout)

			// Velero's DefaultLogger logs to stdout, so all is good there.
			logger := logging.DefaultLogger(logLevel, format)

			logger.Infof("setting log-level to %s", strings.ToUpper(logLevel.String()))

			logger.Infof("Starting Velero server %s (%s)", buildinfo.Version, buildinfo.FormattedGitSHA())
			if len(features.All()) > 0 {
				logger.Infof("%d feature flags enabled %s", len(features.All()), features.All())
			} else {
				logger.Info("No feature flags enabled")
			}

			if volumeSnapshotLocations.Data() != nil {
				config.defaultVolumeSnapshotLocations = volumeSnapshotLocations.Data()
			}

			f.SetBasename(fmt.Sprintf("%s-%s", c.Parent().Name(), c.Name()))

			s, err := newServer(f, config, logger)
			cmd.CheckError(err)

			cmd.CheckError(s.run())
		},
	}

	command.Flags().Var(logLevelFlag, "log-level", fmt.Sprintf("The level at which to log. Valid values are %s.", strings.Join(logLevelFlag.AllowedValues(), ", ")))
	command.Flags().Var(config.formatFlag, "log-format", fmt.Sprintf("The format for log output. Valid values are %s.", strings.Join(config.formatFlag.AllowedValues(), ", ")))
	command.Flags().StringVar(&config.pluginDir, "plugin-dir", config.pluginDir, "Directory containing Velero plugins")
	command.Flags().StringVar(&config.metricsAddress, "metrics-address", config.metricsAddress, "The address to expose prometheus metrics")
	command.Flags().DurationVar(&config.backupSyncPeriod, "backup-sync-period", config.backupSyncPeriod, "How often to ensure all Velero backups in object storage exist as Backup API objects in the cluster. This is the default sync period if none is explicitly specified for a backup storage location.")
	command.Flags().DurationVar(&config.podVolumeOperationTimeout, "restic-timeout", config.podVolumeOperationTimeout, "How long backups/restores of pod volumes should be allowed to run before timing out.")
	command.Flags().BoolVar(&config.restoreOnly, "restore-only", config.restoreOnly, "Run in a mode where only restores are allowed; backups, schedules, and garbage-collection are all disabled. DEPRECATED: this flag will be removed in v2.0. Use read-only backup storage locations instead.")
	command.Flags().StringSliceVar(&config.disabledControllers, "disable-controllers", config.disabledControllers, fmt.Sprintf("List of controllers to disable on startup. Valid values are %s", strings.Join(controller.DisableableControllers, ",")))
	command.Flags().StringSliceVar(&config.restoreResourcePriorities, "restore-resource-priorities", config.restoreResourcePriorities, "Desired order of resource restores; any resource not in the list will be restored alphabetically after the prioritized resources.")
	command.Flags().StringVar(&config.defaultBackupLocation, "default-backup-storage-location", config.defaultBackupLocation, "Name of the default backup storage location. DEPRECATED: this flag will be removed in v2.0. Use \"velero backup-location set --default\" instead.")
	command.Flags().DurationVar(&config.storeValidationFrequency, "store-validation-frequency", config.storeValidationFrequency, "How often to verify if the storage is valid. Optional. Set this to `0s` to disable sync. Default 1 minute.")
	command.Flags().Var(&volumeSnapshotLocations, "default-volume-snapshot-locations", "List of unique volume providers and default volume snapshot location (provider1:location-01,provider2:location-02,...)")
	command.Flags().Float32Var(&config.clientQPS, "client-qps", config.clientQPS, "Maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached.")
	command.Flags().IntVar(&config.clientBurst, "client-burst", config.clientBurst, "Maximum number of requests by the server to the Kubernetes API in a short period of time.")
	command.Flags().StringVar(&config.profilerAddress, "profiler-address", config.profilerAddress, "The address to expose the pprof profiler.")
	command.Flags().DurationVar(&config.resourceTerminatingTimeout, "terminating-resource-timeout", config.resourceTerminatingTimeout, "How long to wait on persistent volumes and namespaces to terminate during a restore before timing out.")
	command.Flags().DurationVar(&config.defaultBackupTTL, "default-backup-ttl", config.defaultBackupTTL, "How long to wait by default before backups can be garbage collected.")
	command.Flags().DurationVar(&config.defaultResticMaintenanceFrequency, "default-restic-prune-frequency", config.defaultResticMaintenanceFrequency, "How often 'restic prune' is run for restic repositories by default.")
	command.Flags().BoolVar(&config.defaultVolumesToRestic, "default-volumes-to-restic", config.defaultVolumesToRestic, "Backup all volumes with restic by default.")

	return command
}

type server struct {
	namespace                           string
	metricsAddress                      string
	kubeClientConfig                    *rest.Config
	kubeClient                          kubernetes.Interface
	veleroClient                        clientset.Interface
	discoveryClient                     discovery.DiscoveryInterface
	discoveryHelper                     velerodiscovery.Helper
	dynamicClient                       dynamic.Interface
	sharedInformerFactory               informers.SharedInformerFactory
	csiSnapshotterSharedInformerFactory *CSIInformerFactoryWrapper
	csiSnapshotClient                   *snapshotv1beta1client.Clientset
	ctx                                 context.Context
	cancelFunc                          context.CancelFunc
	logger                              logrus.FieldLogger
	logLevel                            logrus.Level
	pluginRegistry                      clientmgmt.Registry
	resticManager                       restic.RepositoryManager
	metrics                             *metrics.ServerMetrics
	config                              serverConfig
	mgr                                 manager.Manager
	credentialFileStore                 credentials.FileStore
}

func newServer(f client.Factory, config serverConfig, logger *logrus.Logger) (*server, error) {
	if config.clientQPS < 0.0 {
		return nil, errors.New("client-qps must be positive")
	}
	f.SetClientQPS(config.clientQPS)

	if config.clientBurst <= 0 {
		return nil, errors.New("client-burst must be positive")
	}
	f.SetClientBurst(config.clientBurst)

	kubeClient, err := f.KubeClient()
	if err != nil {
		return nil, err
	}

	veleroClient, err := f.Client()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return nil, err
	}

	pluginRegistry := clientmgmt.NewRegistry(config.pluginDir, logger, logger.Level)
	if err := pluginRegistry.DiscoverPlugins(); err != nil {
		return nil, err
	}

	// cancelFunc is not deferred here because if it was, then ctx would immediately
	// be cancelled once this function exited, making it useless to any informers using later.
	// That, in turn, causes the velero server to halt when the first informer tries to use it (probably restic's).
	// Therefore, we must explicitly call it on the error paths in this function.
	ctx, cancelFunc := context.WithCancel(context.Background())

	clientConfig, err := f.ClientConfig()
	if err != nil {
		cancelFunc()
		return nil, err
	}

	var csiSnapClient *snapshotv1beta1client.Clientset
	if features.IsEnabled(velerov1api.CSIFeatureFlag) {
		csiSnapClient, err = snapshotv1beta1client.NewForConfig(clientConfig)
		if err != nil {
			cancelFunc()
			return nil, err
		}
	}

	scheme := runtime.NewScheme()
	velerov1api.AddToScheme(scheme)
	corev1api.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(clientConfig, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		cancelFunc()
		return nil, err
	}

	credentialFileStore, err := credentials.NewNamespacedFileStore(
		mgr.GetClient(),
		f.Namespace(),
		defaultCredentialsDirectory,
		filesystem.NewFileSystem(),
	)
	if err != nil {
		cancelFunc()
		return nil, err
	}

	s := &server{
		namespace:                           f.Namespace(),
		metricsAddress:                      config.metricsAddress,
		kubeClientConfig:                    clientConfig,
		kubeClient:                          kubeClient,
		veleroClient:                        veleroClient,
		discoveryClient:                     veleroClient.Discovery(),
		dynamicClient:                       dynamicClient,
		sharedInformerFactory:               informers.NewSharedInformerFactoryWithOptions(veleroClient, 0, informers.WithNamespace(f.Namespace())),
		csiSnapshotterSharedInformerFactory: NewCSIInformerFactoryWrapper(csiSnapClient),
		csiSnapshotClient:                   csiSnapClient,
		ctx:                                 ctx,
		cancelFunc:                          cancelFunc,
		logger:                              logger,
		logLevel:                            logger.Level,
		pluginRegistry:                      pluginRegistry,
		config:                              config,
		mgr:                                 mgr,
		credentialFileStore:                 credentialFileStore,
	}

	return s, nil
}

func (s *server) run() error {
	signals.CancelOnShutdown(s.cancelFunc, s.logger)

	if s.config.profilerAddress != "" {
		go s.runProfiler()
	}

	// Since s.namespace, which specifies where backups/restores/schedules/etc. should live,
	// *could* be different from the namespace where the Velero server pod runs, check to make
	// sure it exists, and fail fast if it doesn't.
	if err := s.namespaceExists(s.namespace); err != nil {
		return err
	}

	if err := s.initDiscoveryHelper(); err != nil {
		return err
	}

	if err := s.veleroResourcesExist(); err != nil {
		return err
	}

	if err := s.initRestic(); err != nil {
		return err
	}

	if err := s.runControllers(s.config.defaultVolumeSnapshotLocations); err != nil {
		return err
	}

	return nil
}

// namespaceExists returns nil if namespace can be successfully
// gotten from the kubernetes API, or an error otherwise.
func (s *server) namespaceExists(namespace string) error {
	s.logger.WithField("namespace", namespace).Info("Checking existence of namespace.")

	if _, err := s.kubeClient.CoreV1().Namespaces().Get(s.ctx, namespace, metav1.GetOptions{}); err != nil {
		return errors.WithStack(err)
	}

	s.logger.WithField("namespace", namespace).Info("Namespace exists")
	return nil
}

// initDiscoveryHelper instantiates the server's discovery helper and spawns a
// goroutine to call Refresh() every 5 minutes.
func (s *server) initDiscoveryHelper() error {
	discoveryHelper, err := velerodiscovery.NewHelper(s.discoveryClient, s.logger)
	if err != nil {
		return err
	}
	s.discoveryHelper = discoveryHelper

	go wait.Until(
		func() {
			if err := discoveryHelper.Refresh(); err != nil {
				s.logger.WithError(err).Error("Error refreshing discovery")
			}
		},
		5*time.Minute,
		s.ctx.Done(),
	)

	return nil
}

// veleroResourcesExist checks for the existence of each Velero CRD via discovery
// and returns an error if any of them don't exist.
func (s *server) veleroResourcesExist() error {
	s.logger.Info("Checking existence of Velero custom resource definitions")

	var veleroGroupVersion *metav1.APIResourceList
	for _, gv := range s.discoveryHelper.Resources() {
		if gv.GroupVersion == velerov1api.SchemeGroupVersion.String() {
			veleroGroupVersion = gv
			break
		}
	}

	if veleroGroupVersion == nil {
		return errors.Errorf("Velero API group %s not found. Apply examples/common/00-prereqs.yaml to create it.", velerov1api.SchemeGroupVersion)
	}

	foundResources := sets.NewString()
	for _, resource := range veleroGroupVersion.APIResources {
		foundResources.Insert(resource.Kind)
	}

	var errs []error
	for kind := range velerov1api.CustomResources() {
		if foundResources.Has(kind) {
			s.logger.WithField("kind", kind).Debug("Found custom resource")
			continue
		}

		errs = append(errs, errors.Errorf("custom resource %s not found in Velero API group %s", kind, velerov1api.SchemeGroupVersion))
	}

	if len(errs) > 0 {
		errs = append(errs, errors.New("Velero custom resources not found - apply examples/common/00-prereqs.yaml to update the custom resource definitions"))
		return kubeerrs.NewAggregate(errs)
	}

	s.logger.Info("All Velero custom resource definitions exist")
	return nil
}

// - Custom Resource Definitions come before Custom Resource so that they can be
//   restored with their corresponding CRD.
// - Namespaces go second because all namespaced resources depend on them.
// - Storage Classes are needed to create PVs and PVCs correctly.
// - VolumeSnapshotClasses  are needed to provision volumes using volumesnapshots
// - VolumeSnapshotContents are needed as they contain the handle to the volume snapshot in the
//	 storage provider
// - VolumeSnapshots are needed to create PVCs using the VolumeSnapshot as their data source.
// - PVs go before PVCs because PVCs depend on them.
// - PVCs go before pods or controllers so they can be mounted as volumes.
// - Secrets and config maps go before pods or controllers so they can be mounted
// 	 as volumes.
// - Service accounts go before pods or controllers so pods can use them.
// - Limit ranges go before pods or controllers so pods can use them.
// - Pods go before controllers so they can be explicitly restored and potentially
//	 have restic restores run before controllers adopt the pods.
// - Replica sets go before deployments/other controllers so they can be explicitly
//	 restored and be adopted by controllers.
// - CAPI Clusters come before ClusterResourceSets because failing to do so means the CAPI controller-manager will panic.
//	 Both Clusters and ClusterResourceSets need to come before ClusterResourceSetBinding in order to properly restore workload clusters.
//   See https://github.com/kubernetes-sigs/cluster-api/issues/4105
var defaultRestorePriorities = []string{
	"customresourcedefinitions",
	"namespaces",
	"storageclasses",
	"volumesnapshotclass.snapshot.storage.k8s.io",
	"volumesnapshotcontents.snapshot.storage.k8s.io",
	"volumesnapshots.snapshot.storage.k8s.io",
	"persistentvolumes",
	"persistentvolumeclaims",
	"secrets",
	"configmaps",
	"serviceaccounts",
	"limitranges",
	"pods",
	// we fully qualify replicasets.apps because prior to Kubernetes 1.16, replicasets also
	// existed in the extensions API group, but we back up replicasets from "apps" so we want
	// to ensure that we prioritize restoring from "apps" too, since this is how they're stored
	// in the backup.
	"replicasets.apps",
	"clusters.cluster.x-k8s.io",
	"clusterresourcesets.addons.cluster.x-k8s.io",
}

func (s *server) initRestic() error {
	// warn if restic daemonset does not exist
	if _, err := s.kubeClient.AppsV1().DaemonSets(s.namespace).Get(s.ctx, restic.DaemonSet, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		s.logger.Warn("Velero restic daemonset not found; restic backups/restores will not work until it's created")
	} else if err != nil {
		s.logger.WithError(errors.WithStack(err)).Warn("Error checking for existence of velero restic daemonset")
	}

	// ensure the repo key secret is set up
	if err := restic.EnsureCommonRepositoryKey(s.kubeClient.CoreV1(), s.namespace); err != nil {
		return err
	}

	res, err := restic.NewRepositoryManager(
		s.ctx,
		s.namespace,
		s.veleroClient,
		s.sharedInformerFactory.Velero().V1().ResticRepositories(),
		s.veleroClient.VeleroV1(),
		s.mgr.GetClient(),
		s.kubeClient.CoreV1(),
		s.kubeClient.CoreV1(),
		s.credentialFileStore,
		s.logger,
	)
	if err != nil {
		return err
	}
	s.resticManager = res

	return nil
}

func (s *server) getCSISnapshotListers() (snapshotv1beta1listers.VolumeSnapshotLister, snapshotv1beta1listers.VolumeSnapshotContentLister) {
	// Make empty listers that will only be populated if CSI is properly enabled.
	var vsLister snapshotv1beta1listers.VolumeSnapshotLister
	var vscLister snapshotv1beta1listers.VolumeSnapshotContentLister
	var err error

	// If CSI is enabled, check for the CSI groups and generate the listers
	// If CSI isn't enabled, return empty listers.
	if features.IsEnabled(velerov1api.CSIFeatureFlag) {
		_, err = s.discoveryClient.ServerResourcesForGroupVersion(snapshotv1beta1api.SchemeGroupVersion.String())
		switch {
		case apierrors.IsNotFound(err):
			// CSI is enabled, but the required CRDs aren't installed, so halt.
			s.logger.Fatalf("The '%s' feature flag was specified, but CSI API group [%s] was not found.", velerov1api.CSIFeatureFlag, snapshotv1beta1api.SchemeGroupVersion.String())
		case err == nil:
			// CSI is enabled, and the resources were found.
			// Instantiate the listers fully
			s.logger.Debug("Creating CSI listers")
			// Access the wrapped factory directly here since we've already done the feature flag check above to know it's safe.
			vsLister = s.csiSnapshotterSharedInformerFactory.factory.Snapshot().V1beta1().VolumeSnapshots().Lister()
			vscLister = s.csiSnapshotterSharedInformerFactory.factory.Snapshot().V1beta1().VolumeSnapshotContents().Lister()
		case err != nil:
			cmd.CheckError(err)
		}
	}
	return vsLister, vscLister
}

func (s *server) runControllers(defaultVolumeSnapshotLocations map[string]string) error {
	s.logger.Info("Starting controllers")

	ctx := s.ctx

	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		s.logger.Infof("Starting metric server at address [%s]", s.metricsAddress)
		if err := http.ListenAndServe(s.metricsAddress, metricsMux); err != nil {
			s.logger.Fatalf("Failed to start metric server at [%s]: %v", s.metricsAddress, err)
		}
	}()
	s.metrics = metrics.NewServerMetrics()
	s.metrics.RegisterAllMetrics()
	// Initialize manual backup metrics
	s.metrics.InitSchedule("")

	newPluginManager := func(logger logrus.FieldLogger) clientmgmt.Manager {
		return clientmgmt.NewManager(logger, s.logLevel, s.pluginRegistry)
	}

	backupStoreGetter := persistence.NewObjectBackupStoreGetter(s.credentialFileStore)

	csiVSLister, csiVSCLister := s.getCSISnapshotListers()

	backupSyncControllerRunInfo := func() controllerRunInfo {
		backupSyncContoller := controller.NewBackupSyncController(
			s.veleroClient.VeleroV1(),
			s.mgr.GetClient(),
			s.veleroClient.VeleroV1(),
			s.sharedInformerFactory.Velero().V1().Backups().Lister(),
			s.config.backupSyncPeriod,
			s.namespace,
			s.csiSnapshotClient,
			s.kubeClient,
			s.config.defaultBackupLocation,
			newPluginManager,
			backupStoreGetter,
			s.logger,
		)

		return controllerRunInfo{
			controller: backupSyncContoller,
			numWorkers: defaultControllerWorkers,
		}
	}

	backupTracker := controller.NewBackupTracker()

	backupControllerRunInfo := func() controllerRunInfo {
		backupper, err := backup.NewKubernetesBackupper(
			s.veleroClient.VeleroV1(),
			s.discoveryHelper,
			client.NewDynamicFactory(s.dynamicClient),
			podexec.NewPodCommandExecutor(s.kubeClientConfig, s.kubeClient.CoreV1().RESTClient()),
			s.resticManager,
			s.config.podVolumeOperationTimeout,
			s.config.defaultVolumesToRestic,
		)
		cmd.CheckError(err)

		backupController := controller.NewBackupController(
			s.sharedInformerFactory.Velero().V1().Backups(),
			s.veleroClient.VeleroV1(),
			s.discoveryHelper,
			backupper,
			s.logger,
			s.logLevel,
			newPluginManager,
			backupTracker,
			s.mgr.GetClient(),
			s.config.defaultBackupLocation,
			s.config.defaultVolumesToRestic,
			s.config.defaultBackupTTL,
			s.sharedInformerFactory.Velero().V1().VolumeSnapshotLocations().Lister(),
			defaultVolumeSnapshotLocations,
			s.metrics,
			s.config.formatFlag.Parse(),
			csiVSLister,
			csiVSCLister,
			backupStoreGetter,
		)

		return controllerRunInfo{
			controller: backupController,
			numWorkers: defaultControllerWorkers,
		}
	}

	scheduleControllerRunInfo := func() controllerRunInfo {
		scheduleController := controller.NewScheduleController(
			s.namespace,
			s.veleroClient.VeleroV1(),
			s.veleroClient.VeleroV1(),
			s.sharedInformerFactory.Velero().V1().Schedules(),
			s.logger,
			s.metrics,
		)

		return controllerRunInfo{
			controller: scheduleController,
			numWorkers: defaultControllerWorkers,
		}
	}

	gcControllerRunInfo := func() controllerRunInfo {
		gcController := controller.NewGCController(
			s.logger,
			s.sharedInformerFactory.Velero().V1().Backups(),
			s.sharedInformerFactory.Velero().V1().DeleteBackupRequests().Lister(),
			s.veleroClient.VeleroV1(),
			s.mgr.GetClient(),
		)

		return controllerRunInfo{
			controller: gcController,
			numWorkers: defaultControllerWorkers,
		}
	}

	deletionControllerRunInfo := func() controllerRunInfo {
		deletionController := controller.NewBackupDeletionController(
			s.logger,
			s.sharedInformerFactory.Velero().V1().DeleteBackupRequests(),
			s.veleroClient.VeleroV1(), // deleteBackupRequestClient
			s.veleroClient.VeleroV1(), // backupClient
			s.sharedInformerFactory.Velero().V1().Restores().Lister(),
			s.veleroClient.VeleroV1(), // restoreClient
			backupTracker,
			s.resticManager,
			s.sharedInformerFactory.Velero().V1().PodVolumeBackups().Lister(),
			s.mgr.GetClient(),
			s.sharedInformerFactory.Velero().V1().VolumeSnapshotLocations().Lister(),
			csiVSLister,
			csiVSCLister,
			s.csiSnapshotClient,
			newPluginManager,
			backupStoreGetter,
			s.metrics,
			s.discoveryHelper,
		)

		return controllerRunInfo{
			controller: deletionController,
			numWorkers: defaultControllerWorkers,
		}
	}

	restoreControllerRunInfo := func() controllerRunInfo {
		restorer, err := restore.NewKubernetesRestorer(
			s.veleroClient.VeleroV1(),
			s.discoveryHelper,
			client.NewDynamicFactory(s.dynamicClient),
			s.config.restoreResourcePriorities,
			s.kubeClient.CoreV1().Namespaces(),
			s.resticManager,
			s.config.podVolumeOperationTimeout,
			s.config.resourceTerminatingTimeout,
			s.logger,
			podexec.NewPodCommandExecutor(s.kubeClientConfig, s.kubeClient.CoreV1().RESTClient()),
			s.kubeClient.CoreV1().RESTClient(),
		)
		cmd.CheckError(err)

		restoreController := controller.NewRestoreController(
			s.namespace,
			s.sharedInformerFactory.Velero().V1().Restores(),
			s.veleroClient.VeleroV1(),
			s.veleroClient.VeleroV1(),
			restorer,
			s.sharedInformerFactory.Velero().V1().Backups().Lister(),
			s.mgr.GetClient(),
			s.sharedInformerFactory.Velero().V1().VolumeSnapshotLocations().Lister(),
			s.logger,
			s.logLevel,
			newPluginManager,
			backupStoreGetter,
			s.metrics,
			s.config.formatFlag.Parse(),
		)

		return controllerRunInfo{
			controller: restoreController,
			numWorkers: defaultControllerWorkers,
		}
	}

	resticRepoControllerRunInfo := func() controllerRunInfo {
		resticRepoController := controller.NewResticRepositoryController(
			s.logger,
			s.sharedInformerFactory.Velero().V1().ResticRepositories(),
			s.veleroClient.VeleroV1(),
			s.mgr.GetClient(),
			s.resticManager,
			s.config.defaultResticMaintenanceFrequency,
		)

		return controllerRunInfo{
			controller: resticRepoController,
			numWorkers: defaultControllerWorkers,
		}
	}

	enabledControllers := map[string]func() controllerRunInfo{
		controller.BackupSync:        backupSyncControllerRunInfo,
		controller.Backup:            backupControllerRunInfo,
		controller.Schedule:          scheduleControllerRunInfo,
		controller.GarbageCollection: gcControllerRunInfo,
		controller.BackupDeletion:    deletionControllerRunInfo,
		controller.Restore:           restoreControllerRunInfo,
		controller.ResticRepo:        resticRepoControllerRunInfo,
	}
	// Note: all runtime type controllers that can be disabled are grouped separately, below:
	enabledRuntimeControllers := make(map[string]struct{})
	enabledRuntimeControllers[controller.ServerStatusRequest] = struct{}{}
	enabledRuntimeControllers[controller.DownloadRequest] = struct{}{}

	if s.config.restoreOnly {
		s.logger.Info("Restore only mode - not starting the backup, schedule, delete-backup, or GC controllers")
		s.config.disabledControllers = append(s.config.disabledControllers,
			controller.Backup,
			controller.Schedule,
			controller.GarbageCollection,
			controller.BackupDeletion,
		)
	}

	// Remove disabled controllers so they are not initialized. If a match is not found we want
	// to hault the system so the user knows this operation was not possible.
	if err := removeControllers(s.config.disabledControllers, enabledControllers, enabledRuntimeControllers, s.logger); err != nil {
		log.Fatal(err, "unable to disable a controller")
	}

	// Instantiate the enabled controllers. This needs to be done *before*
	// the shared informer factory is started, because the controller
	// constructors add event handlers to various informers, which should
	// be done before the informers are running.
	controllers := make([]controllerRunInfo, 0, len(enabledControllers))
	for _, newController := range enabledControllers {
		controllers = append(controllers, newController())
	}

	// start the informers & and wait for the caches to sync
	s.sharedInformerFactory.Start(ctx.Done())
	s.csiSnapshotterSharedInformerFactory.Start(ctx.Done())
	s.logger.Info("Waiting for informer caches to sync")
	cacheSyncResults := s.sharedInformerFactory.WaitForCacheSync(ctx.Done())
	csiCacheSyncResults := s.csiSnapshotterSharedInformerFactory.WaitForCacheSync(ctx.Done())
	s.logger.Info("Done waiting for informer caches to sync")

	// Append our CSI informer types into the larger list of caches, so we can check them all at once
	for informer, synced := range csiCacheSyncResults {
		cacheSyncResults[informer] = synced
	}

	for informer, synced := range cacheSyncResults {
		if !synced {
			return errors.Errorf("cache was not synced for informer %v", informer)
		}
		s.logger.WithField("informer", informer).Info("Informer cache synced")
	}

	bslr := controller.BackupStorageLocationReconciler{
		Ctx:    s.ctx,
		Client: s.mgr.GetClient(),
		Scheme: s.mgr.GetScheme(),
		DefaultBackupLocationInfo: storage.DefaultBackupLocationInfo{
			StorageLocation:           s.config.defaultBackupLocation,
			ServerValidationFrequency: s.config.storeValidationFrequency,
		},
		NewPluginManager:  newPluginManager,
		BackupStoreGetter: backupStoreGetter,
		Log:               s.logger,
	}
	if err := bslr.SetupWithManager(s.mgr); err != nil {
		s.logger.Fatal(err, "unable to create controller", "controller", controller.BackupStorageLocation)
	}

	if _, ok := enabledRuntimeControllers[controller.ServerStatusRequest]; ok {
		r := controller.ServerStatusRequestReconciler{
			Scheme:         s.mgr.GetScheme(),
			Client:         s.mgr.GetClient(),
			Ctx:            s.ctx,
			PluginRegistry: s.pluginRegistry,
			Clock:          clock.RealClock{},
			Log:            s.logger,
		}
		if err := r.SetupWithManager(s.mgr); err != nil {
			s.logger.Fatal(err, "unable to create controller", "controller", controller.ServerStatusRequest)
		}
	}

	if _, ok := enabledRuntimeControllers[controller.DownloadRequest]; ok {
		r := controller.DownloadRequestReconciler{
			Scheme:            s.mgr.GetScheme(),
			Client:            s.mgr.GetClient(),
			Clock:             clock.RealClock{},
			NewPluginManager:  newPluginManager,
			BackupStoreGetter: backupStoreGetter,
			Log:               s.logger,
		}
		if err := r.SetupWithManager(s.mgr); err != nil {
			s.logger.Fatal(err, "unable to create controller", "controller", controller.DownloadRequest)
		}
	}

	// TODO(2.0): presuming all controllers and resources are converted to runtime-controller
	// by v2.0, the block from this line and including the `s.mgr.Start() will be
	// deprecated, since the manager auto-starts all the caches. Until then, we need to start the
	// cache for them manually.
	for i := range controllers {
		controllerRunInfo := controllers[i]
		// Adding the controllers to the manager will register them as a (runtime-controller) runnable,
		// so the manager will ensure the cache is started and ready before all controller are started
		s.mgr.Add(managercontroller.Runnable(controllerRunInfo.controller, controllerRunInfo.numWorkers))
	}

	s.logger.Info("Server starting...")

	if err := s.mgr.Start(s.ctx); err != nil {
		s.logger.Fatal("Problem starting manager", err)
	}

	return nil
}

// removeControllers will remove any controller listed to be disabled from the list
// of controllers to be initialized.  First it will check the legacy list of controllers,
// then it will check the new runtime controllers. If both checks fail a match
// wasn't found and it returns an error.
func removeControllers(disabledControllers []string, enabledControllers map[string]func() controllerRunInfo, enabledRuntimeControllers map[string]struct{}, logger logrus.FieldLogger) error {
	for _, controllerName := range disabledControllers {
		if _, ok := enabledControllers[controllerName]; ok {
			logger.Infof("Disabling controller: %s", controllerName)
			delete(enabledControllers, controllerName)
		} else {
			// maybe it is a runtime type controllers, so attempt to remove that
			if _, ok := enabledRuntimeControllers[controllerName]; ok {
				logger.Infof("Disabling controller: %s", controllerName)
				delete(enabledRuntimeControllers, controllerName)
			} else {
				msg := fmt.Sprintf("Invalid value for --disable-controllers flag provided: %s. Valid values are: %s", controllerName, strings.Join(controller.DisableableControllers, ","))
				return errors.New(msg)
			}
		}
	}
	return nil
}

func (s *server) runProfiler() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	if err := http.ListenAndServe(s.config.profilerAddress, mux); err != nil {
		s.logger.WithError(errors.WithStack(err)).Error("error running profiler http server")
	}
}

// CSIInformerFactoryWrapper is a proxy around the CSI SharedInformerFactory that checks the CSI feature flag before performing operations.
type CSIInformerFactoryWrapper struct {
	factory snapshotv1beta1informers.SharedInformerFactory
}

func NewCSIInformerFactoryWrapper(c snapshotv1beta1client.Interface) *CSIInformerFactoryWrapper {
	// If no namespace is specified, all namespaces are watched.
	// This is desirable for VolumeSnapshots, as we want to query for all VolumeSnapshots across all namespaces using this informer
	w := &CSIInformerFactoryWrapper{}

	if features.IsEnabled(velerov1api.CSIFeatureFlag) {
		w.factory = snapshotv1beta1informers.NewSharedInformerFactoryWithOptions(c, 0)
	}
	return w
}

// Start proxies the Start call to the CSI SharedInformerFactory.
func (w *CSIInformerFactoryWrapper) Start(stopCh <-chan struct{}) {
	if features.IsEnabled(velerov1api.CSIFeatureFlag) {
		w.factory.Start(stopCh)
	}
}

// WaitForCacheSync proxies the WaitForCacheSync call to the CSI SharedInformerFactory.
func (w *CSIInformerFactoryWrapper) WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool {
	if features.IsEnabled(velerov1api.CSIFeatureFlag) {
		return w.factory.WaitForCacheSync(stopCh)
	}
	return nil
}
