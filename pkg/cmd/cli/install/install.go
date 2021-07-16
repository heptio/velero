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

package install

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/vmware-tanzu/velero/internal/velero"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/install"
	kubeutil "github.com/vmware-tanzu/velero/pkg/util/kube"
)

// InstallOptions collects all the options for installing Velero into a Kubernetes cluster.
type InstallOptions struct {
	Namespace                         string
	Image                             string
	BucketName                        string
	Prefix                            string
	ProviderName                      string
	PodAnnotations                    flag.Map
	ServiceAccountAnnotations         flag.Map
	VeleroPodCPURequest               string
	VeleroPodMemRequest               string
	VeleroPodCPULimit                 string
	VeleroPodMemLimit                 string
	ResticPodCPURequest               string
	ResticPodMemRequest               string
	ResticPodCPULimit                 string
	ResticPodMemLimit                 string
	RestoreOnly                       bool
	SecretFile                        string
	NoSecret                          bool
	DryRun                            bool
	BackupStorageConfig               flag.Map
	VolumeSnapshotConfig              flag.Map
	UseRestic                         bool
	Wait                              bool
	UseVolumeSnapshots                bool
	DefaultResticMaintenanceFrequency time.Duration
	Plugins                           flag.StringArray
	NoDefaultBackupLocation           bool
	CRDsOnly                          bool
	CACertFile                        string
	Features                          string
	DefaultVolumesToRestic            bool
}

