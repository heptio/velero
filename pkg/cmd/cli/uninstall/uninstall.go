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

package uninstall

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeerrs "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	kbclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli"
	"github.com/vmware-tanzu/velero/pkg/install"
)

// uninstallOptions collects all the options for uninstalling Velero from a Kubernetes cluster.
type uninstallOptions struct {
	wait, force bool
}

// BindFlags adds command line values to the options struct.
func (o *uninstallOptions) BindFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&o.wait, "wait", o.wait, "Wait for Velero uninstall to be ready. Optional.")
	flags.BoolVar(&o.force, "force", o.force, "Forces the Velero uninstall. Optional.")
}

// NewCommand creates a cobra command.
func NewCommand(f client.Factory) *cobra.Command {
	o := &uninstallOptions{}

	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Velero",
		Long: `Uninstall Velero along with the CRDs and clusterrolebinding.

The '--namespace' flag can be used to specify the namespace where velero is installed (default: velero).
Use '--wait' to wait for the Velero uninstall to be ready before proceeding.
Use '--force' to skip the prompt confirming if you want to uninstall Velero.
		`,
		Example: ` # velero uninstall --namespace staging`,
		Run: func(c *cobra.Command, args []string) {

			// Confirm if not asked to force-skip confirmation
			if !o.force {
				fmt.Println("You are about to uninstall Velero.")
				if !cli.GetConfirmation() {
					// Don't do anything unless we get confirmation
					return
				}
			}

			kbClient, err := f.KubebuilderClient()
			cmd.CheckError(err)
			cmd.CheckError(Run(context.Background(), kbClient, f.Namespace(), o.wait))
		},
	}

	o.BindFlags(c.Flags())
	return c
}

// Run removes all components that were deployed using the Velero install command
func Run(ctx context.Context, kbClient kbclient.Client, namespace string, waitToTerminate bool) error {
	var errs []error

	// namespace
	ns := &corev1.Namespace{}
	key := kbclient.ObjectKey{Name: namespace}
	if err := kbClient.Get(ctx, key, ns); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("Velero namespace %q does not exist, skipping.\n", namespace)
		} else {
			errs = append(errs, errors.WithStack(err))
		}
	} else {
		if ns.Status.Phase == corev1.NamespaceTerminating {
			fmt.Printf("Velero namespace %q is terminating.\n", namespace)
		} else {
			if err := kbClient.Delete(ctx, ns); err != nil {
				errs = append(errs, errors.WithStack(err))
			}
		}
	}

	// ClusterRoleBinding
	crb := install.ClusterRoleBinding(namespace)
	key = kbclient.ObjectKey{Name: crb.Name}
	if err := kbClient.Get(ctx, key, crb); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("Velero ClusterRoleBinding %q does not exist, skipping.\n", crb.Name)
		} else {
			errs = append(errs, errors.WithStack(err))
		}
	} else {
		if err := kbClient.Delete(ctx, crb); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
	}

	// CRDs
	veleroLabels := labels.FormatLabels(install.Labels())
	crdList := apiextv1beta1.CustomResourceDefinitionList{}
	opts := kbclient.ListOptions{
		Namespace: namespace,
		Raw: &metav1.ListOptions{
			LabelSelector: veleroLabels,
		},
	}
	if err := kbClient.List(context.Background(), &crdList, &opts); err != nil {
		errs = append(errs, errors.WithStack(err))
	} else {
		if len(crdList.Items) == 0 {
			fmt.Print("Velero CRDs do not exist, skipping.\n")
		} else {
			veleroLabelSelector := labels.SelectorFromSet(install.Labels())
			opts := []kbclient.DeleteAllOfOption{
				kbclient.InNamespace(namespace),
				kbclient.MatchingLabelsSelector{
					Selector: veleroLabelSelector,
				},
			}
			crd := &apiextv1beta1.CustomResourceDefinition{}
			if err := kbClient.DeleteAllOf(ctx, crd, opts...); err != nil {
				errs = append(errs, errors.WithStack(err))
			}
		}
	}

	if waitToTerminate && len(ns.Name) != 0 {
		fmt.Printf("Waiting for Velero uninstall to complete on namespace %s. You may safely press ctrl-c to stop waiting - uninstall will continue in the background.\n", ns.Name)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		checkFunc := func() {
			err := kbClient.Get(ctx, key, ns)
			if err != nil {
				if apierrors.IsNotFound(err) {
					fmt.Print("\n")
					cancel()
					return
				}
				errs = append(errs, errors.WithStack(err))
			}
			fmt.Print(".")
		}

		wait.Until(checkFunc, 5*time.Millisecond, ctx.Done())
	}

	if kubeerrs.NewAggregate(errs) != nil {
		fmt.Printf("Errors while attempting to uninstall Velero: %q from the namespace %s ", kubeerrs.NewAggregate(errs), namespace)
		return kubeerrs.NewAggregate(errs)
	}

	msg := fmt.Sprintf("Velero uninstalled from the %s namespace", namespace)
	fmt.Println(msg + " ⛵")
	return nil
}
