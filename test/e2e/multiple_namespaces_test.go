package e2e

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/uuid"
	"sigs.k8s.io/cluster-api/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
)

var _ = Describe("Backup/restore of multiple namespaces", func() {
	client, err := newTestClient()
	Expect(err).To(Succeed(), "Failed to instantiate cluster client for multiple namespace tests")
	labelValue := "multiple-namespaces-" + util.RandomString(3)
	// Randomize the namespace so resource creation doesn;t collide with previously terminating resources in the same namespace.
	backupRestorMultipleeNamespaces := veleroNamespace("backup-restore-multiple-") + veleroNamespace(util.RandomString(5))

	BeforeEach(func() {
		var err error
		flag.Parse()
		uuidgen, err = uuid.NewRandom()
		Expect(err).To(Succeed())

		Expect(veleroInstall(client.ctx, backupRestorMultipleeNamespaces, veleroImage, cloudProvider, objectStoreProvider,
			cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, "", false)).To(Succeed())
	})

	AfterEach(func() {
		timeoutCTX, _ := context.WithTimeout(client.ctx, time.Minute)
		err := veleroUninstall(timeoutCTX, client.kubebuilder, veleroCLI, backupRestorMultipleeNamespaces)
		Expect(err).To(Succeed())
	})

	When("I create 2 namespaces", func() {
		It("should successfully back up and restore [2 namespaces]", func() {
			backupName := "backup-" + uuidgen.String()
			restoreName := "restore-" + uuidgen.String()
			fiveMinTimeout, _ := context.WithTimeout(client.ctx, 5*time.Minute)

			if err := runMultipleNamespaceTest(fiveMinTimeout, client, backupRestorMultipleeNamespaces, 2,
				"nstest-"+uuidgen.String(), backupName, restoreName, "default", labelValue); err != nil {
				Expect(err).To(Succeed(), "Failed to successfully backup to/restore from 2 namespaces")
			}
		})
	})

	When("When I create 2500 namespaces", func() {
		It("should successfully back up and restore [2500 namespaces]", func() {
			backupName := "backup-" + uuidgen.String()
			restoreName := "restore-" + uuidgen.String()
			oneHourTimeout, _ := context.WithTimeout(client.ctx, 1*time.Hour)

			if err := runMultipleNamespaceTest(oneHourTimeout, client, backupRestorMultipleeNamespaces, 2500,
				"nstest-"+uuidgen.String(), backupName, restoreName, "default", labelValue); err != nil {
				Expect(err).To(Succeed(), "Failed to successfully backup to/restore from 2500 namespaces")
			}
		})
	})
})

func runMultipleNamespaceTest(ctx context.Context, client testClient, veleroNamespace veleroNamespace, numberOfNamespaces int,
	nsBaseName, backupName, restoreName, backupLocation, labelValue string) error {
	shortTimeout, _ := context.WithTimeout(ctx, 5*time.Minute)
	defer deleteNamespaceListWithLabel(ctx, client, labelValue) // Run at exit for final cleanup

	// Create and label new namespaces for testing
	for nsNum := 0; nsNum < numberOfNamespaces; nsNum++ {
		createNSName := fmt.Sprintf("%s-%00000d", nsBaseName, nsNum)
		if err := createNamespace(ctx, client, createNSName, labelValue); err != nil {
			return errors.Wrapf(err, "Failed to create namespace %s", createNSName)
		}
	}

	// Backup namespaces created with the given label
	if err := veleroBackupNamespacesWithLabel(ctx, veleroNamespace, veleroCLI, backupName, labelValue); err != nil {
		veleroBackupLocationStatus(ctx, veleroNamespace, veleroCLI, backupLocation)
		veleroBackupLogs(ctx, veleroNamespace, veleroCLI, backupName)

		err = fmt.Errorf("failed to backup the namespaces %s-* with error %s", nsBaseName, errors.WithStack(err))
		return err
	}

	// Simulate a disaster
	namespaces, err := deleteNamespaceListWithLabel(ctx, client, labelValue)
	if err != nil || len(namespaces) == 0 {
		return errors.Wrap(err, "failed disaster simulation")
	}

	err = veleroRestoreNamespace(ctx, veleroNamespace, veleroCLI, restoreName, backupName)
	if err != nil {
		veleroBackupLocationStatus(ctx, veleroNamespace, veleroCLI, backupLocation)
		veleroRestoreLogs(ctx, veleroNamespace, veleroCLI, restoreName)

		err = fmt.Errorf("restore %s failed from backup %s with error %s", restoreName, backupName, errors.WithStack(err))
		return err
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

	return nil
}
