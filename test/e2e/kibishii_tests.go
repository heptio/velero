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
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/pkg/errors"
	corev1api "k8s.io/api/core/v1"

	veleroexec "github.com/vmware-tanzu/velero/pkg/util/exec"
)

const (
	kibishiiNamespace = "kibishii-workload"
	jumpPadPod        = "jump-pad"
)

func installKibishiiWorkload(client testClient, cloudPlatform, labelValue string) error {
	fmt.Println("Creating the kibishii namespace and workload")
	fiveMinTimeout, cancel := context.WithTimeout(client.ctx, 5*time.Minute)
	defer cancel()

	if err := createNamespace(fiveMinTimeout, client, kibishiiNamespace, labelValue); err != nil {
		return errors.Wrapf(err, "Failed to create the namespace kibishii for the Kibishii workload")
	}

	// We use kustomize to generate YAML for Kibishii from the checked-in yaml directories
	kibishiiInstallCmd := exec.CommandContext(client.ctx, "kubectl", "apply", "-n", kibishiiNamespace, "-k",
		"github.com/vmware-tanzu-experiments/distributed-data-generator/kubernetes/yaml/"+cloudPlatform)
	_, stderr, err := veleroexec.RunCommand(kibishiiInstallCmd)
	if err != nil {
		return errors.Wrapf(err, "failed to install kibishii, stderr=%s", stderr)
	}

	kibishiiSetWaitCmd := exec.CommandContext(client.ctx, "kubectl", "rollout", "status", "statefulset.apps/kibishii-deployment",
		"-n", kibishiiNamespace, "-w", "--timeout=30m")
	_, stderr, err = veleroexec.RunCommand(kibishiiSetWaitCmd)
	if err != nil {
		return errors.Wrapf(err, "failed to rollout, stderr=%s", stderr)
	}

	fmt.Printf("Waiting for kibishii jump-pad pod to be ready\n")
	jumpPadWaitCmd := exec.CommandContext(client.ctx, "kubectl", "wait", "--for=condition=ready", "-n", kibishiiNamespace, "pod/jump-pad")
	_, stderr, err = veleroexec.RunCommand(jumpPadWaitCmd)
	if err != nil {
		return errors.Wrapf(err, "Failed to wait for ready status of pod %s/%s, stderr=%s", kibishiiNamespace, jumpPadPod, stderr)
	}

	return err
}

// terminateKibishiiWorkload deletes all namespaces that match the label "e2e:<labelValue>". If it succeeeds,
// it waits for the found namespaces to be terminated and returns the list of the namespaces deleted.
func terminateKibishiiWorkload(client testClient, labelValue string) ([]corev1api.Namespace, error) {
	fiveMinTimeout, cancel := context.WithTimeout(client.ctx, 5*time.Minute)
	defer cancel()

	oneHourTimeout, stop := context.WithTimeout(client.ctx, time.Minute*60)
	defer stop()

	interval := 5 * time.Second
	timeout := 10 * time.Minute

	// delete the ns
	var namespaces []corev1api.Namespace
	var err error
	if namespaces, err = deleteNamespaceListWithLabel(oneHourTimeout, client, labelValue); err != nil {
		return namespaces, errors.Wrap(err, "failed to delete the kibishii namespace")
	}

	if len(namespaces) == 0 {
		fmt.Printf("An attempt was made to delete namespaces with the label %s, but none was found\n", e2eLabel(labelValue))
		return nil, nil
	}

	// wait for ns delete
	err = waitForNamespaceDeletion(fiveMinTimeout, interval, timeout, client, kibishiiNamespace)
	if err != nil {
		return namespaces, errors.Wrap(err, "failed to wait for the kibishii namespace to terminate")
	}

	return namespaces, nil
}

