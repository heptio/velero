package e2e

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

const (
	kibishiNamespace = "kibishii-workload"
)

func installKibishii(ctx context.Context, namespace string, cloudPlatform string) error {
	// We use kustomize to generate YAML for Kibishii from the checked-in yaml directories
	kibishiiInstallCmd := exec.CommandContext(ctx, "kubectl", "apply", "-n", namespace, "-k",
		"github.com/vmware-tanzu-labs/distributed-data-generator/kubernetes/yaml/"+cloudPlatform)
	stdoutPipe, err := kibishiiInstallCmd.StdoutPipe()
	if err != nil {
		return err
	}
	err = kibishiiInstallCmd.Start()
	if err != nil {
		return err
	}

	defer stdoutPipe.Close()
	// copy the data written to the PipeReader via the cmd to stdout
	_, err = io.Copy(os.Stdout, stdoutPipe)

	if err != nil {
		return err
	}
	err = kibishiiInstallCmd.Wait()
	if err != nil {
		return err
	}

	kibishiiSetWaitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status", "statefulset.apps/kibishii-deployment",
		"-n", namespace, "-w", "--timeout=30m")
	err = kibishiiSetWaitCmd.Run()

	if err != nil {
		return err
	}

	jumpPadWaitCmd := exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=ready", "-n", namespace, "pod/jump-pad")
	err = jumpPadWaitCmd.Run()

	return err
}

func generateData(ctx context.Context, namespace string, levels int, filesPerLevel int, dirsPerLevel int, fileSize int,
	blockSize int, passNum int, expectedNodes int) error {
	kibishiiGenerateCmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, "jump-pad", "--",
		"/usr/local/bin/generate.sh", strconv.Itoa(levels), strconv.Itoa(filesPerLevel), strconv.Itoa(dirsPerLevel), strconv.Itoa(fileSize),
		strconv.Itoa(blockSize), strconv.Itoa(passNum), strconv.Itoa(expectedNodes))
	fmt.Printf("kibishiiGenerateCmd cmd =%v\n", kibishiiGenerateCmd)
	stdoutPipe, err := kibishiiGenerateCmd.StdoutPipe()
	if err != nil {
		return err
	}
	err = kibishiiGenerateCmd.Start()
	if err != nil {
		return err
	}
	defer stdoutPipe.Close()

	stdoutReader := bufio.NewReader(stdoutPipe)
	var readErr error
	for true {
		buf, isPrefix, err := stdoutReader.ReadLine()
		if err != nil {
			readErr = err
			break
		} else {
			if isPrefix {
				readErr = errors.New("line returned exceeded max length")
				break
			}
			line := strings.TrimSpace(string(buf))
			if line == "success" {
				break
			}
			if line == "failed" {
				readErr = errors.New("generate failed")
				break
			}
			fmt.Println(line)
		}
	}
	err = kibishiiGenerateCmd.Wait()
	if readErr != nil {
		err = readErr // Squash the Wait err, the read error is probably more interesting
	}
	return err
}

func verifyData(ctx context.Context, namespace string, levels int, filesPerLevel int, dirsPerLevel int, fileSize int,
	blockSize int, passNum int, expectedNodes int) error {
	kibishiiVerifyCmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, "jump-pad", "--",
		"/usr/local/bin/verify.sh", strconv.Itoa(levels), strconv.Itoa(filesPerLevel), strconv.Itoa(dirsPerLevel), strconv.Itoa(fileSize),
		strconv.Itoa(blockSize), strconv.Itoa(passNum), strconv.Itoa(expectedNodes))
	fmt.Printf("kibishiiVerifyCmd cmd =%v\n", kibishiiVerifyCmd)
	stdoutPipe, err := kibishiiVerifyCmd.StdoutPipe()
	if err != nil {
		return err
	}
	err = kibishiiVerifyCmd.Start()
	if err != nil {
		return err
	}
	defer stdoutPipe.Close()

	stdoutReader := bufio.NewReader(stdoutPipe)
	var readErr error
	for true {
		buf, isPrefix, err := stdoutReader.ReadLine()
		if err != nil {
			readErr = err
			break
		} else {
			if isPrefix {
				readErr = errors.New("line returned exceeded max length")
				break
			}
			line := strings.TrimSpace(string(buf))
			if line == "success" {
				break
			}
			if line == "failed" {
				readErr = errors.New("generate failed")
				break
			}
			fmt.Println(line)
		}
	}
	err = kibishiiVerifyCmd.Wait()
	if readErr != nil {
		err = readErr // Squash the Wait err, the read error is probably more interesting
	}
	return err
}

// RunKibishiiTests runs kibishii tests on the provider.
func RunKibishiiTests(client *kubernetes.Clientset, providerName, veleroCLI, backupName, restoreName string) error {
	oneMinTimeout, _ := context.WithTimeout(context.Background(), time.Minute)

	if err := CreateNamespace(oneMinTimeout, client, kibishiNamespace); err != nil {
		errors.Wrapf(err, "Failed to create namespace %s to install Kibishii workload", kibishiNamespace)
	}

	if err := installKibishii(oneMinTimeout, kibishiNamespace, providerName); err != nil {
		errors.Wrap(err, "Failed to install Kibishii workload")
	}

	oneHourTimeout, _ := context.WithTimeout(context.Background(), time.Minute*60)

	fmt.Printf("running kibishii generate")
	if err := generateData(oneHourTimeout, kibishiNamespace, 2, 10, 10, 1024, 1024, 0, 2); err != nil {
		return errors.Wrap(err, "Failed to generate data")
	}

	fmt.Printf("executing backup")
	thirtyMinTimeout, _ := context.WithTimeout(context.Background(), time.Minute*30)

	if err := BackupNamespace(thirtyMinTimeout, veleroCLI, backupName, kibishiNamespace); err != nil {
		return errors.Wrapf(err, "Failed to backup kibishii namespace %s", kibishiNamespace)
	}

	if err := CheckBackupPhase(oneMinTimeout, veleroCLI, backupName, velerov1api.BackupPhaseCompleted); err != nil {
		return errors.Wrapf(err, "Failed to verify status of backup %s", backupName)
	}

	fmt.Printf("Simulating a disaster by removing namespace %s", kibishiNamespace)
	if err := client.CoreV1().Namespaces().Delete(oneMinTimeout, kibishiNamespace, metav1.DeleteOptions{}); err != nil {
		return errors.Wrap(err, "Failed to simulate a disaster")
	}

	fmt.Printf("restoring namespace")
	if err := RestoreNamespace(thirtyMinTimeout, veleroCLI, restoreName, backupName); err != nil {
		return errors.Wrapf(err, "Restore %s failed from backup %s", restoreName, backupName)
	}

	// TODO - check that namespace exists
	fmt.Printf("running kibishii verify")

	if err := verifyData(oneHourTimeout, kibishiNamespace, 2, 10, 10, 1024, 1024, 0, 2); err != nil {
		return errors.Wrap(err, "Failed to verify data generated by kiboshii")
	}

	if err := client.CoreV1().Namespaces().Delete(oneMinTimeout, kibishiNamespace, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "Failed to cleanup %s wrokload namespace", kibishiNamespace)
	}

	// kibishii test completed successfully
	return nil
}
