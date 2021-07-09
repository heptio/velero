package e2e

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("[Basic] Backup/restore of 2 namespaces", func() {

	client, err := newTestClient()
	Expect(err).To(Succeed(), "Failed to instantiate cluster client for multiple namespace tests")

	BeforeEach(func() {
		var err error
		flag.Parse()
		uuidgen, err = uuid.NewRandom()
		Expect(err).To(Succeed())
		if installVelero {
			Expect(veleroInstall(context.Background(), veleroImage, veleroNamespace, cloudProvider, objectStoreProvider, false,
				cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, crdsVersion, "")).To(Succeed())

		}
	})

	AfterEach(func() {
		if installVelero {
			timeoutCTX, _ := context.WithTimeout(context.Background(), time.Minute)
			err := veleroUninstall(timeoutCTX, client.kubebuilder, installVelero, veleroNamespace)
			Expect(err).To(Succeed())
		}

	})

	Context("When I create 2 namespaces", func() {
		It("should be successfully backed up and restored", func() {
			backupName := "backup-" + uuidgen.String()
			restoreName := "restore-" + uuidgen.String()
			fiveMinTimeout, _ := context.WithTimeout(context.Background(), 5*time.Minute)
			RunMultipleNamespaceTest(fiveMinTimeout, client, "nstest-"+uuidgen.String(), 2,
				backupName, restoreName)
		})
	})
})

var _ = Describe("[Scale] Backup/restore of 2500 namespaces", func() {

	client, err := newTestClient()
	Expect(err).To(Succeed(), "Failed to instantiate cluster client for multiple namespace tests")

	BeforeEach(func() {
		var err error
		flag.Parse()
		uuidgen, err = uuid.NewRandom()
		Expect(err).To(Succeed())
		if installVelero {
			Expect(veleroInstall(context.Background(), veleroImage, veleroNamespace, cloudProvider, objectStoreProvider, false,
				cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, crdsVersion, "")).To(Succeed())

		}
	})

	AfterEach(func() {
		if installVelero {
			timeoutCTX, _ := context.WithTimeout(context.Background(), time.Minute)
			err := veleroUninstall(timeoutCTX, client.kubebuilder, installVelero, veleroNamespace)
			Expect(err).To(Succeed())
		}

	})

	Context("When I create 2500 namespaces", func() {
		It("should be successfully backed up and restored", func() {
			backupName := "backup-" + uuidgen.String()
			restoreName := "restore-" + uuidgen.String()
			oneHourTimeout, _ := context.WithTimeout(context.Background(), 1*time.Hour)
			RunMultipleNamespaceTest(oneHourTimeout, client, "nstest-"+uuidgen.String(), 2500,
				backupName, restoreName)
		})
	})
})

func RunMultipleNamespaceTest(ctx context.Context, client testClient, nsBaseName string, numberOfNamespaces int, backupName string, restoreName string) error {
	shortTimeout, _ := context.WithTimeout(ctx, 5*time.Minute)
	defer cleanupNamespaces(ctx, client, nsBaseName) // Run at exit for final cleanup
	var excludeNamespaces []string

	// Currently it's hard to build a large list of namespaces to include and wildcards do not work so instead
	// we will exclude all of the namespaces that existed prior to the test from the backup
	namespaces, err := client.clientGo.CoreV1().Namespaces().List(shortTimeout, v1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "Could not retrieve namespaces")
	}

	for _, excludeNamespace := range namespaces.Items {
		excludeNamespaces = append(excludeNamespaces, excludeNamespace.Name)
	}
	for nsNum := 0; nsNum < numberOfNamespaces; nsNum++ {
		createNSName := fmt.Sprintf("%s-%00000d", nsBaseName, nsNum)
		if err := createNamespace(ctx, client, createNSName); err != nil {
			return errors.Wrapf(err, "Failed to create namespace %s", createNSName)
		}
	}
	if err := veleroBackupExcludeNamespaces(ctx, veleroCLI, veleroNamespace, backupName, excludeNamespaces); err != nil {
		veleroBackupLogs(ctx, veleroCLI, "", backupName)
		return errors.Wrapf(err, "Failed to backup backup namespaces %s-*", nsBaseName)
	}

	err = cleanupNamespaces(ctx, client, nsBaseName)
	if err != nil {
		return errors.Wrap(err, "Could cleanup retrieve namespaces")
	}

	err = veleroRestore(ctx, veleroCLI, veleroNamespace, restoreName, backupName)
	if err != nil {
		return errors.Wrap(err, "Restore failed")
	}

	// Verify that we got back all of the namespaces we created
	for nsNum := 0; nsNum < numberOfNamespaces; nsNum++ {
		checkNSName := fmt.Sprintf("%s-%00000d", nsBaseName, nsNum)
		checkNS, err := getNamespace(shortTimeout, client, checkNSName)
		if err != nil {
			return errors.Wrapf(err, "Could not retrieve test namespace %s", checkNSName)
		}
		if checkNS.Name != checkNSName {
			return errors.Errorf("Retrieved namespace for %s has name %s instead", checkNSName, checkNS.Name)
		}
	}
	// Cleanup is automatic on the way out
	return nil
}

func cleanupNamespaces(ctx context.Context, client testClient, nsBaseName string) error {
	namespaces, err := client.clientGo.CoreV1().Namespaces().List(ctx, v1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "Could not retrieve namespaces")
	}

	for _, checkNamespace := range namespaces.Items {
		if strings.HasPrefix(checkNamespace.Name, nsBaseName) {
			fmt.Printf("Cleaning up namespace %s\n", checkNamespace.Name)
			err = client.clientGo.CoreV1().Namespaces().Delete(ctx, checkNamespace.Name, v1.DeleteOptions{})
			if err != nil {
				return errors.Wrapf(err, "Could not delete namespace %s", checkNamespace.Name)
			}
		}
	}
	return nil
}
