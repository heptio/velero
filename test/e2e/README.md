<!-- vscode-markdown-toc -->
* 1. [Prerequisites](#Prerequisites)
* 2. [Flow of a Velero e2e test](#FlowofaVeleroe2etest)
* 3. [Configuration of E2E tests](#ConfigurationofE2Etests)
	* 3.1. [Examples:](#Examples:)
* 4. [Running tests locally using `make`](#Runningtestslocallyusingmake)
	* 4.1. [Run tests with Kind](#RuntestswithKind)
		* 4.1.1. [Run Minio as a Docker image](#RunMinioasaDockerimage)
		* 4.1.2. [Instructions](#Instructions)
	* 4.2. [Run tests on a provider](#Runtestsonaprovider)
		* 4.2.1. [Note about snapshottingg](#Noteaboutsnapshottingg)
		* 4.2.2. [Instructions](#Instructions-1)
* 5. [Code under test](#Codeundertest)
* 6. [Adding resources and tests](#Addingresourcesandtests)
	* 6.1. [Add resources](#Addresources)
	* 6.2. [Add a label to all resources created in a test](#Addalabeltoallresourcescreatedinatest)
	* 6.3. [Cleaning up test data](#Cleaninguptestdata)
	* 6.4. [Util code](#Utilcode)
	* 6.5. [API clients](#APIclients)
	* 6.6. [Tips / Troubleshooting](#TipsTroubleshooting)

<!-- vscode-markdown-toc-config
	numbering=true
	autoSave=true
	/vscode-markdown-toc-config -->
<!-- /vscode-markdown-toc -->

# End-to-end tests

Document for running Velero end-to-end test suite.

The E2E tests are validating end-to-end behavior of Velero including install, backup and restore operations. These tests take longer to complete and is not expected to be part of day-to-day developer workflow. It is for this reason that they are disabled when running unit tests. This is accomplished by running unit tests in [`short`](https://golang.org/pkg/testing/#Short) mode using the `-short` flag to `go test`.

If you previously ran unit tests using the `go test ./...` command or any of its variations, then you will now run the same command with the  `-short` flag to `go test` to accomplish the same behavior. Alternatively, you can use the `make test` command to run unit tests.

##  1. <a name='Prerequisites'></a>Prerequisites

Running the E2E tests expects:
1. A running kubernetes cluster:
    1. With DNS and CNI installed.
    1. Compatible with Velero- running Kubernetes v1.10 or later.
    1. With necessary storage drivers/provisioners installed.
1. `kubectl` installed locally.

Velero does not need to be installed in the cluster.

##  2. <a name='FlowofaVeleroe2etest'></a>Flow of a Velero e2e test

1. An API client is created at setup time

    This client is passed through to all the test specs to use.

1. Install Velero in a custom namespace

    For each test spec setup, Velero is installed in a custom namespace. This custom namespace is trickled down through all the test code for this spec, since it's where any Velero object needs to be created (backups, restores, etc). Having a custom namespace for each install is needed to avoid race conditions.

    At setup time, a unique label value is also created and passed through to all test specs so any cluster object created by these tests can have the "e2e" label added to them. Objects can have shared or unique labels, as well as multiple labels. Having at least one label helps with resource deletion with API calls, and also helps when manually having to delete dangling resources from when tests are unexpectedly stopped. See [Tips / Troubleshooting](#TipsTroubleshooting) for more information.

1. Load test data, create workloads

     The section [Adding resources and tests](#Addingresourcesandtests) has helpful instructions for adding resources to the cluster.

1. Execute

    Create tests, run, pass, win.

3. Termination

    Each test spec that had resources created in the cluster takes care of terminating them.

    The `veleroUninstall` will run delete actions in the same custom namespace where Velero was installed and delete all backups, all restores, and all backup locations that exist in that namespace, in this order (it's important that the backup location is deleted last), before uninstalling Velero.

##  3. <a name='ConfigurationofE2Etests'></a>Configuration of E2E tests

The [e2e_suite_test.go](e2e_suite_test.go) contains the set of flags needed to configure Velero. Those configurations or parameters are used to generate install options for Velero for each test suite. The input to these flags come from the variables in the [Makefile](Makefile).

To configure a single test spec to run, prepend the `GINKGO_FOCUS` variable with the name of the spec.

###  3.1. <a name='Examples:'></a>Examples:

To run the spec `It("should successfully back up and restore [2 namespaces]"...`, prepend:

`GINKGO_FOCUS='2 namespaces'`

To skip the spec `Describe("[Snapshot] Velero tests on cluster using the plugin provider for object storage and snapshots for volume backups"`, prepend:

`GINKGO_SKIP='Snapshot'` - This flag setting is particularly useful for running the e2e tests against a Kind cluster, since it will run all tests except the tests related to snapshotting.

If a test spec is a subtest and you make it the focus of the test run, it will run once for all parent tests. If you want to isolate the test run to only 1 set of parent/child, using the `GINKGO_SKIP` + `GINKGO_FOCUS` will work. For example, to run `Describe("[Restic] Velero tests on cluster using the plugin provider for object storage and Restic for volume backups"` + `It("should be successfully backed up and restored [using the default BackupStorageLocation]"` and skip the `[Snapshot]` spec:

`GINKGO_SKIP='Snapshot' GINKGO_FOCUS='using the default BackupStorageLocation'`

Any test that is skipped by setting the combination of those flags will keep from having the BeforeEach/AfterEach actions invoked for those tests.

Note: By default, the test suite is configured to skip a test that creates 2,500 namespaces. To enable this test to run, set:

`GINKGO_SKIP_LONG=''`

##  4. <a name='Runningtestslocallyusingmake'></a>Running tests locally using `make`

E2E tests can be run from the Velero repository root by running `make test-e2e`. While running E2E tests using `make` the E2E test configuration values are passed using `make` variables.

Tests can be run with the Kubernetes cluster hosted in various cloud providers or in a _Kind_ cluster with storage in a specified object store type.  Currently supported cloud provider types are _aws_, _azure_, _vsphere_ and _kind_.

###  4.1. <a name='RuntestswithKind'></a>Run tests with Kind

####  4.1.1. <a name='RunMinioasaDockerimage'></a>Run Minio as a Docker image

    [[Contributing FAQ] How do I run MinIO locally on my computer (as a standalone as opposed to running it on a pod)? · Discussion #3381 · vmware-tanzu/velero](https://github.com/vmware-tanzu/velero/discussions/3381)
####  4.1.2. <a name='Instructions'></a>Instructions

- Run Velero tests on a Kind cluster with MInio as the storage provider:

    ```bash
    GINKGO_SKIP='Snapshot' BSL_PREFIX=<PREFIX_UNDER_BUCKET> BSL_BUCKET=<BUCKET_FOR_E2E_TEST_BACKUP> CREDS_FILE=/path/to/aws-creds BSL_CONFIG=region=minio,s3ForcePathStyle="true",s3Url=http://,<YOUR IP>:9000 CLOUD_PROVIDER=ind OBJECT_STORE_PROVIDER=aws make test-e2e
    ```

###  4.2. <a name='Runtestsonaprovider'></a>Run tests on a provider

####  4.2.1. <a name='Noteaboutsnapshottingg'></a>Note about snapshottingg

    When running tests that take a snapshot on a provider, the optional paramenter `VSL_CONFIG` **must** be configured. This parameter is optional for tests where only objects (and not snapshots) are being backed up /restored. If you don't configure this parameter, be sure you are only running tests that don't need a snapshot.

    The test will detect when it is configured with a provider other than Kind but without this configuration and ask for confirmation. To skip this check, add `FORCE=true` to pass it as a variable to the test command. To configure the snapshot settings, pass the `VSL_CONFIG` parameter with the proper values for your provider. Here's an example for AWS: `VSL_CONFIG=region=us-west-2`.
####  4.2.2. <a name='Instructions-1'></a>Instructions

1. Run Velero tests in an AWS cluster:

    ```bash
    BSL_PREFIX=<PREFIX_UNDER_BUCKET> BSL_BUCKET=<BUCKET_FOR_E2E_TEST_BACKUP> CREDS_FILE=/path/to/aws-creds CLOUD_PROVIDER=aws make test-e2e
    ```

1. Run Velero tests in a Microsoft Azure cluster:

    ```bash
    BSL_CONFIG="resourceGroup=$AZURE_BACKUP_RESOURCE_GROUP,storageAccount=$AZURE_STORAGE_ACCOUNT_ID,subscriptionId=$AZURE_BACKUP_SUBSCRIPTION_ID" BSL_BUCKET=<BUCKET_FOR_E2E_TEST_BACKUP> CREDS_FILE=/path/to/azure-creds CLOUD_PROVIDER=azure make test-e2e
    ```

    Please refer to `velero-plugin-for-microsoft-azure` documentation for instruction to [set up permissions for Velero](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure#set-permissions-for-velero) and to [set up azure storage account and blob container](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure#setup-azure-storage-account-and-blob-container)

1. Run Ginko-focused Restore Multi-API Groups tests using Minio as the backup storage location:

   ```bash
   BSL_CONFIG="region=minio,s3ForcePathStyle=\"true\",s3Url=<ip address>:9000" BSL_PREFIX=<prefix> BSL_BUCKET=<bucket> CREDS_FILE=<absolute path to minio credentials file> CLOUD_PROVIDER=Kind OBJECT_STORE_PROVIDER=aws GINKGO_FOCUS="API group versions" make test-e2e
   ```

1. Run Velero tests in a Kind cluster with AWS (or Minio) as the storage provider and use Microsoft Azure as the storage provider for an additional Backup Storage Location:

    ```bash
    make test-e2e \
      CLOUD_PROVIDER=Kind OBJECT_STORE_PROVIDER=aws BSL_BUCKET=<BUCKET_FOR_E2E_TEST_BACKUP> BSL_PREFIX=<PREFIX_UNDER_BUCKET> CREDS_FILE=/path/to/aws-creds \
      ADDITIONAL_OBJECT_STORE_PROVIDER=azure ADDITIONAL_BSL_BUCKET=<BUCKET_FOR_AZURE_BSL> ADDITIONAL_BSL_PREFIX=<PREFIX_UNDER_BUCKET> ADDITIONAL_BSL_CONFIG=<CONFIG_FOR_AZURE_BUCKET> ADDITIONAL_CREDS_FILE=/path/to/azure-creds
    ```

   Please refer to `velero-plugin-for-microsoft-azure` documentation for instruction to [set up permissions for Velero](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure#set-permissions-for-velero) and to [set up azure storage account and blob container](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure#setup-azure-storage-account-and-blob-container)

##  5. <a name='Codeundertest'></a>Code under test

By default, the e2e tests will run against the `velero/velero:main` version of Velero. If you would like to test the code with any other image of Velero, pass the `<registry/image:version>` value to the `VELERO_IMAGE` paramenter to the `make test-e2e` command.

##  6. <a name='Addingresourcesandtests'></a>Adding resources and tests

###  6.1. <a name='Addresources'></a>Add resources
It is common for tests to need to create object resources in a cluster.

These resources can be created using the `Kubebuilder` API client (preferred) but it can also be created by running the `kubectl` command against `yaml` files. If you use new `yaml` files, create a new folder for the test under the [testdata](/testdada) directory and add them in there.

Resources created for the purpose of testing should be created in their own namespace, not in the namespace where Velero is installed.

Note: The Velero e2e suite already has functionality to create workloads, generate and load data onto it, and verify this data was properly restored. Any test that needs data can reuse this existing setup. Look for `Kibishii` related functionalaties.

###  6.2. <a name='Addalabeltoallresourcescreatedinatest'></a>Add a label to all resources created in a test
To facilitate cleanup in a cluster when things go awry, please add a label to every resource your test creates. The label should have `velero-e2e` as the key, and a unique name as the value. Example:

`e2e:multiple-namespaces`

In the [common.go](common.go) file there is a helper function `addE2ELabel` to which you can pass any object and value for the label.

If you are using `yaml` files, add the`velero-e2e=<name>` label to the manifests.

###  6.3. <a name='Cleaninguptestdata'></a>Cleaning up test data
Please clean up any created test data after each test run. For example, if namespaces were created (and properly labeled), add this to the `AfterEach` block:

```
err = deleteNamespaceListWithLabel(client.ctx, client, labelValue)
Expect(err).NotTo(HaveOccurred())
```

###  6.4. <a name='Utilcode'></a>Util code

Please look at the files [common.go](common.go) and [velero_utils.go](velero_utils.go) for functionality that you will likely need.

###  6.5. <a name='APIclients'></a>API clients
When adding a test, aim to instantiate an API client only once at the beginning of the test. There is a constructor `newTestClient` that facilitates the configuration and instantiation of clients.

Also, please use the `Kubebuilder` runtime controller client for any new test, as we will phase out usage of `client-go` API clients.

Finally, use the `context.Context` object that is instantiated with the `newTestClient` to pass it through to the tests.

###  6.6. <a name='TipsTroubleshooting'></a>Tips / Troubleshooting

1) Logs

    Look for the ⛵ emoji printed at the end of each install and uninstall log. There should not be two install/unintall in a row, and there should be tests between an install and an uninstall.

2) Clean up dangling resources

    If you had to stop a test that created resources and you need to manually delete them, the easiest way is to use the label added when creating resources.

    To see a list of namespaces and their labels:

    `kubectl get ns -A --show-labels`

```
    NAME                       STATUS   AGE    LABELS

    multiple-namespaces-llpn   Active   24m    velero-e2e=multiple-namespaces
```


    To delete a specific label:

    `kubectl delete ns -l e2e=multiple-namespaces`

    To delete all resources created with the `velero-e2e` label

    `kubectl delete ns -l velero-e2e`
