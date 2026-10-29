## v1.19

### Download
https://github.com/velero-io/velero/releases/tag/v1.19.0

### Container Image
`velero/velero:v1.19.0`

### Documentation
https://velero.io/docs/v1.19/

### Upgrading
https://velero.io/docs/v1.19/upgrade-to-1.19/

### Highlights

#### Block data mover

Velero 1.19 introduces a block data mover (velero-block) for CSI snapshot data movement, enabling block-level backup and restore for both file system and block volumes backed by block storage. It integrates with the Kubernetes CSI Changed Block Tracking (CBT) API (https://kubernetes.io/blog/2025/09/25/csi-changed-block-tracking/) to transfer only changed blocks during incremental backups and restores with compatible CSI drivers, avoiding file system scanning overhead and improving backup and restore efficiency. For more information, check Velero document https://velero.io/docs/v1.19/csi-snapshot-data-movement/.    

#### Volume data in-place restore

Velero 1.19 introduces volume data in-place restore, allowing backup data to be restored directly into existing volumes. It supports both full and incremental restores for CSI snapshot data movement (velero-fs and velero-block) and File System Backup, with incremental restores skipping unchanged data to reduce data transfer and recovery time. It also supports clone-and-restore workflows, where users first clone a volume from a snapshot and then perform an in-place restore into the cloned volume. For more information, check Velero document https://velero.io/docs/v1.19/volume-data-inplace-restore/.  

#### Fine-grained filters

Velero 1.19 introduces fine-grained filters for backups and restores through ResourcePolicy ConfigMaps. Users can define per-namespace and per-resource-kind rules with independent label selectors and resource name patterns, providing more precise control over which namespaced and cluster-scoped resources are backed up or restored. For more information, check Velero document https://velero.io/docs/v1.19/fine-grained-backup-filters/ and https://velero.io/docs/v1.19/fine-grained-restore-filters/.  

#### Namespace selection by labels

Velero 1.19 supports dynamically including or excluding namespaces from backups based on namespace labels through ResourcePolicy ConfigMaps. This simplifies backup and schedule management by automatically selecting matching namespaces when each backup starts, without maintaining explicit namespace lists. For more information, check Velero document https://velero.io/docs/v1.19/resource-filtering/#namespace-selection-by-label-backup-only.   

#### Global volume policy

Velero 1.19 introduces global volume policies, allowing administrators to configure default volume backup rules for all backups through a shared ConfigMap. Backup-specific volume policies take precedence over global rules, providing consistent defaults while allowing customization for individual backups. For more information, check Velero document https://velero.io/docs/v1.19/resource-filtering/#global-backup-volume-policies.  

#### Custom action type in volume policy

Velero 1.19 adds a `custom` action type to volume policies, allowing third-party BackupItemAction plugins to handle volume backups using custom methods. Users can select volumes for custom handling and pass plugin-specific parameters alongside existing snapshot and file system backup policies. For more information, check the Velero document https://velero.io/docs/v1.19/resource-filtering/#custom-volume-policy-actions.  

#### Default restore resource modifier

Velero 1.19 allows administrators to configure a default restore resource modifier through a shared ConfigMap, automatically applying common resource transformations during restores. Individual restores can override the default with their own modifier or explicitly skip it, simplifying consistent restore customization. For more information, check the Velero document https://velero.io/docs/v1.19/restore-resource-modifiers/#default-resource-modifiers.  

### Runtime And Dependencies
<TBD>

### Limitations/Known Issues

#### Block data mover gap on snapshot preservation

Block data mover incremental backups and restores support CSI drivers that can compute changed blocks without keeping the parent backup's snapshot; the [CSI Changed Block Tracking KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/3314-csi-changed-block-tracking) allows drivers either way.  
Drivers that need the parent snapshot to still exist, for example Ceph RBD, are not supported in 1.19.0: their incremental backups read all allocated blocks instead of only the changed ones, and incremental restores run as full restores.  
Support for these drivers is tracked in https://github.com/velero-io/velero/issues/10743.  

#### CSI snapshot data movement restore limitation on pods with both Immediate and WaitForFirstConsumer PVCs
CSI snapshot data movement restoring a pod that has both Immediate and WaitForFirstConsumer PVCs may stall, and the restore goes PartiallyFailed. If you are not running a multiple-zone cluster with storage topologies, you could work around this issue by setting the node-agent configuration `restorePVC.ignoreDelayBinding: true`; otherwise, you should avoid mounting both Immediate and WaitForFirstConsumer PVCs to a single pod.  

### Breaking Changes

#### GitHub repository change

Velero's code repository and Go module path changed from github.com/vmware-tanzu/velero to github.com/velero-io/velero. Downstream projects that import Velero as a library must update their imports and go.mod. Container images and CLI usage are unchanged.   

#### Restic removal

Restic path for fs-backup is completely removed in 1.19. Therefore, Velero 1.19 marks restores from backups with Restic path as FailedValidation; backup sync ignores backups with Restic path, so they won't appear in the cluster even though they are kept as is in the backup storage; if there are existing backups with Restic path in the cluster, after upgrade to 1.19, deletion request of these backups will fail.   

#### Default prepare queue length

A default prepare queue length (5) is set for data movers, so that pending data mover pods won't occupy large amount of resources from the cluster. In the previous versions there is no limit. For more information about data mover prepare queue length, check Velero document https://velero.io/docs/v1.19/node-agent-prepare-queue-length/.  

#### Hashing algorithm for Kopia repository

Kopia repository's hashing algorithm is changed from `BLAKE2B-256-128` to `HMAC-SHA256-128` for security compliance. This change applies to new repositories only, existing repositories still use `BLAKE2B-256-128`.  

#### User-provided credentials file paths ignored

Velero 1.19 ignores user-provided `spec.config.credentialsFile` values in BackupStorageLocation (BSL) and VolumeSnapshotLocation (VSL). If you previously configured credentials this way, store them in a Kubernetes Secret in the Velero namespace and reference it using `spec.credential.name` and `spec.credential.key`.  

### All Changes
<TBD>