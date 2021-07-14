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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	veleroCLI, veleroImage, cloudCredentialsFile, bslConfig, bslBucket, bslPrefix, vslConfig, cloudProvider, objectStoreProvider string
	additionalBSLProvider, additionalBSLBucket, additionalBSLPrefix, additionalBSLConfig, additionalBSLCredentials               string
)

func init() {
	flag.StringVar(&cloudProvider, "cloud-provider", "", "Cloud that Velero will be installed into.  Required.")
	flag.StringVar(&objectStoreProvider, "object-store-provider", "", "Provider of object store plugin. Required if cloud-provider is kind, otherwise ignored.")
	flag.StringVar(&bslBucket, "bucket", "", "name of the object storage bucket where backups from e2e tests should be stored. Required.")
	flag.StringVar(&cloudCredentialsFile, "credentials-file", "", "file containing credentials for backup and volume provider. Required.")
	flag.StringVar(&veleroCLI, "velerocli", "velero", "path to the velero application to use.")
	flag.StringVar(&veleroImage, "velero-image", "velero/velero:main", "image for the velero server to be tested.")
	flag.StringVar(&bslConfig, "bsl-config", "", "configuration to use for the backup storage location. Format is key1=value1,key2=value2")
	flag.StringVar(&bslPrefix, "prefix", "", "prefix under which all Velero data should be stored within the bucket. Optional.")
	flag.StringVar(&vslConfig, "vsl-config", "", "configuration to use for the volume snapshot location. Format is key1=value1,key2=value2")

	// Flags to create an additional BSL for multiple credentials test
	flag.StringVar(&additionalBSLProvider, "additional-bsl-object-store-provider", "", "Provider of object store plugin for additional backup storage location. Required if testing multiple credentials support.")
	flag.StringVar(&additionalBSLBucket, "additional-bsl-bucket", "", "name of the object storage bucket for additional backup storage location. Required if testing multiple credentials support.")
	flag.StringVar(&additionalBSLPrefix, "additional-bsl-prefix", "", "prefix under which all Velero data should be stored within the bucket for additional backup storage location. Optional.")
	flag.StringVar(&additionalBSLConfig, "additional-bsl-config", "", "configuration to use for the additional backup storage location. Format is key1=value1,key2=value2")
	flag.StringVar(&additionalBSLCredentials, "additional-bsl-credentials-file", "", "file containing credentials for additional backup storage location provider. Required if testing multiple credentials support.")
}

func TestE2e(t *testing.T) {
	// Skip running E2E tests when running only "short" tests because:
	// 1. E2E tests are long running tests involving installation of Velero and performing backup and restore operations.
	// 2. E2E tests require a kubernetes cluster to install and run velero which further requires ore configuration. See above referenced command line flags.
	if testing.Short() {
		t.Skip("Skipping E2E tests")
	}

	if cloudProvider != "kind" && vslConfig == "" {
		fmt.Println("\nWARNING! You are about to test Velero on a cloud provider but " +
			"did not set the `vslConfig` configuration for snapshots. Continue testing only if " +
			"not running tests that interact with snapshots.\n")
	}

	fmt.Println("========================")

	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
