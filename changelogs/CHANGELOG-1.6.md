## v1.6.2
### 2021-07-16

### Download
https://github.com/vmware-tanzu/velero/releases/tag/v1.6.2

### Container Image
`velero/velero:v1.6.2`

### Documentation
https://velero.io/docs/v1.6/

### Upgrading
https://velero.io/docs/v1.6/upgrade-to-1.6/

This release contains no user facing changes but includes fixes for CVE-2021-3121 and CVE-2021-3580.

## v1.6.1
### 2021-06-21

### Download
https://github.com/vmware-tanzu/velero/releases/tag/v1.6.1

### Container Image
`velero/velero:v1.6.1`

### Documentation
https://velero.io/docs/v1.6/

### Upgrading
https://velero.io/docs/v1.6/upgrade-to-1.6/

### All Changes

  * Fix CR restore regression introduced in 1.6 restore progress. (#3845, @sseago)
  * Skip the restore of volumes that originally came from a projected volume when using restic. (#3877, @zubron)
  * skip backuping projected volume when using restic (#3866, @alaypatel07)
  * 🐛 Fix plugin name derivation from image name (#3711, @ashish-amarnath)

## v1.6.0
### 2021-04-12

### Download
https://github.com/vmware-tanzu/velero/releases/tag/v1.6.0

### Container Image
`velero/velero:v1.6.0`

### Documentation
https://velero.io/docs/v1.6/

### Upgrading
https://velero.io/docs/v1.6/upgrade-to-1.6/

### Highlights

 * Support for per-BSL credentials
 * Progress reporting for restores
 * Restore API Groups by priority level
 * Restic v0.12.0 upgrade
 * End-to-end testing 
 * CLI usability improvements

### All Changes

  * Add support for restic to use per-BSL credentials. Velero will now serialize the secret referenced by the `Credential` field in the BSL and use this path when setting provider specific environment variables for restic commands.  (#3489, @zubron)
  * Upgrade restic from v0.9.6 to v0.12.0. (#3528, @ashish-amarnath)
  * Progress reporting added for Velero Restores (#3125, @pranavgaikwad)
  * Add uninstall option for velero cli (#3399, @vadasambar)
  * Add support for per-BSL credentials. Velero will now serialize the secret referenced by the `Credential` field in the BSL and pass this path through to Object Storage plugins via the `config` map using the `credentialsFile` key. (#3442, @zubron)
  * Fixed a bug where restic volumes would not be restored when using a namespace mapping. (#3475, @zubron)
  * Restore API group version by priority. Increase timeout to 3 minutes in DeploymentIsReady(...) function in the install package (#3133, @codegold79)
  * Add field and cli flag to associate a credential with a BSL on BSL create|set. (#3190, @carlisia)
  * Add colored output to `describe schedule/backup/restore` commands (#3275, @mike1808)
  * Add CAPI Cluster and ClusterResourceSets to default restore priorities so that the capi-controller-manager does not panic on restores. (#3446, @nrb)
  * Use label to select Velero deployment in plugin cmd (#3447, @codegold79)
  * feat: support setting BackupStorageLocation CA certificate via `velero backup-location set --cacert` (#3167, @jenting)
  * Add restic initContainer length check in pod volume restore to prevent restic plugin container disappear in runtime (#3198, @shellwedance)
  * Bump versions of external snapshotter and others in order to make `go get` to succeed (#3202, @georgettica)
  * Support fish shell completion (#3231, @jenting)
  * Change the logging level of PV deletion timeout from Debug to Warn (#3316, @MadhavJivrajani)
  * Set the BSL created at install time as the "default" (#3172, @carlisia)
  * Capitalize all help messages (#3209, @jenting)
  * Increased default Velero pod memory limit to 512Mi (#3234, @dsmithuchida)
  * Fixed an issue where the deletion of a backup would fail if the backup tarball couldn't be downloaded from object storage. Now the tarball is only downloaded if there are associated DeleteItemAction plugins and if downloading the tarball fails, the plugins are skipped. (#2993, @zubron)
  * feat: add delete sub-command for BSL (#3073, @jenting)
  * 🐛 BSLs with validation disabled should be validated at least once (#3084, @ashish-amarnath)
  * feat: support configures BackupStorageLocation custom resources to indicate which one is the default (#3092, @jenting)
  * Added "--preserve-nodeports" flag to preserve original nodePorts when restoring. (#3095, @yusufgungor)
  * Owner reference in backup when created from schedule (#3127, @matheusjuvelino)
  * issue: add flag to the schedule cmd to configure the `useOwnerReferencesInBackup` option #3176 (#3182, @matheusjuvelino)
  * cli: allow creating multiple instances of Velero across two different namespaces (#2886, @alaypatel07)
  * Feature: It is possible to change the timezone of the container by specifying in the manifest.. env: [TZ: Zone/Country], or in the Helm Chart.. configuration: {extraEnvVars: [TZ: 'Zone/Country']} (#2944, @mickkael)
  * Fix issue where bare `velero` command returned an error code. (#2947, @nrb)
  * Restore CRD Resource name to fix CRD wait functionality. (#2949, @sseago)
  * Fixed 'velero.io/change-pvc-node-selector' plugin to fetch configmap using label key "velero.io/change-pvc-node-selector"  (#2970, @mynktl)
  * Compile with Go 1.15 (#2974, @gliptak)
  * Fix BSL controller to avoid invoking init() on all BSLs regardless of ValidationFrequency (#2992, @betta1)
  * Ensure that bound PVCs and PVs remain bound on restore. (#3007, @nrb)
  * Allows the restic-wait container to exist in any order in the pod being restored. Prints a warning message in the case where the restic-wait container isn't the first container in the list of initialization containers. (#3011, @doughepi)
  * Add warning to velero version cmd if the client and server versions mismatch.  (#3024, @cvhariharan)
  * 🐛  Use namespace and name to match PVB to Pod restore (#3051, @ashish-amarnath)
  * Fixed various typos across codebase (#3057, @invidian)
  * 🐛  ItemAction plugins for unresolvable types should not be run for all types (#3059, @ashish-amarnath)
  * Basic end-to-end tests, generate data/backup/remove/restore/verify. Uses distributed data generator (#3060, @dsu-igeek)
  * Added GitHub Workflow running Codespell for spell checking (#3064, @invidian)
  * Pass annotations from schedule to backup it creates the same way it is done for labels. Add WithannotationsMap function to builder to be able to pass map instead of key/val list (#3067, @funkycode)
  * Add instructions to clone repository for examples in docs (#3074, @MadhavJivrajani)
  * 🏃‍♂️ update setup-kind github actions CI (#3085, @ashish-amarnath)
  * Modify wrong function name to correct one. (#3106, @shellwedance)
