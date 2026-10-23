# OwnerReference Relinking

## Glossary

- **UID mapping:** In-memory lookup from an object's backup UID to a record of its created or eligible existing target object's live UID, GroupKind, name, namespace, and discovered scope.
- **Pass 1:** Immediate ownerReference resolution and patching after synchronous restoration finishes. Its child API patch loop is bounded by `resourceTimeout` (default 10 minutes); pending-state persistence has a separate budget.
- **Pass 2:** Finalizer-side parent UID refresh and retry of persisted ownerReference patches, after the asynchronous restore-operation wait. Refresh each distinct parent once per execution; do not revisit children completed in Pass 1. It does not pause or unpause resources.
- **Pending patch:** A child's identity and already-resolved ownerReference slice persisted for retry without reconstructing the UID map.
- **Pending snapshot:** An immutable, Restore-owned ConfigMap containing the complete compressed pending queue. `PendingPatchesConfigMap` names the authoritative snapshot; `PendingOwnerRefPatchCount` reports its number of child requests.
- **Live precedence:** Preserve an existing live controlling owner and demote a different incoming restored controlling owner to avoid multiple controlling owners.

## Background

### OwnerReferences and Garbage Collection

An ownerReference identifies its parent by apiVersion, kind, name, and UID, with optional `controller` and `blockOwnerDeletion` flags. Newly created objects receive new UIDs during restore. Reusing backed-up owner UIDs can expose restored children to garbage collection because those original owners do not exist in the destination cluster.

Velero's `resetMetadataAndStatus()` strips `metadata.ownerReferences` before creation. This avoids dangling restored ownerReferences, but custom operators may not adopt the resulting orphans. Built-in workload controllers have established selector-adoption behavior that this design preserves through resource priorities and a deny list.

The restoration pipeline can create children before their parents. Rather than imposing a topological order, this design creates objects without ownerReferences and relinks them once synchronous restoration finishes and eligible parent UIDs are known.

### Controller Races Are an Operational Boundary