// BindFlags adds command line values to the options struct.
func (o *InstallOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ProviderName, "provider", o.ProviderName, "Provider name for backup and volume storage")
	flags.StringVar(&o.BucketName, "bucket", o.BucketName, "Name of the object storage bucket where backups should be stored")
	flags.StringVar(&o.SecretFile, "secret-file", o.SecretFile, "File containing credentials for backup and volume provider. If not specified, --no-secret must be used for confirmation. Optional.")
	flags.BoolVar(&o.NoSecret, "no-secret", o.NoSecret, "Flag indicating if a secret should be created. Must be used as confirmation if --secret-file is not provided. Optional.")
	flags.BoolVar(&o.NoDefaultBackupLocation, "no-default-backup-location", o.NoDefaultBackupLocation, "Flag indicating if a default backup location should be created. Must be used as confirmation if --bucket or --provider are not provided. Optional.")
	flags.StringVar(&o.Image, "image", o.Image, "Image to use for the Velero and restic server pods. Optional.")
	flags.StringVar(&o.Prefix, "prefix", o.Prefix, "Prefix under which all Velero data should be stored within the bucket. Optional.")
	flags.Var(&o.PodAnnotations, "pod-annotations", "Annotations to add to the Velero and restic pods. Optional. Format is key1=value1,key2=value2")
	flags.Var(&o.ServiceAccountAnnotations, "sa-annotations", "Annotations to add to the Velero ServiceAccount. Add iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_NAME].iam.gserviceaccount.com for workload identity. Optional. Format is key1=value1,key2=value2")
	flags.StringVar(&o.VeleroPodCPURequest, "velero-pod-cpu-request", o.VeleroPodCPURequest, `CPU request for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.VeleroPodMemRequest, "velero-pod-mem-request", o.VeleroPodMemRequest, `Memory request for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.VeleroPodCPULimit, "velero-pod-cpu-limit", o.VeleroPodCPULimit, `CPU limit for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.VeleroPodMemLimit, "velero-pod-mem-limit", o.VeleroPodMemLimit, `Memory limit for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.ResticPodCPURequest, "restic-pod-cpu-request", o.ResticPodCPURequest, `CPU request for restic pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.ResticPodMemRequest, "restic-pod-mem-request", o.ResticPodMemRequest, `Memory request for restic pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.ResticPodCPULimit, "restic-pod-cpu-limit", o.ResticPodCPULimit, `CPU limit for restic pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.ResticPodMemLimit, "restic-pod-mem-limit", o.ResticPodMemLimit, `Memory limit for restic pod. A value of "0" is treated as unbounded. Optional.`)
	flags.Var(&o.BackupStorageConfig, "backup-location-config", "Configuration to use for the backup storage location. Format is key1=value1,key2=value2")
	flags.Var(&o.VolumeSnapshotConfig, "snapshot-location-config", "Configuration to use for the volume snapshot location. Format is key1=value1,key2=value2")
	flags.BoolVar(&o.UseVolumeSnapshots, "use-volume-snapshots", o.UseVolumeSnapshots, "Whether or not to create snapshot location automatically. Set to false if you do not plan to create volume snapshots via a storage provider.")
	flags.BoolVar(&o.RestoreOnly, "restore-only", o.RestoreOnly, "Run the server in restore-only mode. Optional.")
	flags.BoolVar(&o.DryRun, "dry-run", o.DryRun, "Generate resources, but don't send them to the cluster. Use with -o. Optional.")
	flags.BoolVar(&o.UseRestic, "use-restic", o.UseRestic, "Create restic daemonset. Optional.")
	flags.BoolVar(&o.Wait, "wait", o.Wait, "Wait for Velero deployment to be ready. Optional.")
	flags.DurationVar(&o.DefaultResticMaintenanceFrequency, "default-restic-prune-frequency", o.DefaultResticMaintenanceFrequency, "How often 'restic prune' is run for restic repositories by default. Optional.")
	flags.Var(&o.Plugins, "plugins", "Plugin container images to install into the Velero Deployment")
	flags.BoolVar(&o.CRDsOnly, "crds-only", o.CRDsOnly, "Only generate CustomResourceDefinition resources. Useful for updating CRDs for an existing Velero install.")
	flags.StringVar(&o.CACertFile, "cacert", o.CACertFile, "File containing a certificate bundle to use when verifying TLS connections to the object store. Optional.")
	flags.StringVar(&o.Features, "features", o.Features, "Comma separated list of Velero feature flags to be set on the Velero deployment and the restic daemonset, if restic is enabled")
	flags.BoolVar(&o.DefaultVolumesToRestic, "default-volumes-to-restic", o.DefaultVolumesToRestic, "Bool flag to configure Velero server to use restic by default to backup all pod volumes on all backups. Optional.")
}

// NewInstallOptions instantiates a new, default InstallOptions struct.
func NewInstallOptions() *InstallOptions {
	return &InstallOptions{
		Namespace:                 velerov1api.DefaultNamespace,
		Image:                     velero.DefaultVeleroImage(),
		BackupStorageConfig:       flag.NewMap(),
		VolumeSnapshotConfig:      flag.NewMap(),
		PodAnnotations:            flag.NewMap(),
		ServiceAccountAnnotations: flag.NewMap(),
		VeleroPodCPURequest:       install.DefaultVeleroPodCPURequest,
		VeleroPodMemRequest:       install.DefaultVeleroPodMemRequest,
		VeleroPodCPULimit:         install.DefaultVeleroPodCPULimit,
		VeleroPodMemLimit:         install.DefaultVeleroPodMemLimit,
		ResticPodCPURequest:       install.DefaultResticPodCPURequest,
		ResticPodMemRequest:       install.DefaultResticPodMemRequest,
		ResticPodCPULimit:         install.DefaultResticPodCPULimit,
		ResticPodMemLimit:         install.DefaultResticPodMemLimit,
		// Default to creating a VSL unless we're told otherwise
		UseVolumeSnapshots:      true,
		NoDefaultBackupLocation: false,
		CRDsOnly:                false,
		DefaultVolumesToRestic:  false,
	}
}

// AsVeleroOptions translates the values provided at the command line into values used to instantiate Kubernetes resources
func (o *InstallOptions) AsVeleroOptions() (*install.VeleroOptions, error) {
	var secretData []byte
	if o.SecretFile != "" && !o.NoSecret {
		realPath, err := filepath.Abs(o.SecretFile)
		if err != nil {
			return nil, err
		}
		secretData, err = ioutil.ReadFile(realPath)
		if err != nil {
			return nil, err
		}
	}
	var caCertData []byte
	if o.CACertFile != "" {
		realPath, err := filepath.Abs(o.CACertFile)
		if err != nil {
			return nil, err
		}
		caCertData, err = ioutil.ReadFile(realPath)
		if err != nil {
			return nil, err
		}
	}
	veleroPodResources, err := kubeutil.ParseResourceRequirements(o.VeleroPodCPURequest, o.VeleroPodMemRequest, o.VeleroPodCPULimit, o.VeleroPodMemLimit)
	if err != nil {
		return nil, err
	}
	resticPodResources, err := kubeutil.ParseResourceRequirements(o.ResticPodCPURequest, o.ResticPodMemRequest, o.ResticPodCPULimit, o.ResticPodMemLimit)
	if err != nil {
		return nil, err
	}

	return &install.VeleroOptions{
		Namespace:                         o.Namespace,
		Image:                             o.Image,
		ProviderName:                      o.ProviderName,
		Bucket:                            o.BucketName,
		Prefix:                            o.Prefix,
		PodAnnotations:                    o.PodAnnotations.Data(),
		ServiceAccountAnnotations:         o.ServiceAccountAnnotations.Data(),
		VeleroPodResources:                veleroPodResources,
		ResticPodResources:                resticPodResources,
		SecretData:                        secretData,
		RestoreOnly:                       o.RestoreOnly,
		UseRestic:                         o.UseRestic,
		UseVolumeSnapshots:                o.UseVolumeSnapshots,
		BSLConfig:                         o.BackupStorageConfig.Data(),
		VSLConfig:                         o.VolumeSnapshotConfig.Data(),
		DefaultResticMaintenanceFrequency: o.DefaultResticMaintenanceFrequency,
		Plugins:                           o.Plugins,
		NoDefaultBackupLocation:           o.NoDefaultBackupLocation,
		CACertData:                        caCertData,
		Features:                          strings.Split(o.Features, ","),
		DefaultVolumesToRestic:            o.DefaultVolumesToRestic,
	}, nil
}

// NewCommand creates a cobra command.
func NewCommand(f client.Factory) *cobra.Command {
	o := NewInstallOptions()
	c := &cobra.Command{
		Use:   "install",
		Short: "Install Velero",
		Long: `Install Velero onto a Kubernetes cluster using the supplied provider information, such as
the provider's name, a bucket name, and a file containing the credentials to access that bucket.
A prefix within the bucket and configuration for the backup store location may also be supplied.
Additionally, volume snapshot information for the same provider may be supplied.

All required CustomResourceDefinitions will be installed to the server, as well as the
Velero Deployment and associated Restic DaemonSet.

The provided secret data will be created in a Secret named 'cloud-credentials'.

All namespaced resources will be placed in the 'velero' namespace by default. 

The '--namespace' flag can be used to specify a different namespace to install into.

Use '--wait' to wait for the Velero Deployment to be ready before proceeding.

Use '-o yaml' or '-o json'  with '--dry-run' to output all generated resources as text instead of sending the resources to the server.
This is useful as a starting point for more customized installations.
		`,
		Example: `  # velero install --provider gcp --plugins velero/velero-plugin-for-gcp:v1.0.0 --bucket mybucket --secret-file ./gcp-service-account.json

  # velero install --provider aws --plugins velero/velero-plugin-for-aws:v1.0.0 --bucket backups --secret-file ./aws-iam-creds --backup-location-config region=us-east-2 --snapshot-location-config region=us-east-2

  # velero install --provider aws --plugins velero/velero-plugin-for-aws:v1.0.0 --bucket backups --secret-file ./aws-iam-creds --backup-location-config region=us-east-2 --snapshot-location-config region=us-east-2 --use-restic

  # velero install --provider gcp --plugins velero/velero-plugin-for-gcp:v1.0.0 --bucket gcp-backups --secret-file ./gcp-creds.json --wait

  # velero install --provider aws --plugins velero/velero-plugin-for-aws:v1.0.0 --bucket backups --backup-location-config region=us-west-2 --snapshot-location-config region=us-west-2 --no-secret --pod-annotations iam.amazonaws.com/role=arn:aws:iam::<AWS_ACCOUNT_ID>:role/<VELERO_ROLE_NAME>

  # velero install --provider gcp --plugins velero/velero-plugin-for-gcp:v1.0.0 --bucket gcp-backups --secret-file ./gcp-creds.json --velero-pod-cpu-request=1000m --velero-pod-cpu-limit=5000m --velero-pod-mem-request=512Mi --velero-pod-mem-limit=1024Mi

  # velero install --provider gcp --plugins velero/velero-plugin-for-gcp:v1.0.0 --bucket gcp-backups --secret-file ./gcp-creds.json --restic-pod-cpu-request=1000m --restic-pod-cpu-limit=5000m --restic-pod-mem-request=512Mi --restic-pod-mem-limit=1024Mi

  # velero install --provider azure --plugins velero/velero-plugin-for-microsoft-azure:v1.0.0 --bucket $BLOB_CONTAINER --secret-file ./credentials-velero --backup-location-config resourceGroup=$AZURE_BACKUP_RESOURCE_GROUP,storageAccount=$AZURE_STORAGE_ACCOUNT_ID[,subscriptionId=$AZURE_BACKUP_SUBSCRIPTION_ID] --snapshot-location-config apiTimeout=<YOUR_TIMEOUT>[,resourceGroup=$AZURE_BACKUP_RESOURCE_GROUP,subscriptionId=$AZURE_BACKUP_SUBSCRIPTION_ID]`,
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(o.Validate(c, args, f))
			cmd.CheckError(o.Complete(args, f))
			cmd.CheckError(o.Run(c, f))
		},
	}

	o.BindFlags(c.Flags())
	output.BindFlags(c.Flags())
	output.ClearOutputFlagDefault(c)

	return c
}