// runKibishiiTests:
// - inserts data into the kibishii workload
// - creates a Velero backup of the kibishii workload in the given namespace
// - deletes the kibishii workload by deleting its namespace to simulate a disaster
// - creates a Velero restore of the created backup in the given names-ace
// - verifies the data restored is what's expected
// Assumes the kibishii workload has been created and Velero has been installed.
func runKibishiiTests(client testClient, veleroNamespace veleroNamespace, providerName, veleroCLI, backupName,
	restoreName, backupLocation, labelValue string, useVolumeSnapshots bool) error {
	fmt.Println("Starting the backup and restore of the kibishii workload")
	fiveMinTimeout, cancel := context.WithTimeout(client.ctx, 5*time.Minute)
	defer cancel()

	oneHourTimeout, stop := context.WithTimeout(client.ctx, time.Minute*60)
	defer stop()

	// TODO - Fix kibishii so we can check that it is ready to go
	fmt.Printf("Waiting for kibishii pods to be ready\n")
	if err := waitForKibishiiPods(oneHourTimeout, client); err != nil {
		return errors.Wrapf(err, "failed to wait for ready status of kibishii pods in %s", kibishiiNamespace)
	}

	if err := generateData(oneHourTimeout, 1, 1, 1, 1024, 1024, 0, 2); err != nil {
		return errors.Wrap(err, "failed to generate data")
	}

	if err := veleroBackupNamespace(oneHourTimeout, veleroNamespace, veleroCLI, backupName, kibishiiNamespace, backupLocation, useVolumeSnapshots); err != nil {
		veleroBackupLocationStatus(fiveMinTimeout, veleroNamespace, veleroCLI, backupLocation)
		veleroBackupLogs(fiveMinTimeout, veleroNamespace, veleroCLI, backupName)

		err = fmt.Errorf("failed to backup the kibishii namespace with error %s", errors.WithStack(err))
		return err
	}

	if providerName == "vsphere" && useVolumeSnapshots {
		// Wait for uploads started by the Velero Plug-in for vSphere to complete
		// TODO - remove after upload progress monitoring is implemented
		fmt.Println("Waiting for vSphere uploads to complete")
		if err := waitForVSphereUploadCompletion(oneHourTimeout, time.Hour, kibishiiNamespace); err != nil {
			return errors.Wrapf(err, "error waiting for uploads to complete")
		}
	}

	fmt.Println("Simulating a disaster by removing the kibishii namespace")
	namespaces, err := terminateKibishiiWorkload(client, labelValue)
	if err != nil || len(namespaces) == 0 {
		return errors.Wrap(err, "failed to simulate a disaster")
	}

	if err := veleroRestoreNamespace(oneHourTimeout, veleroNamespace, veleroCLI, restoreName, backupName); err != nil {
		veleroBackupLocationStatus(fiveMinTimeout, veleroNamespace, veleroCLI, backupLocation)
		veleroRestoreLogs(fiveMinTimeout, veleroNamespace, veleroCLI, restoreName)

		err = fmt.Errorf("restore %s failed from backup %s with error %s", restoreName, backupName, errors.WithStack(err))
		return err
	}

	// TODO - Fix kibishii so we can check that it is ready to go
	fmt.Printf("Waiting for kibishii pods to be ready\n")
	if err := waitForKibishiiPods(oneHourTimeout, client); err != nil {
		return errors.Wrap(err, "failed to wait for ready status of kibishii pods in the kibishii namespace")
	}

	// TODO - check that namespace exists
	fmt.Printf("running kibishii verify\n")
	if err := verifyData(oneHourTimeout, 2, 10, 10, 1024, 1024, 0, 2); err != nil {
		return errors.Wrap(err, "failed to verify data generated by kibishii")
	}

	fmt.Println("Backup and restore of the kibishii workload completed successfully")
	return nil
}

func waitForKibishiiPods(ctx context.Context, client testClient) error {
	return waitForPods(ctx, client, kibishiiNamespace, []string{"jump-pad", "etcd0", "etcd1", "etcd2", "kibishii-deployment-0", "kibishii-deployment-1"})
}

func generateData(ctx context.Context, levels int, filesPerLevel int, dirsPerLevel int, fileSize int,
	blockSize int, passNum int, expectedNodes int) error {
	kibishiiGenerateCmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", kibishiiNamespace, "jump-pad", "--",
		"/usr/local/bin/generate.sh", strconv.Itoa(levels), strconv.Itoa(filesPerLevel), strconv.Itoa(dirsPerLevel), strconv.Itoa(fileSize),
		strconv.Itoa(blockSize), strconv.Itoa(passNum), strconv.Itoa(expectedNodes))
	fmt.Printf("kibishiiGenerateCmd cmd =%v\n", kibishiiGenerateCmd)
	_, stderr, err := veleroexec.RunCommand(kibishiiGenerateCmd)
	if err != nil {
		return errors.Wrapf(err, "failed to generate data, stderr=%s", stderr)
	}

	return nil
}

func verifyData(ctx context.Context, levels int, filesPerLevel int, dirsPerLevel int, fileSize int,
	blockSize int, passNum int, expectedNodes int) error {
	kibishiiVerifyCmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", kibishiiNamespace, "jump-pad", "--",
		"/usr/local/bin/verify.sh", strconv.Itoa(levels), strconv.Itoa(filesPerLevel), strconv.Itoa(dirsPerLevel), strconv.Itoa(fileSize),
		strconv.Itoa(blockSize), strconv.Itoa(passNum), strconv.Itoa(expectedNodes))
	fmt.Printf("kibishiiVerifyCmd cmd =%v\n", kibishiiVerifyCmd)
	_, stderr, err := veleroexec.RunCommand(kibishiiVerifyCmd)
	if err != nil {
		return errors.Wrapf(err, "failed to verify data, stderr=%s", stderr)
	}

	return nil
}
