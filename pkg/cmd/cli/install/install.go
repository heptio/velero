/*
Copyright 2019 the Velero contributors.

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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	velerov1api "github.com/heptio/velero/pkg/apis/velero/v1"
	"github.com/heptio/velero/pkg/client"
	"github.com/heptio/velero/pkg/cmd"
	"github.com/heptio/velero/pkg/cmd/util/flag"
	"github.com/heptio/velero/pkg/cmd/util/output"
	"github.com/heptio/velero/pkg/install"
)

// InstallOptions collects all the options for installing Velero into a Kubernetes cluster.
type InstallOptions struct {
	Namespace            string
	Image                string
	BucketName           string
	Prefix               string
	ProviderName         string
	PodAnnotations       flag.Map
	VeleroPodCPURequest  string
	VeleroPodMemRequest  string
	VeleroPodCPULimit    string
	VeleroPodMemLimit    string
	ResticPodCPURequest  string
	ResticPodMemRequest  string
	ResticPodCPULimit    string
	ResticPodMemLimit    string
	RestoreOnly          bool
	SecretFile           string
	NoSecret             bool
	DryRun               bool
	BackupStorageConfig  flag.Map
	VolumeSnapshotConfig flag.Map
	UseRestic            bool
	Wait                 bool
	UseVolumeSnapshots   bool
}

// BindFlags adds command line values to the options struct.
func (o *InstallOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ProviderName, "provider", o.ProviderName, "provider name for backup and volume storage")
	flags.StringVar(&o.BucketName, "bucket", o.BucketName, "name of the object storage bucket where backups should be stored")
	flags.StringVar(&o.SecretFile, "secret-file", o.SecretFile, "file containing credentials for backup and volume provider. If not specified, --no-secret must be used for confirmation. Optional.")
	flags.BoolVar(&o.NoSecret, "no-secret", o.NoSecret, "flag indicating if a secret should be created. Must be used as confirmation if --secret-file is not provided. Optional.")
	flags.StringVar(&o.Image, "image", o.Image, "image to use for the Velero and restic server pods. Optional.")
	flags.StringVar(&o.Prefix, "prefix", o.Prefix, "prefix under which all Velero data should be stored within the bucket. Optional.")
	flags.Var(&o.PodAnnotations, "pod-annotations", "annotations to add to the Velero and restic pods. Optional. Format is key1=value1,key2=value2")
	flags.StringVar(&o.VeleroPodCPURequest, "velero-pod-cpu-request", o.VeleroPodCPURequest, "CPU request for Velero pod. Optional.")
	flags.StringVar(&o.VeleroPodMemRequest, "velero-pod-mem-request", o.VeleroPodMemRequest, "memory request for Velero pod. Optional.")
	flags.StringVar(&o.VeleroPodCPULimit, "velero-pod-cpu-limit", o.VeleroPodCPULimit, "CPU limit for Velero pod. Optional.")
	flags.StringVar(&o.VeleroPodMemLimit, "velero-pod-mem-limit", o.VeleroPodMemLimit, "memory limit for Velero pod. Optional.")
	flags.StringVar(&o.ResticPodCPURequest, "restic-pod-cpu-request", o.ResticPodCPURequest, "CPU request for restic pods. Optional.")
	flags.StringVar(&o.ResticPodMemRequest, "restic-pod-mem-request", o.ResticPodMemRequest, "memory request for restic pods. Optional.")
	flags.StringVar(&o.ResticPodCPULimit, "restic-pod-cpu-limit", o.ResticPodCPULimit, "CPU limit for restic pods. Optional.")
	flags.StringVar(&o.ResticPodMemLimit, "restic-pod-mem-limit", o.ResticPodMemLimit, "memory limit for restic pods. Optional.")
	flags.Var(&o.BackupStorageConfig, "backup-location-config", "configuration to use for the backup storage location. Format is key1=value1,key2=value2")
	flags.Var(&o.VolumeSnapshotConfig, "snapshot-location-config", "configuration to use for the volume snapshot location. Format is key1=value1,key2=value2")
	flags.BoolVar(&o.UseVolumeSnapshots, "use-volume-snapshots", o.UseVolumeSnapshots, "whether or not to create snapshot location automatically. Set to false if you do not plan to create volume snapshots via a storage provider.")
	flags.BoolVar(&o.RestoreOnly, "restore-only", o.RestoreOnly, "run the server in restore-only mode. Optional.")
	flags.BoolVar(&o.DryRun, "dry-run", o.DryRun, "generate resources, but don't send them to the cluster. Use with -o. Optional.")
	flags.BoolVar(&o.UseRestic, "use-restic", o.UseRestic, "create restic deployment. Optional.")
	flags.BoolVar(&o.Wait, "wait", o.Wait, "wait for Velero deployment to be ready. Optional.")
}

// NewInstallOptions instantiates a new, default InstallOptions struct.
func NewInstallOptions() *InstallOptions {
	return &InstallOptions{
		Namespace:            velerov1api.DefaultNamespace,
		Image:                install.DefaultImage,
		BackupStorageConfig:  flag.NewMap(),
		VolumeSnapshotConfig: flag.NewMap(),
		PodAnnotations:       flag.NewMap(),
		VeleroPodCPURequest:  install.DefaultVeleroPodCPURequest,
		VeleroPodMemRequest:  install.DefaultVeleroPodMemRequest,
		VeleroPodCPULimit:    install.DefaultVeleroPodCPULimit,
		VeleroPodMemLimit:    install.DefaultVeleroPodMemLimit,
		ResticPodCPURequest:  install.DefaultResticPodCPURequest,
		ResticPodMemRequest:  install.DefaultResticPodMemRequest,
		ResticPodCPULimit:    install.DefaultResticPodCPULimit,
		ResticPodMemLimit:    install.DefaultResticPodMemLimit,
		// Default to creating a VSL unless we're told otherwise
		UseVolumeSnapshots: true,
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
	veleroPodResources, err := parseResourceRequests(o.VeleroPodCPURequest, o.VeleroPodMemRequest, o.VeleroPodCPULimit, o.VeleroPodMemLimit)
	if err != nil {
		return nil, err
	}
	resticPodResources, err := parseResourceRequests(o.ResticPodCPURequest, o.ResticPodMemRequest, o.ResticPodCPULimit, o.ResticPodMemLimit)
	if err != nil {
		return nil, err
	}

	return &install.VeleroOptions{
		Namespace:          o.Namespace,
		Image:              o.Image,
		ProviderName:       o.ProviderName,
		Bucket:             o.BucketName,
		Prefix:             o.Prefix,
		PodAnnotations:     o.PodAnnotations.Data(),
		VeleroPodResources: veleroPodResources,
		ResticPodResources: resticPodResources,
		SecretData:         secretData,
		RestoreOnly:        o.RestoreOnly,
		UseRestic:          o.UseRestic,
		UseVolumeSnapshots: o.UseVolumeSnapshots,
		BSLConfig:          o.BackupStorageConfig.Data(),
		VSLConfig:          o.VolumeSnapshotConfig.Data(),
	}, nil
}

// NewCommand creates a cobra command.
func NewCommand(f client.Factory) *cobra.Command {
	o := NewInstallOptions()
	c := &cobra.Command{
		Use:   "install",
		Short: "Install Velero",
		Long: `
Install Velero onto a Kubernetes cluster using the supplied provider information, such as
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
		Example: `	# velero install --bucket mybucket --provider gcp --secret-file ./gcp-service-account.json

	# velero install --bucket backups --provider aws --secret-file ./aws-iam-creds --backup-location-config region=us-east-2 --snapshot-location-config region=us-east-2

	# velero install --bucket backups --provider aws --secret-file ./aws-iam-creds --backup-location-config region=us-east-2 --snapshot-location-config region=us-east-2 --use-restic

	# velero install --bucket gcp-backups --provider gcp --secret-file ./gcp-creds.json --wait

	# velero install --bucket backups --provider aws --backup-location-config region=us-west-2 --snapshot-location-config region=us-west-2 --no-secret --pod-annotations iam.amazonaws.com/role=arn:aws:iam::<AWS_ACCOUNT_ID>:role/<VELERO_ROLE_NAME>

	# velero install --bucket gcp-backups --provider gcp --secret-file ./gcp-creds.json --velero-pod-cpu-request=1000m --velero-pod-cpu-limit=5000m --velero-pod-mem-request=512Mi --velero-pod-mem-limit=1024Mi

	# velero install --bucket gcp-backups --provider gcp --secret-file ./gcp-creds.json --restic-pod-cpu-request=1000m --restic-pod-cpu-limit=5000m --restic-pod-mem-request=512Mi --restic-pod-mem-limit=1024Mi

		`,
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
	vo, err := o.AsVeleroOptions()
	if err != nil {
		return err
	}

	resources, err := install.AllResources(vo)
	if err != nil {
		return err
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
		fmt.Println("Waiting for Velero to be ready.")
		if _, err = install.DeploymentIsReady(factory, o.Namespace); err != nil {
			return errors.Wrap(err, errorMsg)
		}
	}
	if o.SecretFile == "" {
		fmt.Printf("\nNo secret file was specified, no Secret created.\n\n")
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

	if o.BucketName == "" {
		return errors.New("--bucket is required")
	}

	// Our main 3 providers don't support bucket names starting with a dash, and a bucket name starting with one
	// can indicate that an environment variable was left blank.
	// This case will help catch that error
	if strings.HasPrefix(o.BucketName, "-") {
		return errors.Errorf("Bucket names cannot begin with a dash. Bucket name was: %s", o.BucketName)
	}

	if o.ProviderName == "" {
		return errors.New("--provider is required")
	}

	switch {
	case o.SecretFile == "" && !o.NoSecret:
		return errors.New("One of --secret-file or --no-secret is required")
	case o.SecretFile != "" && o.NoSecret:
		return errors.New("Cannot use both --secret-file and --no-secret")
	}

	return nil
}

// parseResourceRequests takes a set of CPU and memory requests and limit string
// values and returns a ResourceRequirements struct to be used in a Container.
// An error is returned if we cannot parse the request/limit.
func parseResourceRequests(cpuRequest, memRequest, cpuLimit, memLimit string) (corev1.ResourceRequirements, error) {
	var resources corev1.ResourceRequirements

	parsedCPURequest, err := resource.ParseQuantity(cpuRequest)
	if err != nil {
		return resources, errors.Wrapf(err, `couldn't parse CPU request "%s"`, cpuRequest)
	}

	parsedMemRequest, err := resource.ParseQuantity(memRequest)
	if err != nil {
		return resources, errors.Wrapf(err, `couldn't parse memory request "%s"`, memRequest)
	}

	parsedCPULimit, err := resource.ParseQuantity(cpuLimit)
	if err != nil {
		return resources, errors.Wrapf(err, `couldn't parse CPU limit "%s"`, cpuLimit)
	}

	parsedMemLimit, err := resource.ParseQuantity(memLimit)
	if err != nil {
		return resources, errors.Wrapf(err, `couldn't parse memory limit "%s"`, memLimit)
	}

	if parsedCPURequest.Cmp(parsedCPULimit) > 0 {
		return resources, errors.WithStack(errors.Errorf(`CPU request "%s" must be less than or equal to CPU limit "%s"`, cpuRequest, cpuLimit))
	}

	if parsedMemRequest.Cmp(parsedMemLimit) > 0 {
		return resources, errors.WithStack(errors.Errorf(`Memory request "%s" must be less than or equal to Memory limit "%s"`, memRequest, memLimit))
	}

	resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU:    parsedCPURequest,
		corev1.ResourceMemory: parsedMemRequest,
	}
	resources.Limits = corev1.ResourceList{
		corev1.ResourceCPU:    parsedCPULimit,
		corev1.ResourceMemory: parsedMemLimit,
	}

	return resources, nil
}