Active controllers can observe restored resources before their ownership relationships are established. Relinking alone does not prevent duplicate provisioning or other controller mutations during this interval. Workloads that cannot tolerate this intermediate state require external controller suspension; see [External Pause and Resume Procedure](#external-pause-and-resume-procedure) for the race scenario and operational steps.

## Goals

- Relink allowlisted Kubernetes ownerReferences using data already present in existing backup archives.
- Preserve the current resource priority order and deny-listed workload adoption behavior.
- Resolve references during Pass 1 and persist transient failures for restart-tolerant Pass 2 retries.
- Merge restored ownerReferences with live references under optimistic concurrency control.
- Persist pending patches in immutable, Restore-owned ConfigMap snapshots, preserving recoverability until replacement status is durable.
- Gate the feature with `--features=OwnerRefRelink`, with baseline and per-restore ConfigMap scope configuration.
- Expose the pending request count and snapshot reference in Restore status and CLI output, alongside relink counts and diagnostics.

## Non-Goals

- Backup-side graphs, sidecar manifests, archive changes, or global topological sorting.
- Relinking or rewriting UID-bearing object references within resource `.spec`.
- Relinking or rewriting references within resource `.status`.
- Automated controller pause injection or unpause orchestration (such as pause tracking labels, paused-object status, and leftover-pause catch-up).
- Parent-to-root dependency graphs, transitive root pinning, or per-workload unquiesce evaluation.
- Relinking pre-existing live objects through update, skip, or ServiceAccount merge paths. Their UIDs may be registered for newly created children.
- Relinking a Namespace's own ownerReferences. Namespaces participate only as eligible parents for other restored resources.
- Resuming a restore that crashes while its phase is `InProgress`.
- Sharding a pending queue across multiple ConfigMaps.
- Authorizing cross-namespace ownership. Namespaced owners must share their dependent's namespace; cluster-scoped dependents may have only cluster-scoped owners.

## Configuration Model

`Restore.Spec.OwnerRefConfigMap` (`--owner-ref-restore-configmap`) references an ownerReference scope ConfigMap in the Velero namespace. The server baseline is selected by `--owner-ref-configmap`, or the conventional `velero-ownerref-config` name.

The server baseline accepts `inScope`. A per-restore ConfigMap accepts `inScope` and optional `outOfScope`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: velero-ownerref-config
  namespace: velero
data:
  inScope: |
    # Entire API group: omitting 'kind' includes all kinds in that group
    - group: cluster.x-k8s.io
    # Specific kind: restricts matching to this GroupKind only
    - group: controlplane.cluster.x-k8s.io
      kind: KubeadmControlPlane
    - group: bootstrap.cluster.x-k8s.io
      kind: KubeadmConfig
    # Entire API groups without kind
    - group: infrastructure.cluster.x-k8s.io
    - group: ipam.cluster.x-k8s.io
    - group: addons.cluster.x-k8s.io
```

Each entry in either section matches an API group and optional kind; matching ignores version. Core resources use an empty group with an explicit kind. Baseline and per-restore `inScope` entries are unioned and deduplicated, then per-restore `outOfScope` matches are excluded. Exclusions always win, including group-wide exclusions against exact-kind inclusions. The built-in child restrictions always apply: denied workloads and Namespaces cannot be selected for relinking their own ownerReferences. There are no pause rules or field-path override semantics in this design.

Both sections target the **child (dependent)** resource that carries `metadata.ownerReferences`, not the referenced parent:

- **Allowlisted child (in scope):** When a newly created resource matches `inScope`, is neither excluded nor denied, and contains backed-up ownerReferences, its references are stripped prior to creation and the object is enqueued for relinking in Pass 1 (with Pass 2 durable retry if needed).
- **Unlisted or excluded child (legacy behavior):** When a restored resource is not in `inScope` or matches `outOfScope`, Velero preserves its legacy strip behavior—`metadata.ownerReferences` remain stripped and the resource is never enqueued for relinking. The resource is still restored normally.
- **Parent resources:** A parent resource does not need to be listed in `inScope` unless its own ownerReferences should also be relinked. Velero records old-to-new UID mappings for eligible restored resources before checking child scope. A parent matching `outOfScope` can therefore still supply its UID for relinking an included child.

An exclusion-only per-restore ConfigMap is valid. An absent or empty `outOfScope` list preserves the inclusion-only behavior; duplicate entries are harmless. Scope is resolved during restore initialization. Later ConfigMap edits do not cancel patches already persisted for Pass 2. Partial relinking may leave an incomplete ownership hierarchy, so exclusions are not a guarantee that controller reconciliation is safe.

Unsupported owner-reference configuration sections must produce validation errors rather than silently activating or ignoring deferred features.

### Feature Gating and Precedence

| Source | Behavior |
| :--- | :--- |
| Feature flag disabled, no ownerRef ConfigMap reference | Preserve legacy behavior; the relinking engine is dormant. |
| Feature flag disabled, Restore references an ownerRef ConfigMap | Fail validation before restore begins. |
| Explicit server baseline ConfigMap | Load from the Velero namespace; missing/unreadable baseline warns and proceeds with an empty baseline. Invalid content fails validation. |
| Server flag unset | Load conventional `velero-ownerref-config` from the Velero namespace; missing/unreadable storage warns and proceeds with an empty baseline. Invalid content fails validation. |
| Per-restore ConfigMap specified | Require a readable valid ConfigMap; union its `inScope` with baseline inclusions, then apply its `outOfScope` exclusions. Otherwise fail validation. |

Effective child scope is `(baseline inScope ∪ per-restore inScope) − per-restore outOfScope − built-in denied child kinds`. These child restrictions cover the workload deny list and `core/Namespace`; the PVC parent filter covers only the workload deny list. Reject `outOfScope` in any server baseline, including an empty section. Validate each source before merging; an exclusion cannot hide an invalid inclusion.

`NewScope()` starts with no configured inclusions or exclusions; the built-in child restrictions always apply. CAPI defaults are delivered in the baseline ConfigMap rather than hardcoded into the binary. PVCs are not included in the curated baseline.

### Scope and Safety Boundaries

| Resource class | Configuration rule | OwnerReference behavior |
| :--- | :--- | :--- |
| **Denied workloads**<br>(`core/Pod`, `apps/ReplicaSet`, `core/ReplicationController`, `batch/Job`) | Forbidden in `inScope` (validation error) | **Always stripped** (legacy behavior retained for controller adoption). Never relinked. |
| **Namespaces**<br>(`core/Namespace`) | Forbidden in baseline and per-restore `inScope`; harmless in per-restore `outOfScope` | **Always stripped on creation; never queued as children.** Existing Namespace ownerReferences remain untouched by the relinking engine. Eligible Namespace UID mappings remain available to other children, including PVCs. |
| **PersistentVolumeClaims**<br>(`core/PersistentVolumeClaim`) | Explicit per-restore `inScope` and not excluded; inclusion forbidden in server baselines | **Stripped by default.** When selected, relink eligible backed-up ownerReferences, omitting references to Pod, ReplicaSet, ReplicationController, and Job. |
| **Allowlisted resources**<br>(In-scope CRDs, or higher-level workloads like `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`) | Included in baseline or per-restore `inScope`, with no matching `outOfScope` entry | **Created stripped, then relinked** in Pass 1 / Pass 2 with resolved parent UIDs. |
| **Unlisted or excluded resources** | Omitted from `inScope` or matched by per-restore `outOfScope` | **Preserves legacy strip behavior.** Restored normally, stripped prior to creation, and never enqueued for relinking; eligible parent UID mappings remain available. |

PVC relinking supports operator-managed storage lifecycles, such as KubeVirt DataVolume-owned disks and Tekton PipelineRun/TaskRun-owned workspaces. Restoring these relationships preserves ownership-based cleanup when the owner is deleted; leaving them stripped makes ownership repair dependent on operator adoption or manual intervention.

This design requires explicit per-restore selection as a conservative configuration policy: enabling the feature or configuring a server baseline must not automatically enable PVC relinking across restores. Selection covers all newly created PVCs included in that restore, not only those belonging to an allowlisted operator. Parents need not appear in `inScope`, so eligible backed-up StatefulSet ownership may also be restored.

The PVC parent filter is a separate boundary. References to Pod, ReplicaSet, ReplicationController, and Job are omitted to avoid restoring PVC deletion relationships tied to workloads whose ownership this design leaves to native controller reconciliation.

PVC opt-in restores only eligible ownership found in the backup and can therefore restore the workload's original deletion behavior. It does not invent an ownerReference absent from the backup or change a StatefulSet's `persistentVolumeClaimRetentionPolicy`.

For StatefulSet template PVCs, the default `Retain` policy normally leaves them without StatefulSet/Pod deletion ownerReferences. With `whenDeleted: Delete`, the controller uses StatefulSet ownership to implement PVC deletion; this design may relink that backed-up relationship. With `whenScaled: Delete`, the controller uses Pod ownership for replicas being scaled down; this design omits backed-up Pod references under its PVC parent deny-list filter. Disabling PVC relinking does not guarantee retention: the StatefulSet controller can independently reconcile these references according to its restored policy. See [Kubernetes StatefulSet PVC retention](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#persistentvolumeclaim-retention).

Administrators must assess the backed-up owners and target retention policies before opting in; enabling the feature alone must not activate PVC relinking.

PVC inclusion is a source-specific restriction. Reject `group: ""`, `kind: PersistentVolumeClaim` in any server baseline, including a custom baseline, during configuration validation. Accept it in per-restore `inScope`, where it explicitly opts newly created PVCs into relinking unless matched by `outOfScope`. Validate each configuration source before unioning entries so the opt-in cannot be inherited from the baseline or an invalid baseline hidden by exclusions.

For example, a separate per-restore ConfigMap may contain:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: restore-ownerrefs
  namespace: velero
data:
  inScope: |
    - group: ""
      kind: PersistentVolumeClaim
  outOfScope: |
    - group: cluster.x-k8s.io
      kind: Machine
```

Select it with `--owner-ref-restore-configmap restore-ownerrefs`. This adds PVC relinking and excludes Machine children even if the baseline includes the whole `cluster.x-k8s.io` group. Machines are still restored and may supply parent UIDs for other children. Other baseline inclusions remain effective. The PVC parent deny-list filter still applies, and existing PVCs are never enqueued for relinking.

## Restore Workflow

```mermaid
flowchart TD
    Input["Read backup object"] --> Stash["Stash original UID and ownerReferences"]
    Stash --> Sanitize["Strip ownerReferences and reset metadata"]
    Sanitize --> Modifiers["Run existing item actions and ResourceModifiers"]
    Modifiers --> Create["Create target object"]
    Create --> Register["Record UID mapping and queue allowlisted child"]
    Register --> Pass1["After synchronous restore: resolve and patch ownerReferences"]
    Pass1 --> Prepare1["Prepare complete pending snapshot and count"]
    Prepare1 --> Commit1["Persist pending work and transition Restore phase"]
    Commit1 --> Wait["Existing asynchronous operation wait, if any"]
    Wait --> Pass2["Finalizer refreshes pending parent UIDs and retries patches"]
    Pass2 --> Prepare2["Prepare replacement snapshot or clear reference/count"]
    Prepare2 --> Ready{"Preparation succeeded?"}
    Ready -->|No| Retry["Preserve durable queue and retry reconciliation"]
    Ready -->|Yes| Commit2["Commit final status with resourceVersion precondition"]
    Commit2 --> Cleanup["Delete owned unreferenced snapshots"]
    Cleanup --> Done["Completed or PartiallyFailed; external resume procedure"]
```

### Synchronous Restoration: Create and Register UIDs

In `restoreItem()`, retain `itemFromBackup` unchanged and copy its original UID and ownerReferences before `resetMetadataAndStatus()`. Continue running existing restore item actions and ResourceModifiers on `obj`.

Immediately after a successful `resourceClient.Create(obj)`, register the backup UID to created UID mapping and enqueue ownerReferences for an allowlisted child. Capture target identity from the actual created object, including its final GVK, name, namespace, and created UID. Store that created UID as `RestoredUID` on queued and persisted requests. Register before status updates, managedFields patches, or hooks that may return early.

Each mapping stores a `UIDMappingRecord` as defined below, rather than only a UID pair. Capture the parent's actual target GroupKind, destination API version, served-version snapshot, name, namespace, and discovered scope for every registered resource, including resources outside the child allowlist. Obtain scope from restore discovery/RESTMapper information during synchronous restoration and cache it in the record; capture the child's scope on its queued request as well. If scope cannot be established, keep `ScopeKnown: false` and diagnose affected relationships as omissions. An empty namespace alone must not be treated as proof of cluster scope.

Record eligible existing parent identities and UIDs before sanitizing the live object's metadata. Update, ServiceAccount merge, and equal-skip paths register target identity records only; they do not enqueue ownerReference patches against existing objects. Where the selected restore mode supports restoring volume data into an existing PVC/PV, register the existing PVC's backup UID to target record while leaving its ownerReferences unchanged. This registration lets newly created children refer to that PVC; it does not certify that volume data restoration succeeded. A different object skipped without an update policy is not mapped by this design. Failed creation is a normal restore error; external readiness checks must account for missing children.

Namespaces are never queued for ownerReference relinking. Newly created Namespaces retain the existing strip behavior; existing Namespace ownerReferences remain unchanged by the relinking engine. During namespace ensure/create, register the backup-to-live UID mapping for newly created or eligible existing Namespaces so other restored resources can resolve explicit Namespace ownerReferences. Namespaces synthesized without an archive entry have no backup UID to register. 

No parent graph or quiesced-root index is built. UID mappings are collected for eligible resources even when those resources are not themselves in the child relink allowlist.

### Worked Example: Child Created Before Parent

Assume the backup contains a namespaced `WidgetSet` named `workers` with UID `parent-old` and an allowlisted `Widget` named `worker-1` whose ownerReference points to that WidgetSet. Both kinds belong to `workloads.example.io`; names, kinds, and namespaces remain unchanged during this restore.

1. Restore `worker-1` first with ownerReferences stripped. Its created UID is `child-new`; record its UID mapping and queue its original ownerReference with `RestoredUID: child-new`.
2. Restore `workers` later with UID `parent-new`. Record `uidMap[parent-old]` with live UID `parent-new`, GroupKind `workloads.example.io/WidgetSet`, name `workers`, its target namespace, and known namespaced scope, even if WidgetSet is outside the child relink allowlist.
3. After synchronous restoration finishes, Pass 1 resolves the queued owner UID to `parent-new`, GETs `worker-1`, and merges and patches its ownerReferences using the fetched resourceVersion.
4. If the patch succeeds, count one relinked child and remove the request. If it fails transiently, persist the child's target identity, `RestoredUID`, and the resolved ownerReference with UID `parent-new` for Pass 2.

The child can therefore be restored before its parent without changing resource priorities. Pass 2 needs only the persisted request, not the original UID map. Controller suspension, where needed, remains an external responsibility throughout this sequence.

### Pass 1: Immediate OwnerReference Relinking

`RestoreController` invokes `ApplyOwnerRefRelinking()` immediately after `RestoreWithResolvers` finishes, before phase transition and result persistence. RestoreItemAction additional items restored through the synchronous item loop participate in UID mapping and enqueueing.

The UID map, including target identities and cached scope information, is frozen when synchronous `RestoreWithResolvers` processing finishes, including AdditionalItems restored through that synchronous item loop. Pass 1 resolves and validates against those completed records. Objects created later by asynchronous operations or live controllers are not added dynamically. The freeze is a Pass 1 snapshot boundary. Pass 1 performs no additional parent GETs before patching. Pass 2 may refresh UIDs for established parent targets referenced by pending requests, including same-identity replacements; it does not expand the eligible relationship set.

For each queued child:

1. Resolve each original owner UID through the in-memory UID map.
2. Omit missing/excluded owners with warnings. Verify owner identity, cached scope, and actual target namespace against the frozen mapping record; omit invalid owner relationships with diagnostics.
3. Fetch the live child and merge references resolved from the frozen map with its current ownerReferences. Do not GET parents or refresh their UIDs in Pass 1.
4. Patch with a resourceVersion precondition and bounded retries.
5. Count successful child patches in `OwnerRefsRelinked`; collect transient failures as pending requests containing already-resolved references.

The child patch loop is bounded by `resourceTimeout`. Once its context expires, resolve and validate the remaining queue entries from the frozen records without further child/parent API calls, discovery calls, or repeated backoff sleeps. Persist the resulting pending payloads under a separate bounded persistence context as described below. No spec GET, spec traversal, or fallback spec-payload fetch is needed.

Before leaving `InProgress`, persist all remaining requests in one immutable ConfigMap snapshot. The pending state becomes restart-recoverable only once the Restore phase and snapshot reference/count are committed.

### Pass 2: Durable Finalizer Retry

`RestoreFinalizerController` runs relink retries in `Finalizing` or `FinalizingPartiallyFailed`, after the existing asynchronous-operation wait and finalization tasks. This scheduling does not assert that every volume operation succeeded; external resume checks remain responsible for workload readiness.

Load the complete queue from the referenced snapshot and validate its count against status. A positive count without a reference, or any mismatch between status and payload count, is a persistence error. Always inspect a referenced snapshot even when the status count is zero. Missing, unreadable, empty, or corrupt referenced storage must fail closed: retain the reference and pending state, never treat partial loading as an empty queue, and report failure. Before patching children, build an execution-local parent UID snapshot from the complete pending queue:

1. Deduplicate established parent targets by GroupKind, target namespace, and name, ignoring API version. Discover scope so namespaced parents are read in the child's actual target namespace and cluster-scoped parents are read without a namespace. If the recorded API version is unavailable, use a served version of the same GroupKind for both lookup and the emitted ownerReference, following the emitted-version rule below.
2. GET each distinct parent at most once per execution. Cache both successful results and failures. A same-identity replacement supplies its current UID and produces a replacement warning; do not require equality with the previously persisted UID. Missing, deleting, or UID-less parents are unavailable, so affected children remain pending without being patched. Permanent lookup/scope errors fail the affected requests; transient lookup failures leave them pending.
3. Rewrite all pending ownerReferences from the refreshed snapshot before child patching. If one owner cannot be refreshed, do not patch that child. Preserve successful refreshes for its other owners in the retained payload. A permanent failure on any owner takes precedence over unavailable/transient owners.
4. Patch eligible children with fresh child GETs and the normal merge/resourceVersion retry behavior. Conflict retries and the child Forbidden fallback reuse the snapshot without additional parent GETs. For example, 100 pending children with one shared parent require one parent GET.
5. Persist refreshed ownerReferences in remaining requests through the existing copy-on-write snapshot protocol. Never mutate the previously loaded durable queue in place. A subsequent finalizer reconciliation or restart refreshes parents again from its loaded pending work.

Pass 2 does not reconstruct or persist the original backup-UID map. The execution-local mapping contains only established targets in pending references. Owners omitted in Pass 1, excluded parents, and new relationships are not discovered. Successfully patched Pass 1 children are not retained or revisited.

This is a bounded retry contract, not a hierarchy-wide reconciliation guarantee. If Pass 1 completed C1 against parent UID A and Pass 2 refreshes pending C2 to replacement UID B, C1 remains untouched. A parent replacement after the Pass 2 snapshot is likewise outside the guarantee. Parent reads are not an atomic cluster snapshot, and relinking does not recover children already deleted by garbage collection; controller suspension remains an operational responsibility.

Pass 2 parent discovery and child patching share the existing `resourceTimeout` budget (default 10 minutes). At the deadline, retain unresolved and unprocessed requests without further parent, discovery, or child API calls. Parent lookup failures are cached for this execution rather than retried per child. Successful child patches drain, non-retriable failures become restore errors and leave the retry queue, and unavailable parents or exhausted transient failures remain pending. Remaining work follows the existing terminal `PartiallyFailed` contract; it does not introduce an indefinite polling loop.

Prepare the resulting pending queue with copy-on-write storage. Preparation failures return a reconciliation error, preserve the previous durable queue, and avoid terminal status changes. After successful preparation, persist replay-safe results, commit the snapshot reference/count, other counters, and final phase to the Restore, then clean up obsolete storage.

Failure to load the complete durable queue also returns a reconciliation error and preserves the previous phase and pending state. These storage failures retry through reconciliation. By contrast, remaining child patches after bounded retries do not keep finalization running indefinitely: once their snapshot reference/count is safely committed, the Restore becomes terminal `PartiallyFailed` with those requests retained for external remediation. Subsequent terminal reconciliations only retry snapshot cleanup; they do not replay ownerReference patches or automatically reopen the Restore.

Pass 2 remains necessary even without automatic unpause: it retries Pass 1 timeouts and transient failures using persisted payloads across an asynchronous wait or pod restart. A restore with no pending work proceeds through ordinary finalization and snapshot cleanup without an additional relink patch loop.

### Timeout and Persistence Contexts

In both passes, the child patch loop and pending-state persistence have separate deadline budgets. Derive each context from the active restore/reconciliation context, not from an expired child patch context. Parent cancellation or server shutdown still cancels the operations; do not detach them with an uncancellable background context.

- **Pass 1:** Start a child patch context bounded by `resourceTimeout`. When patching ends, start a fresh persistence context bounded by `resourceTimeout` for pending snapshot preparation, result persistence, and the phase/status commit. Expiry of the patch budget alone must not prevent saving pending requests.
- **Pass 2:** Load the complete durable queue under its own bounded storage context before starting the shared parent-refresh and child-patch budget. When patching ends or its deadline expires, start a fresh persistence context bounded by `resourceTimeout` for replacement snapshot preparation, result persistence, and the terminal phase/status commit.
- **Cleanup:** Run post-commit or terminal-reconciliation cleanup with a fresh bounded context. Its failure follows the existing warning/requeue contract and does not reopen relinking.

Each fresh budget uses the existing `resourceTimeout` setting (default 10 minutes); no new timeout option is introduced. These are per-stage bounds, so the whole relinking/finalization sequence may take longer than one `resourceTimeout`. A persistence timeout follows the normal storage failure contract: Pass 1 reports unsaved pending requests as errors if a subsequent status commit is possible, while a failed status commit has no `InProgress` recovery guarantee. Pass 2 preserves the previous durable queue and retries reconciliation; ambiguous commit failures retain snapshots until durable status is reloaded.

### Results and Completion Contract

Missing or excluded owners and deliberately omitted invalid relationships produce warnings and are not retried indefinitely. Actual non-retriable patch failures produce restore errors rather than a paused-root flag. They are omitted from the retry queue but force `PartiallyFailed`. Dropped pending work also records errors and forces `PartiallyFailed`.

A restore can be `Completed` only when its normal error criteria are satisfied and `PendingOwnerRefPatchCount` is zero and `PendingPatchesConfigMap` is empty. Existing volume, creation, hook, and other restore failures remain failures even if relinking drains. Remaining pending requests force `PartiallyFailed` and are retained for inspection.

`Completed` does not certify that every original ownerReference was restored: excluded/missing-owner omissions remain visible through warnings. It also does not mean that controllers were resumed or that unchanged UID-bearing spec references are usable.

## External Pause and Resume Procedure

### Why Controller Suspension May Be Needed

The restore workflow creates resources without their original ownerReferences, then reconstructs eligible ownership relationships after synchronous restoration finishes. Requests that cannot complete in Pass 1 remain pending for Pass 2. During this interval, controllers can observe restored resources before their ownership relationships are established.

For example, suppose parent P owns child C in the backup. Velero creates C without its ownerReference and later restores P. If P's controller reconciles before relinking completes, it may not recognize C as an existing dependent and may create another child, C′. Relinking C afterward does not automatically remove C′ or undo any provisioning it triggered.

Whether this occurs depends on the controller's discovery and adoption behavior. Workloads whose controllers cannot safely reconcile this intermediate state need controller suspension before restoration exposes incomplete resources. Suspension must cover every relevant controller; a pause annotation on the root resource may not be sufficient.

Pause and resume remain external operational responsibilities. Use controller-supported pause mechanisms or suspend the relevant controllers, then resume only after checking restore results, pending patches, ownership relationships, and data restoration. A `Completed` Restore alone is not sufficient evidence that resuming reconciliation is safe.

### ResourceModifier Example

Create a separate ResourceModifier ConfigMap to inject a pause annotation. A merge patch preserves existing annotations and handles an absent annotations map:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: restore-controller-pauses
  namespace: velero
data:
  resource-modifiers.yaml: |
    version: v1
    resourceModifierRules:
      - conditions:
          groupResource: clusters.cluster.x-k8s.io
          namespaces:
            - source-workloads
        mergePatches:
          - patchData: |
              metadata:
                annotations:
                  cluster.x-k8s.io/paused: ""
```

ResourceModifier conditions match the original backup namespace, before namespace mapping. Existing restore priorities place CAPI Clusters late, after Tanzu ClusterBootstraps; this design retains those priorities. This example demonstrates injection only; its controller coverage must be checked for the installed CAPI providers.

```bash
velero restore create cluster-restore \
  --from-backup cluster-backup \
  --owner-ref-restore-configmap velero-ownerref-config \
  --resource-modifier-configmap restore-controller-pauses
```

Use this procedure for fresh-resource DR restores. ResourceModifiers also feed existing-resource update paths, so `existingResourcePolicy: update` can apply pause annotations to running objects. The relinking engine cannot enforce create-only pause behavior for an external modifier.

### Operational Responsibilities

1. Record intentionally pre-existing pauses and the exact resources the procedure will temporarily pause. Velero does not distinguish those cases or maintain a pause inventory.
2. Establish controller suspension before reconciliation can act on incomplete resources. ResourceModifier injection occurs before Create, but root-only rules may be insufficient; use provider-specific suspension where needed.
3. Suspend GitOps or configure it to preserve the externally injected pause annotations during restore.
4. Wait for restore finalization, then inspect restore errors, warnings, pending patches, missing children, and the relevant volume-operation results. `Completed` alone does not prove that every original relationship was relinked or that the workload is ready.
5. Remove only the temporary pauses introduced by the external procedure after dependencies and data restoration are verified. Preserve intentional pauses.

For a verified temporary CAPI pause, external remediation may use:

```bash
kubectl annotate clusters.cluster.x-k8s.io -n target-workloads prod-cluster cluster.x-k8s.io/paused-
```

Velero never runs that command or equivalent unpause patches. Failure, restart, storage exhaustion, cancellation, and Restore deletion leave modifier-injected annotations in place until an external actor changes or deletes the resource.

## OwnerReference Patch Mechanics

### Emitted OwnerReference API Version

Resolve backup-to-destination API-version differences as well as UIDs. For each managed ownerReference, preserve the matching live reference's `apiVersion` when destination discovery identifies it as served; otherwise preserve the resolved incoming reference's version when served; otherwise use the registered/resolved parent's destination-served version. Keep GroupKind and name unchanged. Preserve unrelated live references without rewriting them. Do not switch versions merely because another served version is preferred.

During synchronous registration, retain the parent's actual destination `APIVersion` and a snapshot of `ServedAPIVersions` from RESTMapper metadata. If that discovery is unavailable, retain the known destination version from the successful restore request as the conservative fallback; do not assume other versions are served. Pass 1 resolves backup references using this frozen metadata without additional parent GETs or discovery during child patching, including after the patch deadline. Save the selected version alongside the resolved UID in pending ownerReferences.

Pass 2 normally keeps that persisted version. Its existing scope discovery supplies served versions for the same GroupKind; legacy pending references using an unavailable version fall back to a served destination version for both the parent GET and the emitted ownerReference. Cache discovery per GroupKind and parent GET outcomes per GroupKind/namespace/name. Child conflict retries and the Forbidden fallback reuse the version snapshot, with no per-patch discovery or parent revalidation. Persist successful version corrections together with UID updates in remaining requests through copy-on-write storage.

The motivating case is a backup referencing `example.io/v1` restored into a destination serving only `example.io/v2`: both passes emit `v2`. If both versions remain served, preserve the applicable existing version. Concurrent API-version removal during a restore is not a supported migration workflow; successful Pass 1 children are not revisited. No new Restore API fields are needed, and this rule does not rewrite spec references.

Regression coverage must assert emitted `apiVersion` and UID for removed backup versions, stale matching live versions, preservation of served non-preferred versions, and pending-state persistence/replay. Continue asserting zero parent GETs in Pass 1 and at most one per distinct pending parent in Pass 2.

### Merge and Retry Rules

- Kubernetes merge patch replaces the entire ownerReferences array. Fetch live references, merge in memory, and send the unified array with the current `metadata.resourceVersion`.
- Match restored and live entries by Group, Kind, and Name, ignoring the API version suffix. On a match, keep the live apiVersion only when it is served according to the version snapshot; otherwise use the resolved incoming version. Update the resolved UID and applicable flags. Preserve unrelated live references.
- Preserve a live controlling owner. If a different restored owner requested `controller: true`, demote the incoming reference and warn; do not manufacture a second controlling owner or demote the matching live controlling owner.
- On an admission 403 with restored `blockOwnerDeletion: true`, strip that flag from incoming restored references and retry once. Preserve unrelated live owner flags. A second 403, or a 403 without that flag, is a non-retriable failure.
- Parent discovery occurs only once per distinct target in Pass 2, before child patching. Pass 1 uses frozen records. Child conflict retries and the Forbidden fallback perform no parent revalidation. A parent GET Forbidden is a lookup failure and must not trigger the child `blockOwnerDeletion` fallback.
- Retry conflicts with a fresh child GET and re-merge using the same pass-specific parent UID snapshot. Apply the failure classification below.
- Namespaced owners must remain in the dependent's actual target namespace. Namespaced children may have cluster-scoped owners; cluster-scoped children cannot acquire namespaced owners. Validate using scope cached from discovery during registration; unknown parent or child scope is an omission with a warning rather than an assumed cluster-scoped relationship.
- Before rewriting a UID, compare the backup ownerReference's GroupKind and name with the mapping record's actual target GroupKind and name. A version-only change is supported; a name or GroupKind change is omitted with a warning. This design does not rewrite owner names or kinds. Use the record's actual target namespace to validate ownership, including changes made by plugins/modifiers, rather than relying only on the configured namespace mapping. A missing Pass 1 mapping has no name-based fallback. Pass 2 lookup is limited to established parent identities already present in pending requests. Same-name child replacement is handled by the explicit policy below.

### Failure Classification

| Condition | Behavior |
| :--- | :--- |
| Source owner UID missing from the frozen map, including an excluded parent | Omit that backup reference, warn, and continue with any resolvable references. Do not create pending work for the omission. |
| Invalid or unknown owner/child scope, invalid target namespace, unsupported parent identity change, or a denied PVC parent | Omit the invalid reference with a warning; do not retry it as a temporary API failure. |
| Pass 2 finds a different UID at an established parent target | Refresh pending references to the discovered UID and warn. Do not revisit completed children. |
| Pass 2 parent GET returns NotFound, or the parent is deleting or has no UID | Cache the unavailable result and retain affected requests without patching them. Remaining pending work forces terminal `PartiallyFailed`. This differs from an owner never resolved in Pass 1. |
| Pass 2 parent scope discovery or GET fails | Cache the failure for this execution. Transient failures retain the request, including any successful refreshes of other owners. Permanent failures, including Forbidden or an unavailable kind, fail the child request without mutation and force `PartiallyFailed`. |
| Child GET/PATCH returns 409 Conflict | Retry with a fresh GET, reassess replacement identity, and re-merge. Retain the resolved request if bounded retries are exhausted. |
| Child GET/PATCH returns 429, a transient server error, a timeout, connection reset/refused, closed connection, probable EOF, or HTTP/2 connection loss; pass deadline expires | Retain the resolved request for Pass 2 after Pass 1, or in terminal pending state after Pass 2. Unprocessed resolved requests follow the same rule at the deadline. |
| Patch returns 403 Forbidden while incoming restored references request `blockOwnerDeletion: true` | Remove that flag from incoming references and retry once. Warn if the fallback succeeds; classify any remaining failure by its resulting error. |
| Child GET/PATCH returns 404 NotFound, 400 BadRequest, 422 Invalid, or persistent 403 Forbidden | Record a non-retriable restore error, remove the request from retry work, and force `PartiallyFailed`. A 403 without incoming `blockOwnerDeletion: true` is persistent Forbidden without a fallback. |

A missing child is an actual relink failure, unlike a missing source owner, which is an intentional omission. A later same-name recreation is eligible only if the child is present when a queued request is attempted; once a missing-child error removes that request, the relinking engine does not watch for a replacement or enqueue it again. Missing or corrupt pending storage follows the separate fail-closed recovery contract rather than being treated as a missing child.

### Multiple Owners and Partial Resolution

Resolve each entry in the child's original ownerReferences independently. If Parent A is mapped to its live UID and Parent B was excluded or is missing from the frozen UID map, include Parent A in the restored references, omit Parent B, and record a warning for Parent B. Do not discard the whole child's patch or create a retry request solely for the unresolved Parent B.

Merge the resolvable references with the child's current live references using the normal live-precedence rules. Omitting a backup reference does not authorize deleting an unrelated live ownerReference. If the patch itself fails transiently, persist only the resolved incoming references for Pass 2; the missing-parent omission remains a warning. If no original references resolve, perform no relink patch and preserve current live references.

### Replacement Child Policy

A child deleted and recreated under the same GroupKind, namespace, and name is treated as the current logical restore target. The relinking engine relinks its ownerReferences and reports the replacement as a warning rather than skipping it or failing solely because its UID changed.

Persist the originally created child UID in `RestoredUID`. In both passes, GET the current child before every patch attempt. If its UID differs from `RestoredUID`, record a restore warning identifying the expected and observed UIDs, then continue with the normal live-reference merge. Preserve unrelated live references and live controller precedence. Patch using the fetched object's resourceVersion; on conflict, GET again, reassess the UID, and rebuild the merge. Avoid duplicate warnings for the same observed replacement UID within a pass.

```text
Relinking ownerReferences on replacement Machine default/worker-1:
originally created UID A, current UID B.
```

Successful patching removes the request from pending work and counts as relink success. The restore may finish Completed with warnings; a UID change alone is not a restore error. Actual patch failures retain the normal retry/error behavior.

This is deliberate adoption: adding restored ownerReferences can change the replacement's garbage-collection relationships. External resume checks must inspect replacement warnings. The policy applies only to a child already created and queued by this restore; it does not enable general relinking of pre-existing objects. Parent UID refresh is separately limited to pending work in Pass 2; namespace/scope validation still applies. Neither replacement policy re-enqueues completed children.

## Data Structures and API Additions

```go
type RestoreSpec struct {
    // Existing fields omitted.
    OwnerRefConfigMap *corev1.TypedLocalObjectReference `json:"ownerRefConfigMap,omitempty"`
}

type RestoreStatus struct {
    // Existing fields omitted.
    PendingOwnerRefPatchCount int    `json:"pendingOwnerRefPatchCount,omitempty"`
    PendingPatchesConfigMap   string `json:"pendingPatchesConfigMap,omitempty"`
    OwnerRefsRelinked         int    `json:"ownerRefsRelinked,omitempty"`
}

type PendingPatchRef struct {
    Group           string                  `json:"group"`
    Version         string                  `json:"version"`
    Kind            string                  `json:"kind"`
    Namespace       string                  `json:"namespace"`
    Name            string                  `json:"name"`
    RestoredUID     types.UID               `json:"restoredUID"`
    OwnerReferences []metav1.OwnerReference `json:"ownerReferences,omitempty"`
}
```

Pending requests are ownerReference-only; `PatchType`, `SpecPatchJSON`, and `Targets` are unnecessary in the target schema. There are no `QuiescedObjectRef`, `TargetRef`, `QuiescedObjects`, `UnquiesceBlocked`, or `SpecRefsRelinked` fields in this design. Existing ownerReferences carry resolved parent UIDs and destination-served API versions, refreshed during Pass 2 for remaining work; no persisted UID-map field is needed. `RestoredUID` identifies the child originally created by this restore so replacement can be diagnosed; a mismatch follows the warn-and-relink policy above.

Internal state retains an enabled flag, resolved scope, UID map, and owner patch queue protected by mutexes. The map is keyed by backup UID and contains:

```go
// Internal restore state; not a Restore API or persisted snapshot field.
type UIDMappingRecord struct {
    LiveUID           types.UID
    GroupKind         schema.GroupKind
    APIVersion        string
    ServedAPIVersions []string
    Name              string
    Namespace         string
    Namespaced        bool
    ScopeKnown        bool
}

// uidMap map[types.UID]UIDMappingRecord
```

Populate each record from the created or eligible existing target object before metadata sanitization can erase its UID. `Namespaced` is meaningful only when `ScopeKnown` is true; an unknown scope never authorizes relinking. A queued request also retains its backup UID, original ownerReferences, and child scope (`Namespaced` and `ScopeKnown`) alongside its actual target identity. This supplies all identity/scope validation data in memory, including after Pass 1's patch deadline. Only validated, resolved ownerReferences enter `PendingPatchRef`; the registry and scope fields need not be persisted or reconstructed in Pass 2. There is no spec queue, parentMap, quiesced-root map, or paused-object inventory.

## Pending Storage and Crash Recovery

All pending payloads live in one immutable ConfigMap in the Restore namespace under `binaryData["patches.json.gz"]`. Status stores its name and `pendingOwnerRefPatchCount`. The snapshot contains an ordered array of `PendingPatchRef` values.

`pendingOwnerRefPatchCount` counts child patch requests retained in the authoritative snapshot, not individual ownerReferences or requests that failed to persist. Commit it together with the snapshot reference and phase. Zero requests require count zero and an empty reference; no empty ConfigMap is created. The count is a visibility summary, not permission to skip loading a nonempty reference. The loader validates ownership, immutable flag, ownership label, payload structure, and exact count agreement before any child work. Inconsistent or unreadable storage fails closed without advancing the phase.

### Capacity and Failure Contract

Set `MaxTotalPendingPatches = 10000` and enforce a preflight limit of 1 MiB for compressed payload plus key. These bounds apply to pending retry work, not the total number of restored resources. Every request retains its complete ownerReference array. The full queue must fit one snapshot; measure the compressed payload size before creation because request count alone does not determine storage size. [Kubernetes ConfigMap limits](https://kubernetes.io/docs/concepts/configuration/configmap/)

During Pass 1, retain at most the first 10000 requests and record explicit restore errors for requests beyond that cap. If validation, size preflight, or bounded snapshot creation fails, record errors for all unsaved requests and stage an empty reference/count. These errors force `PartiallyFailed`; count zero alone does not imply successful relinking. Crashes while `InProgress` are not resumed automatically.

Pass 2 never drops previously durable work when replacement preparation fails. Preserve the previous reference/count and snapshot, return a reconciliation error, and retry. A valid nonempty remainder becomes terminal `PartiallyFailed` with its snapshot retained for external remediation.

### Snapshot Commit and Cleanup Protocol

1. **Prepare:** For nonempty pending work, create an immutable `<restore-name>-pending-patches-<hash>` snapshot of the complete queue. Hash the Restore UID and compressed payload, truncate the name prefix within API limits, label it with `velero.io/pending-patches-restore-uid`, and set its Restore controller ownerReference. On `AlreadyExists`, verify ownership, immutability, label, and exact payload before reuse. In Pass 2, reuse the already-loaded and validated snapshot without additional storage calls if the remaining requests, including refreshed parent identities, are unchanged. For an empty remainder, stage an empty reference and count zero without deleting the previous snapshot.
2. **Commit:** Upload results before committing the snapshot reference, pending count, other counters, and phase to the Restore. Object storage and the Restore API do not form one atomic transaction. On every finalizer attempt, reload persisted results and append only messages not already present in the same severity and scope (Velero, cluster, or a specific namespace). Preserve existing messages, including duplicates from earlier restore stages. Reconcile status counters from the merged results without reducing recorded counts. Reload results even when this attempt has no messages, so an earlier error cannot disappear when replay succeeds. The Restore commit has a resourceVersion precondition; conflicts reload through reconciliation. Ambiguous API failures retain all snapshots. Snapshot A remains authoritative until the status referencing B, or clearing the reference and count, is durable.
3. **Cleanup:** After a successful terminal commit, use an uncached Restore GET, verify UID and terminal phase, and LIST snapshots by Restore UID label. Delete only owned snapshots other than the durable reference, with ConfigMap UID deletion preconditions. Terminal reconciliations retry cleanup after restart without replaying restore work. Cleanup failure warns and requeues after 30 seconds without changing the outcome.
4. **Retention:** Preserve a referenced snapshot while pending work remains. Only a drained queue clears the reference/count. A snapshot on a `PartiallyFailed` Restore remains available for remediation. Restore deletion garbage-collects owned snapshots.

One snapshot is authoritative at a time. Replacement and orphan snapshots may temporarily coexist until the status commit and cleanup complete.

### Storage Access and API Calls

Persisting and recovering a nonempty pending queue requires access to its ConfigMap. Inspecting individual requests requires fetching and decompressing the snapshot. Basic describe output reads the count and reference from Restore status, so it does not fetch the pending payload. Restore updates and watch events carry this summary alongside the other status fields.

With one successful Pass 1 and Pass 2 execution, excluding ordinary Restore commits, parent/child calls, and cleanup discovery:

| Pending work | Snapshot calls |
| :--- | :--- |
| None after Pass 1 | 0 |
| Queue drains in Pass 2 | 3: CREATE A, GET A, DELETE A |
| Changed nonempty remainder | 4: CREATE A, GET A, CREATE B, DELETE A; retain B |
| Unchanged nonempty remainder | 2: CREATE A, GET A; retain A |

Cleanup discovery adds one uncached Restore GET and one ConfigMap LIST per cleanup execution in each case. Retries, conflicts, extra terminal reconciliations, and deletion of orphan snapshots add calls. CREATE encountering an existing hash name uses an additional validation GET. The snapshot reference and pending count are committed in the Restore phase/status updates.

Snapshot operations apply to the complete queue. Their call count depends on snapshot creation, loading, replacement, and cleanup, rather than the number of pending children.

<a id="catch-up-invariants-and-crash-matrix"></a>
### Crash Recovery Contract and Crash Matrix

The legacy anchor above is retained for existing research links; this design has no pause catch-up behavior. Recovery concerns ownerReference pending work only.

| Failure point | Durable state and recovery |
| :--- | :--- |
| `InProgress` crash before status commit | No automatic resume. In-memory UID mappings and queues may be lost. Inspect created resources and restore logs; external pauses remain externally managed. |
| Pass 1 status persistence fails | Restore may remain `InProgress`; no finalizer recovery contract starts until the phase/snapshot reference/count is durable. Uncommitted snapshots remain until manual cleanup or Restore deletion. |
| `WaitingForPluginOperations[*]` restart after status commit | Existing controllers continue; Pass 2 later loads already-resolved pending ownerReferences from the referenced snapshot. |
| Pass 2 crash before replacement status commit | Old reference/count and snapshot remain intact. Successful patches may replay; unresolved work remains recoverable. |
| Pass 2 results upload succeeds before a failed status commit or restart | Reload and merge the persisted diagnostics without appending identical messages again. Reconcile counters and final phase from those results even if the child succeeds on replay. |
| Pass 2 crash after commit, before cleanup | New reference/count is authoritative. Terminal reconciliation deletes obsolete snapshots. |
| Pass 2 snapshot preparation fails | Preserve old reference/count, return a reconciliation error, and retry. |
| Referenced snapshot cannot be loaded or count disagrees | Preserve the reference/count and fail closed; report unresolved work rather than completing from an incomplete list. |
| Restore deleted or cancelled | Owner-owned snapshots are garbage-collected. External pause annotations are not removed by this engine. |

## Validation and CLI

Validate the feature flag and ConfigMap reference before restore. `OwnerRefConfigMap` must refer to a ConfigMap in the Velero namespace with an empty API group; incompatible kind/group values fail validation. Strictly parse both scope sections and reject malformed lists, unknown entry fields, empty entries, binary configuration, and unsupported sections. Reject explicit denied workload GroupKinds and `core/Namespace` in either source's `inScope`, PVC inclusion in a server baseline, and any baseline `outOfScope` section, even empty. Already-denied child kinds, including Namespace, are permitted in `outOfScope` as harmless exclusions. Validate all entries from a source before applying any of them; exclusions cannot conceal invalid inclusions. Exclusions and the built-in child restrictions always take precedence over broader group matching. Namespace's child restriction does not exclude it as a PVC parent.

Acceptance coverage must include exact-kind and group exclusions against both inclusion sources, exclusion-only configuration, empty lists, duplicates, API-version-independent matching, PVC opt-in overridden by exclusion, invalid-source atomicity, and baseline rejection. A restore regression must verify that an excluded resource is restored without being queued while its registered UID still resolves ownership for an included child.

Namespace coverage must verify inclusion rejection, zero new Namespace patch requests across both ensure/create paths, unchanged existing ownership, and eligible parent UID resolution for other children including PVCs. Synthesized Namespaces must not introduce backup UID mappings. Previously persisted Namespace requests retain the existing retry contract.

`velero restore create --owner-ref-restore-configmap` continues to select the per-restore scope. The existing `--resource-modifier-configmap` independently supplies external manifest transformations. The server uses `--features=OwnerRefRelink` and optional `--owner-ref-configmap`.

`velero restore describe` reports the ownerRef configuration, relinked object count, pending child request count, and referenced snapshot name. It reads the summary from status without fetching or listing payloads. It contains no spec relink count, paused-object list, or automatic unpause commands.

```text
OwnerReference Relinking:
  ConfigMap:           velero-ownerref-config
  Relinked OwnerRefs:  48 items
  Pending Patches:     0
  Pending Snapshot:    none
```

Pending patches and restore results diagnose relinking failures. The external procedure supplies provider-specific pause inventories and resume commands.

## Compatibility and Relationship to Prior Research

The feature remains disabled by default and adds optional ownerReference API fields without changing backup archives. This design is compatible with backups created by older Velero versions, subject to the existing resource/version compatibility requirements. Existing `inScope`-only configurations retain their behavior. Per-restore `outOfScope` add no Restore API fields, pending-patch format changes, or API calls.

**Development upgrade restriction:** This revision replaces the unreleased hybrid status schema. Drain or recreate restores with old pending state before installing the new CRD/server. Removing `status.pendingOwnerRefPatches` can prune queued work; old hybrid snapshots contain only a suffix and must not be treated as complete queues. No automatic migration is provided. The new loader requires an exact reference/count pair and immutable labeled storage.

The earlier [proposal](https://github.com/velero-io/velero/pull/10556) explored a broader design that combined three distinct capabilities: `metadata.ownerReferences` relinking, automated controller pause injection and unpause orchestration, and spec-level reference rewriting. This design isolates and focuses exclusively on the core restore-side ownerReference relinking engine:

- **Controller Pause and Unpause:** Automated controller pause injection, root propagation, unpause matching, and pause-tracking metadata are deferred from this engine. Operational pause injection and resumption remain external responsibilities (for example, using existing ResourceModifiers and operational procedures). Consequently, this design accepts only scope configuration (`inScope` and per-restore `outOfScope`) in its ConfigMap rather than pause-related sections.
- **Spec Reference Rewriting:** Rewriting UID-bearing references in resource `.spec` (or `.status`) is deferred. That capability addresses different operational needs (such as operators storing raw target UIDs in custom fields) and warrants its own dedicated schema and safety validation. This design contains no dormant spec-reference processing.

## Alternatives Considered

- **Hybrid status and ConfigMap storage:** Replaced by ConfigMap-only payload storage. Avoiding a few calls for small queues did not justify partition limits, migration logic, and pending payload growth in the Restore object.

- **Backup-Side Dependency Graph:** Constructing a DAG during backup and embedding it in the archive was rejected. It would alter the backup archive contract, create tight coupling between backup and restore versions, and is unnecessary because parent-child relationships can be resolved directly on the restore side using captured UID mappings.
- **Global Topological Sorting:** Sorting restored manifests so every parent is created strictly before its children was rejected. Imposing strict topological order conflicts with Velero's established workload restore priorities (for example, restoring Pods before Deployments).
- **Single-Pass Patching (Pass 1 Only):** Attempting all relinking solely at the end of the synchronous restore item loop was rejected. Pass 1 alone cannot survive process restarts or accommodate asynchronous volume data movement without losing retry state. Pass 2 in the finalizer ensures durable retry for transient failures and clean storage lifecycle management.
- **Multi-ConfigMap Snapshot Sharding:** Sharding pending requests across multiple ConfigMaps is a potential enhancement rather than an initial requirement. If real-world restore feedback demonstrates that pending request counts or compressed snapshot sizes regularly exceed the single snapshot limits, sharding can be introduced. Until then, a single authoritative pending snapshot keeps crash-recovery state simple and consistent.

## Open Issues and Limits

- **Live controller interference:** Without external suspension, controller claims and incomplete child creation remain possible. Live precedence prevents an invalid controlling-owner array; it does not guarantee correct operator adoption.
- **UID-bearing spec/status references:** Values are unchanged and may be stale. Their presence can prevent safe workload restoration even when ownerReference relinking succeeds.
