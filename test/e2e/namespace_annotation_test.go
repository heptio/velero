package e2e

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("[Basic] Backup/restore of 2 namespaces -  Namespace Annotation Test", func() {

	client, err := newTestClient()
	Expect(err).To(Succeed(), "Failed to instantiate cluster client for namespace annotation test")

	BeforeEach(func() {
		var err error
		flag.Parse()
		uuidgen, err = uuid.NewRandom()
		Expect(err).To(Succeed())
		if installVelero {
			Expect(veleroInstall(context.Background(), veleroImage, veleroNamespace, cloudProvider, objectStoreProvider, false,
				cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, "")).To(Succeed())

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
		It("should be successfully backed up and restored including annotations", func() {
			backupName := "backup-" + uuidgen.String()
			restoreName := "restore-" + uuidgen.String()
			fiveMinTimeout, _ := context.WithTimeout(context.Background(), 5*time.Minute)
			RunNamespaceAnnotationTest(fiveMinTimeout, client, "nstest-"+uuidgen.String(), 2,
				backupName, restoreName)
		})
	})
})

func RunNamespaceAnnotationTest(ctx context.Context, client testClient, nsBaseName string, numberOfNamespaces int, backupName string, restoreName string) error {

	//defer does not run until the actual surrounding function returns a value, this line is meant for post test cleanup
	defer cleanupNamespaces(ctx, client, nsBaseName) // Run at exit for final cleanup
	var excludeNamespaces []string

	// Currently it's hard to build a large list of namespaces to include and wildcards do not work so instead
	// we will exclude all of the namespaces that existed prior to the test from the backup
	namespaces, err := client.clientGo.CoreV1().Namespaces().List(ctx, v1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "Could not retrieve namespaces")
	}

	//appending current namespaces to list of excluded namespaces, and no operations will be performed on those
	for _, excludeNamespace := range namespaces.Items {
		excludeNamespaces = append(excludeNamespaces, excludeNamespace.Name)
	}

	//creates the test namespaces mentioned in arguments of the function, with format of nstest-someuuid. This loops runs up to the number mentioned in the argument
	for nsNum := 0; nsNum < numberOfNamespaces; nsNum++ {
		createNSName := fmt.Sprintf("%s-%00000d", nsBaseName, nsNum)
		createAnnotationName := fmt.Sprintf("annotation-%s-%00000d", nsBaseName, nsNum)
		if err := createNamespaceAndAnnotation(ctx, client, createNSName, createAnnotationName); err != nil {
			return errors.Wrapf(err, "Failed to create namespace %s", createNSName)
		}
	}

	if err := veleroBackupExcludeNamespaces(ctx, veleroCLI, veleroNamespace, backupName, excludeNamespaces); err != nil {
		veleroBackupLogs(ctx, veleroCLI, "", backupName)
		return errors.Wrapf(err, "Failed to backup backup namespaces %s-*", nsBaseName)
	}

	//if error in cleaning up namespaces, then throw error
	err = cleanupNamespaces(ctx, client, nsBaseName)
	if err != nil {
		return errors.Wrap(err, "Could cleanup retrieve namespaces")
	}

	//if error in restoring namespaces, then throw error
	err = veleroRestore(ctx, veleroCLI, veleroNamespace, restoreName, backupName)
	if err != nil {
		return errors.Wrap(err, "Restore failed")
	}

	// Verify that we got back all of the namespaces we created, (and also check if we got back the annotations)
	for nsNum := 0; nsNum < numberOfNamespaces; nsNum++ {
		checkNSName := fmt.Sprintf("%s-%00000d", nsBaseName, nsNum)
		checkAnnoName := fmt.Sprintf("annotation-%s-%00000d", nsBaseName, nsNum)
		checkNS, err := getNamespace(ctx, client, checkNSName)

		if err != nil {
			return errors.Wrapf(err, "Could not retrieve test namespace %s", checkNSName)
		}

		if checkNS.Name != checkNSName {
			return errors.Errorf("Retrieved namespace for %s has name %s instead", checkNSName, checkNS.Name)
		}

		c := checkNS.ObjectMeta.Annotations["testAnnotation"]

		if c != checkAnnoName {
			return errors.Errorf("Retrieved annotation for %s has name %s instead", checkAnnoName, c)
		}
	}
	// Cleanup is automatic on the way out
	return nil
}