// Run executes a command in the context of the provided arguments.
func (o *InstallOptions) Run(c *cobra.Command, f client.Factory) error {
	var resources *unstructured.UnstructuredList
	if o.CRDsOnly {
		resources = install.AllCRDs()
	} else {
		vo, err := o.AsVeleroOptions()
		if err != nil {
			return err
		}

		resources = install.AllResources(vo)
	}

	if _, err := output.PrintWithFormat(c, resources); err != nil {
		return err
	}

	if o.DryRun {
		return nil
	}
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}
	factory := client.NewDynamicFactory(dynamicClient)

	errorMsg := fmt.Sprintf("\n\nError installing Velero. Use `kubectl logs deploy/velero -n %s` to check the deploy logs", o.Namespace)

	err = install.Install(factory, resources, os.Stdout)
	if err != nil {
		return errors.Wrap(err, errorMsg)
	}

	if o.Wait {
		fmt.Println("Waiting for Velero deployment to be ready.")
		if _, err = install.DeploymentIsReady(factory, o.Namespace); err != nil {
			return errors.Wrap(err, errorMsg)
		}

		if o.UseRestic {
			fmt.Println("Waiting for Velero restic daemonset to be ready.")
			if _, err = install.DaemonSetIsReady(factory, o.Namespace); err != nil {
				return errors.Wrap(err, errorMsg)
			}
		}
	}
	if o.SecretFile == "" {
		fmt.Printf("\nNo secret file was specified, no Secret created.\n\n")
	}

	if o.NoDefaultBackupLocation {
		fmt.Printf("\nNo bucket and provider were specified, no default backup storage location created.\n\n")
	}

	fmt.Printf("Velero is installed! ⛵ Use 'kubectl logs deployment/velero -n %s' to view the status.\n", o.Namespace)
	return nil
}

