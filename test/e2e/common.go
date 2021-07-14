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

package e2e

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	corev1api "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vmware-tanzu/velero/pkg/builder"
	"github.com/vmware-tanzu/velero/pkg/label"
)

// ensureClusterExists returns whether or not a kubernetes cluster exists for tests to be run on.
func ensureClusterExists(ctx context.Context) error {
	return exec.CommandContext(ctx, "kubectl", "cluster-info").Run()
}

// createNamespace creates a Kubernetes namespace with the given name and adds the
// label key "e2e:". The optional labelValue parameter sets the label value: "e2e:<labelValue>".
func createNamespace(ctx context.Context, client testClient, namespace, labelValue string) error {
	ns := builder.ForNamespace(namespace).Result()
	addE2ELabel(ns, labelValue)
	_, err := client.clientGo.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

func getNamespace(ctx context.Context, client testClient, namespace string) (*corev1api.Namespace, error) {
	return client.clientGo.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
}

// deleteNamespaceListWithLabel deletes all namespaces that match the label "e2e:<labelValue>". If a matching list
// is found and successfully deleted, the list of the deleted namespaces is returned. When calling this function
// inside tear-down blocks probably not necessary to check if namespaces were found.
func deleteNamespaceListWithLabel(ctx context.Context, client testClient, labelValue string) ([]corev1api.Namespace, error) {
	if labelValue == "" {
		return nil, errors.New("a label must be specified to delete only the intended namespaces and not all")
	}

	namespaceList, err := client.clientGo.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: e2eLabel(labelValue),
	})
	if err != nil {
		return nil, errors.Wrap(err, "Could not retrieve namespaces")
	}

	fmt.Println("Deleting namespaces with the label", e2eLabel(labelValue))
	for _, ns := range namespaceList.Items {
		err = client.clientGo.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "Could not delete namespace %s", ns.Name)
		}
	}

	return namespaceList.Items, nil
}

// waitForNamespaceDeletion waits for namespace to be deleted.
func waitForNamespaceDeletion(ctx context.Context, interval, timeout time.Duration, client testClient, ns string) error {
	fmt.Printf("Initiating termination of the %s namespace\n", ns)
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		_, err := client.clientGo.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		fmt.Printf("Waiting for namespace %s to terminate...\n", ns)
		return false, nil
	})
	if err != nil {
		fmt.Printf("Namespace %s not successfully terminated...\n", ns)
		return err
	}
	fmt.Printf("Namespace %s successfully terminated...\n", ns)
	return nil
}

func createSecretFromFiles(ctx context.Context, client testClient, veleroNamespace veleroNamespace, name string, files map[string]string) error {
	data := make(map[string][]byte)

	for key, filePath := range files {
		contents, err := ioutil.ReadFile(filePath)
		if err != nil {
			return errors.WithMessagef(err, "Failed to read secret file %q", filePath)
		}

		data[key] = contents
	}

	secret := builder.ForSecret(veleroNamespace.String(), name).Data(data).Result()
	addE2ELabel(secret, "")
	_, err := client.clientGo.CoreV1().Secrets(veleroNamespace.String()).Create(ctx, secret, metav1.CreateOptions{})
	return err
}

// waitForPods waits until all of the pods have gone to PodRunning state
func waitForPods(ctx context.Context, client testClient, namespace string, pods []string) error {
	timeout := 10 * time.Minute
	interval := 5 * time.Second
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		for _, podName := range pods {
			checkPod, err := client.clientGo.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return false, errors.WithMessage(err, fmt.Sprintf("Failed to verify pod %s/%s is %s", namespace, podName, corev1api.PodRunning))
			}
			// If any pod is still waiting we don't need to check any more so return and wait for next poll interval
			if checkPod.Status.Phase != corev1api.PodRunning {
				fmt.Printf("Pod %s is in state %s waiting for it to be %s\n", podName, checkPod.Status.Phase, corev1api.PodRunning)
				return false, nil
			}
		}
		// All pods were in PodRunning state, we're successful
		return true, nil
	})
	if err != nil {
		return errors.Wrapf(err, fmt.Sprintf("Failed to wait for pods in namespace %s to start running", namespace))
	}
	return nil
}

// addE2ELabel labels the provided object with a "e2e" key and, optionally, the given label value.
// Example: an input of "multiple-resources" will add
// a properly formatted label of "e2e=multiple-resources".
// An empty value will label the object with only the key "e2e".
func addE2ELabel(obj metav1.Object, labelValue string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels["velero-e2e"] = label.GetValidName(labelValue)
	obj.SetLabels(labels)
}

func e2eLabel(labelValue string) string {
	label := map[string]string{
		"velero-e2e": labelValue,
	}

	return labels.FormatLabels(label)
}
