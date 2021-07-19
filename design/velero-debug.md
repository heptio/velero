# `velero debug` command for gathering troubleshooting information

## Abstract
To simplify the communication between velero users and developers, this document proposes the `velero debug` command to generate a tarball including the logs needed for debugging.

Github issue: https://github.com/vmware-tanzu/velero/issues/675

## Background
Gathering information to troubleshoot a Velero deployment is currently spread across multiple commands, and is not very efficient. Logs for the Velero server itself are accessed via a kubectl logs command, while information on specific backups or restores are accessed via a Velero subcommand. Restic logs are even more complicated to retrieve, since one must gather logs for every instance of the daemonset, and there’s currently no good mechanism to locate which node a particular restic backup ran against.  
A dedicated subcommand can lower this effort and reduce back-and-forth between user and developer for collecting the logs.


## Goals
- Enable efficient log collection for Velero and associated components, like plugins and restic.

## Non Goals
- Collecting logs for components that do not belong to velero such as storage service.
- Automated log analysis.

## High-Level Design
With the introduction of the new command `velero debug`, the command would download all of the following information:
- velero deployment logs
- restic DaemonSet logs
- Plugin logs - need clarification for vSphere plugin see open quetions
- Resource and log of the backup and restore, if specified in the param
- Resources:
  - BackupStorageLocation
  - PodVolumeBackups
  - PodVolumeRestores

A project called `crash-diagnostics` (or `crashd`) (https://github.com/vmware-tanzu/crash-diagnostics)  implements the Kubernetes API queries and provides Starlark scripting language to abstract details, and collect the information into a local copy.   It can be used as a standalone CLI executing a Starlark script file.
With the capabilities of embedding files in Go 1.16, we can define a Starlark script gathering the necessary information, embed the script at build time, then the velero debug command will invoke `crashd`, passing in the script’s text contents.

## Detailed Design
### Triggering the script
The Starlark script to be called by crashd:

```python
def capture_backup_logs():
    if args.backup:
        kube_capture(what="objects", kinds=['backups'], names=[args.backup])
        backupLogsCmd = "velero backup logs {}".format(args.backup)
        capture_local(cmd=backupLogsCmd)
def capture_restore_logs():
    if args.restore:
        kube_capture(what="objects", kinds=['restores'], names=[args.restore])
        restoreLogsCmd = "velero restore logs {}".format(args.restore)
        capture_local(cmd=restoreLogsCmd)

ns = args.namespace if args.namespace else "velero"
basedir = args.basedir if args.basedir else os.home
output = args.output if args.output else "bundle.tar.gz"
# Working dir for writing during script execution
crshd = crashd_config(workdir="{0}/velero-bundle".format(basedir))
set_defaults(kube_config(path=args.kubeconfig))
capture_local(cmd="velero version")
capture_backup_logs()
capture_restore_logs()
kube_capture(what="logs", namespaces=[ns])
kube_capture(what="objects", namespaces=[ns], kinds=['backupstoragelocations', 'podvolumebackups', 'podvolumerestores'])
archive(output_file=output, source_paths=[crshd.workdir])
```
The sample command to trigger the script via crashd:
```shell
./crashd run ./velero.cshd --args 
'backup=harbor-backup-2nd,namespace=velero,basedir=,restore=,kubeconfig=/home/.kube/minikube-250-224/config,output='
```
To trigger the script in `velero debug`, in the package `pkg/cmd/cli/debug` a struct `option` will be introduced
```go
type option struct {
  // workdir for crashd will be $baseDir/tmp/crashd
  baseDir string
  // the namespace where velero server is installed
  namespace string
  // the absolute path for the log bundle to be generated
  outputPath string
  // the absolute path for the kubeconfig file that will be read by crashd for calling K8S API
  kubeconfigPath string
  // optional, the name of the backup resource whose log will be packaged into the debug bundle
  backup string
  // optional, the name of the restore resource whose log will be packaged into the debug bundle
  restore string
}
```
The code will consolidate the input parameters and execution context of the `velero` CLI to form the option struct, which can be transformed into the `args` string for `crashd`
### kubeconfig
When it comes to accessing the API of k8s, `crashd` has a limitation that it can only accept a path of kubeconfig file, without customizing the `context`, and it does not honor the environment variables such as `KUBECONFIG`.  `velero` does honor the environment variables and allow user to customize the path to kubeconfig and the `context`
There are two ways to make crashd have consistent behavior as velero in terms of getting the kube configuration:
1. Modify crashd to make it honor the environment variable and allow user to set context while calling k8s APIs.  This is a preferred approach and it does make `crashd` better, but it may take longer time because we need to convince the maintainers of `crashd`, and double check the change will not break their current use cases.
   There are 2 issues opened:
   https://github.com/vmware-tanzu/crash-diagnostics/issues/208
   https://github.com/vmware-tanzu/crash-diagnostics/issues/122
   I'll try to contact the maintainers of `crashd` to see the feasibility for velero v1.7
2. Before calling the `crashd` script velero CLI will use `client-go` to generate a temp `kubeconfig` file honoring the environment variable and global flags, and pass it to crashd.  Although there’s no permission elevation and the temp file will be removed, there’s still some security concern because the temp file is accessible by other programs before it’s deleted, or it may not be deleted if an error happens.

Therefore, we should consider `option 1` the better choice, and see `option 2` as the backup.

## Alternatives Considered
The collection could be done via the kubernetes client-go API, but such integration is not necessarily trivial to implement, therefore, `crashd` is preferred approach


## Security Considerations
- The current released version of `crashd` depends on `client-go v0.19.0`  which has a known CVE, we need to make sure that when it’s compiled into velero it uses the version that has the CVE fixed.  We should  write a PR or push crashd maintainer to fix the CVE-2021-3121 in 0.19.0
- The starlark script will be embedded into the velero binary, so there’s little risk that the script will be modified before being called.
- There may be minor security issues if we choose to create a temp `kubeconfig` file for `crashd` and remove it afterwards.  If we have to choose this option, we need to review it with security experts to better understand the risks.

## Compatibility
As the `crashd` project evolves the behavior of the internal functions used in the Starlark script may change.  We’ll ensure the correctness of the script via regular E2E tests.


## Implementation
1. Bump up to use Go v1.16 to compile velero
2. Embed the starlark script
3. Implement the `velero debug` sub-command to call the script
4. Add E2E test case

## Open Questions
- **Log collection for vsphere plugin:**  Per the design of vsphere plugin: https://github.com/vmware-tanzu/velero-plugin-for-vsphere#architecture when user backup resource on a guest cluster the code in component in the supervisor cluster may be called.  Per discussion in v1.7 we will only support collecting logs of process running in one k8s cluster.  In terms of implementation, we will do investigate the possibility to call extra script in crashd and ask vsphere plugin developer to provide a script to do the log collection, but the details remain TBD.
- **Command dependencies:** In the Starlark script, for collecting version info and backup logs, it calls the `velero backup logs` and `velero version`, which makes the call stack like velero debug -> crashd -> velero xxx.  We need to make sure this works under different PATH settings.
- **Progress and error handling:** The log collection may take a relatively long time, log messages should be printed to indicate the progress when different items are being downloaded and packaged.  Additionally, when an error happens, we need to double check if it’s omitted by crashd.