//Complete completes options for a command.
func (o *InstallOptions) Complete(args []string, f client.Factory) error {
	o.Namespace = f.Namespace()
	return nil
}

// Validate validates options provided to a command.
func (o *InstallOptions) Validate(c *cobra.Command, args []string, f client.Factory) error {
	if err := output.ValidateFlags(c); err != nil {
		return err
	}

	// If we're only installing CRDs, we can skip the rest of the validation.
	if o.CRDsOnly {
		return nil
	}

	// Our main 3 providers don't support bucket names starting with a dash, and a bucket name starting with one
	// can indicate that an environment variable was left blank.
	// This case will help catch that error
	if strings.HasPrefix(o.BucketName, "-") {
		return errors.Errorf("Bucket names cannot begin with a dash. Bucket name was: %s", o.BucketName)
	}

	if o.NoDefaultBackupLocation {

		if o.BucketName != "" {
			return errors.New("Cannot use both --bucket and --no-default-backup-location at the same time")
		}

		if o.Prefix != "" {
			return errors.New("Cannot use both --prefix and --no-default-backup-location at the same time")
		}

		if o.BackupStorageConfig.String() != "" {
			return errors.New("Cannot use both --backup-location-config and --no-default-backup-location at the same time")
		}
	} else {
		if o.ProviderName == "" {
			return errors.New("--provider is required")
		}

		if o.BucketName == "" {
			return errors.New("--bucket is required")
		}

	}

	if o.UseVolumeSnapshots {
		if o.ProviderName == "" {
			return errors.New("--provider is required when --use-volume-snapshots is set to true")
		}
	} else {
		if o.VolumeSnapshotConfig.String() != "" {
			return errors.New("--snapshot-location-config must be empty when --use-volume-snapshots=false")
		}
	}

	if o.NoDefaultBackupLocation && !o.UseVolumeSnapshots {
		if o.ProviderName != "" {
			return errors.New("--provider must be empty when using --no-default-backup-location and --use-volume-snapshots=false")
		}
	} else {
		if len(o.Plugins) == 0 {
			return errors.New("--plugins flag is required")
		}
	}

	if o.DefaultVolumesToRestic && !o.UseRestic {
		return errors.New("--use-restic is required when using --default-volumes-to-restic")
	}

	switch {
	case o.SecretFile == "" && !o.NoSecret:
		return errors.New("One of --secret-file or --no-secret is required")
	case o.SecretFile != "" && o.NoSecret:
		return errors.New("Cannot use both --secret-file and --no-secret")
	}

	if o.DefaultResticMaintenanceFrequency < 0 {
		return errors.New("--default-restic-prune-frequency must be non-negative")
	}

	return nil
}
