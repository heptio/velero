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

package backuplocation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	velerov1api "github.com/vmware-tanzu/velero/api/v1"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kbclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = velerov1api.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func NewCreateCommand(f client.Factory, use string) *cobra.Command {
	o := NewCreateOptions()

	c := &cobra.Command{
		Use:   use + " NAME",
		Short: "Create a backup storage location",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(o.Complete(args, f))
			cmd.CheckError(o.Validate(c, args, f))
			cmd.CheckError(o.Run(c, f))
		},
	}

	o.BindFlags(c.Flags())
	output.BindFlags(c.Flags())
	output.ClearOutputFlagDefault(c)

	return c
}

type CreateOptions struct {
	Name             string
	Provider         string
	Bucket           string
	Prefix           string
	BackupSyncPeriod time.Duration
	Config           flag.Map
	Labels           flag.Map
	AccessMode       *flag.Enum
}

func NewCreateOptions() *CreateOptions {
	return &CreateOptions{
		Config: flag.NewMap(),
		AccessMode: flag.NewEnum(
			string(velerov1api.BackupStorageLocationAccessModeReadWrite),
			string(velerov1api.BackupStorageLocationAccessModeReadWrite),
			string(velerov1api.BackupStorageLocationAccessModeReadOnly),
		),
	}
}

func (o *CreateOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Provider, "provider", o.Provider, "name of the backup storage provider (e.g. aws, azure, gcp)")
	flags.StringVar(&o.Bucket, "bucket", o.Bucket, "name of the object storage bucket where backups should be stored")
	flags.StringVar(&o.Prefix, "prefix", o.Prefix, "prefix under which all Velero data should be stored within the bucket. Optional.")
	flags.DurationVar(&o.BackupSyncPeriod, "backup-sync-period", o.BackupSyncPeriod, "how often to ensure all Velero backups in object storage exist as Backup API objects in the cluster. Optional. Set this to `0s` to disable sync")
	flags.Var(&o.Config, "config", "configuration key-value pairs")
	flags.Var(&o.Labels, "labels", "labels to apply to the backup storage location")
	flags.Var(
		o.AccessMode,
		"access-mode",
		fmt.Sprintf("access mode for the backup storage location. Valid values are %s", strings.Join(o.AccessMode.AllowedValues(), ",")),
	)
}

func (o *CreateOptions) Validate(c *cobra.Command, args []string, f client.Factory) error {
	if err := output.ValidateFlags(c); err != nil {
		return err
	}

	if o.Provider == "" {
		return errors.New("--provider is required")
	}

	if o.Bucket == "" {
		return errors.New("--bucket is required")
	}

	if o.BackupSyncPeriod < 0 {
		return errors.New("--backup-sync-period must be non-negative")
	}

	return nil
}

func (o *CreateOptions) Complete(args []string, f client.Factory) error {
	o.Name = args[0]
	return nil
}

func (o *CreateOptions) Run(c *cobra.Command, f client.Factory) error {
	var backupSyncPeriod *metav1.Duration

	if c.Flags().Changed("backup-sync-period") {
		backupSyncPeriod = &metav1.Duration{Duration: o.BackupSyncPeriod}
	}

	backupStorageLocation := &velerov1api.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.Namespace(),
			Name:      o.Name,
			Labels:    o.Labels.Data(),
		},
		Spec: velerov1api.BackupStorageLocationSpec{
			Provider: o.Provider,
			StorageType: velerov1api.StorageType{
				ObjectStorage: &velerov1api.ObjectStorageLocation{
					Bucket: o.Bucket,
					Prefix: o.Prefix,
				},
			},
			Config:           o.Config.Data(),
			AccessMode:       velerov1api.BackupStorageLocationAccessMode(o.AccessMode.String()),
			BackupSyncPeriod: backupSyncPeriod,
		},
	}

	if printed, err := output.PrintWithFormat(c, backupStorageLocation); printed || err != nil {
		return err
	}

	clientConfig, err := f.ClientConfig()
	if err != nil {
		return err
	}

	clientKB, err := kbclient.New(clientConfig, kbclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}

	if err := clientKB.Create(context.Background(), backupStorageLocation, &kbclient.CreateOptions{}); err != nil {
		fmt.Println("argh")
		return err
	}

	fmt.Printf("Backup storage location %q configured successfully.\n", backupStorageLocation.Name)
	return nil
}
