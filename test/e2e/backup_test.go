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
	"flag"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/util"
)

var (
	uuidgen uuid.UUID
)

// Test backup and restore of Kibishi using restic
var _ = Describe("[Restic] Velero tests on cluster using the plugin provider for object storage and Restic for volume backups", backup_restore_with_restic)

var _ = Describe("[Snapshot] Velero tests on cluster using the plugin provider for object storage and snapshots for volume backups", backup_restore_with_snapshots)

func backup_restore_with_snapshots() {
	backupRestoreNamespace := veleroNamespace("backup-restore-snapshot-")
	backup_restore_test(backupRestoreNamespace, true)
}

func backup_restore_with_restic() {
	backupRestoreNamespace := veleroNamespace("backup-restore-restic-")
	backup_restore_test(backupRestoreNamespace, false)
}

func backup_restore_test(backupRestoreNamespace veleroNamespace, useVolumeSnapshots bool) {
	var (
		backupName, restoreName string
	)

	client, err := newTestClient()
	Expect(err).To(Succeed(), "Failed to instantiate cluster client for backup tests")

	Describe("Backing up and restoring the kibishii workload", func() {
		// Randomize the namespace to minimize resource creation collision with previously terminating resources in the same namespace.
		backupRestoreNamespace = backupRestoreNamespace + veleroNamespace(util.RandomString(5))
		labelValue := "backup-restore-kibishii-" + util.RandomString(3)

		BeforeEach(func() {
			if useVolumeSnapshots && cloudProvider == "kind" {
				Skip("Volume snapshots not supported on kind")
			}
			var err error
			flag.Parse()
			uuidgen, err = uuid.NewRandom()
			Expect(err).To(Succeed())

			Expect(veleroInstall(client.ctx, backupRestoreNamespace, veleroImage, cloudProvider, objectStoreProvider,
				cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, "", useVolumeSnapshots)).To(Succeed())

			if err := installKibishiiWorkload(client, cloudProvider, labelValue); err != nil {
				Expect(err).To(Succeed(), "Failed to install the Kibishii workload")
			}
		})

		AfterEach(func() {
			fmt.Print("\nInitialize test clean up...\n")
			if _, err := terminateKibishiiWorkload(client, labelValue); err != nil {
				Expect(err).To(Succeed(), "Failed to terminate the Kibishii workload")
			}

			err = veleroUninstall(client.ctx, client.kubebuilder, veleroCLI, backupRestoreNamespace)
			Expect(err).To(Succeed())
		})

		When("single credential is configured", func() {
			It("should be successfully backed up and restored [using the default BackupStorageLocation]", func() {
				backupName = "backup-" + uuidgen.String()
				restoreName = "restore-" + uuidgen.String()
				// Even though we are using Velero's CloudProvider plugin for object storage, the kubernetes cluster is running on
				// KinD. So use the kind installation for Kibishii.
				Expect(runKibishiiTests(client, backupRestoreNamespace, cloudProvider, veleroCLI, backupName, restoreName, "default", labelValue, useVolumeSnapshots)).To(Succeed(),
					"Failed to successfully backup and restore the Kibishii workload on the %s namespace and using the default BSL", backupRestoreNamespace)
			})

		})
	})

	Describe("Backing up and restoring the kibishii workload", func() {
		// Randomize the namespace to minimize resource creation collision with previously terminating resources in the same namespace.
		backupRestoreNamespace = backupRestoreNamespace + veleroNamespace(util.RandomString(5))
		labelValue := "backup-restore-kibishii-" + util.RandomString(3)

		BeforeEach(func() {
			if additionalBSLProvider == "" && additionalBSLBucket == "" && additionalBSLCredentials == "" {
				Skip("Not enough arguments passed in to configure the additional BackupStorageLocation")
			}
			if useVolumeSnapshots && cloudProvider == "kind" {
				Skip("Volume snapshots not supported on kind")
			}
			var err error
			flag.Parse()
			uuidgen, err = uuid.NewRandom()
			Expect(err).To(Succeed())

			Expect(veleroInstall(client.ctx, backupRestoreNamespace, veleroImage, cloudProvider, objectStoreProvider,
				cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, "", useVolumeSnapshots)).To(Succeed())

			if err := installKibishiiWorkload(client, cloudProvider, labelValue); err != nil {
				Expect(err).To(Succeed(), "Failed to install the Kibishii workload")
			}
		})

		AfterEach(func() {
			fmt.Print("\nInitialize test clean up...\n")
			if _, err := terminateKibishiiWorkload(client, labelValue); err != nil {
				Expect(err).To(Succeed(), "Failed to terminate the Kibishii workload")
			}

			err = veleroUninstall(client.ctx, client.kubebuilder, veleroCLI, backupRestoreNamespace)
			Expect(err).To(Succeed())
		})

		When("additional unique credential is configured", func() {
			It("should be successfully backed up and restored [using each configured BackupStorageLocation]", func() {
				Expect(veleroAddPluginsForProvider(client.ctx, backupRestoreNamespace, veleroCLI, additionalBSLProvider)).To(Succeed())

				// Create Secret for additional BSL
				secretName := fmt.Sprintf("bsl-credentials-%s", uuidgen)
				secretKey := fmt.Sprintf("creds-%s", additionalBSLProvider)
				files := map[string]string{
					secretKey: additionalBSLCredentials,
				}

				Expect(createSecretFromFiles(client.ctx, client, backupRestoreNamespace, secretName, files)).To(Succeed())

				// Create additional BSL using credential
				additionalBsl := fmt.Sprintf("bsl-%s", uuidgen)
				Expect(veleroCreateBackupLocation(client.ctx,
					veleroCLI,
					backupRestoreNamespace,
					additionalBsl,
					additionalBSLProvider,
					additionalBSLBucket,
					additionalBSLPrefix,
					additionalBSLConfig,
					secretName,
					secretKey,
				)).To(Succeed())

				bsls := []string{"default", additionalBsl}

				for _, bsl := range bsls {
					backupName = fmt.Sprintf("backup-%s-%s", bsl, uuidgen)
					restoreName = fmt.Sprintf("restore-%s-%s", bsl, uuidgen)

					Expect(runKibishiiTests(client, backupRestoreNamespace, cloudProvider, veleroCLI, backupName, restoreName, bsl, labelValue, useVolumeSnapshots)).To(Succeed(),
						"Failed to successfully backup and restore Kibishii on the %s namespace and using the %s BSL", backupRestoreNamespace, bsl)
				}
			})
		})
	})

}
