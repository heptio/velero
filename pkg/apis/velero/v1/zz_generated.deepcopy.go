// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Backup) DeepCopyInto(out *Backup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Backup.
func (in *Backup) DeepCopy() *Backup {
	if in == nil {
		return nil
	}
	out := new(Backup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Backup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupHooks) DeepCopyInto(out *BackupHooks) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]BackupResourceHookSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupHooks.
func (in *BackupHooks) DeepCopy() *BackupHooks {
	if in == nil {
		return nil
	}
	out := new(BackupHooks)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupList) DeepCopyInto(out *BackupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Backup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupList.
func (in *BackupList) DeepCopy() *BackupList {
	if in == nil {
		return nil
	}
	out := new(BackupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BackupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupProgress) DeepCopyInto(out *BackupProgress) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupProgress.
func (in *BackupProgress) DeepCopy() *BackupProgress {
	if in == nil {
		return nil
	}
	out := new(BackupProgress)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupResourceHook) DeepCopyInto(out *BackupResourceHook) {
	*out = *in
	if in.Exec != nil {
		in, out := &in.Exec, &out.Exec
		*out = new(ExecHook)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupResourceHook.
func (in *BackupResourceHook) DeepCopy() *BackupResourceHook {
	if in == nil {
		return nil
	}
	out := new(BackupResourceHook)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupResourceHookSpec) DeepCopyInto(out *BackupResourceHookSpec) {
	*out = *in
	if in.IncludedNamespaces != nil {
		in, out := &in.IncludedNamespaces, &out.IncludedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedNamespaces != nil {
		in, out := &in.ExcludedNamespaces, &out.ExcludedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IncludedResources != nil {
		in, out := &in.IncludedResources, &out.IncludedResources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedResources != nil {
		in, out := &in.ExcludedResources, &out.ExcludedResources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.PreHooks != nil {
		in, out := &in.PreHooks, &out.PreHooks
		*out = make([]BackupResourceHook, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.PostHooks != nil {
		in, out := &in.PostHooks, &out.PostHooks
		*out = make([]BackupResourceHook, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupResourceHookSpec.
func (in *BackupResourceHookSpec) DeepCopy() *BackupResourceHookSpec {
	if in == nil {
		return nil
	}
	out := new(BackupResourceHookSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupSpec) DeepCopyInto(out *BackupSpec) {
	*out = *in
	if in.IncludedNamespaces != nil {
		in, out := &in.IncludedNamespaces, &out.IncludedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedNamespaces != nil {
		in, out := &in.ExcludedNamespaces, &out.ExcludedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IncludedResources != nil {
		in, out := &in.IncludedResources, &out.IncludedResources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedResources != nil {
		in, out := &in.ExcludedResources, &out.ExcludedResources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.SnapshotVolumes != nil {
		in, out := &in.SnapshotVolumes, &out.SnapshotVolumes
		*out = new(bool)
		**out = **in
	}
	out.TTL = in.TTL
	if in.IncludeClusterResources != nil {
		in, out := &in.IncludeClusterResources, &out.IncludeClusterResources
		*out = new(bool)
		**out = **in
	}
	in.Hooks.DeepCopyInto(&out.Hooks)
	if in.VolumeSnapshotLocations != nil {
		in, out := &in.VolumeSnapshotLocations, &out.VolumeSnapshotLocations
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupSpec.
func (in *BackupSpec) DeepCopy() *BackupSpec {
	if in == nil {
		return nil
	}
	out := new(BackupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupStatus) DeepCopyInto(out *BackupStatus) {
	*out = *in
	if in.Expiration != nil {
		in, out := &in.Expiration, &out.Expiration
		*out = (*in).DeepCopy()
	}
	if in.ValidationErrors != nil {
		in, out := &in.ValidationErrors, &out.ValidationErrors
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.StartTimestamp != nil {
		in, out := &in.StartTimestamp, &out.StartTimestamp
		*out = (*in).DeepCopy()
	}
	if in.CompletionTimestamp != nil {
		in, out := &in.CompletionTimestamp, &out.CompletionTimestamp
		*out = (*in).DeepCopy()
	}
	if in.Progress != nil {
		in, out := &in.Progress, &out.Progress
		*out = new(BackupProgress)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupStatus.
func (in *BackupStatus) DeepCopy() *BackupStatus {
	if in == nil {
		return nil
	}
	out := new(BackupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeleteBackupRequest) DeepCopyInto(out *DeleteBackupRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeleteBackupRequest.
func (in *DeleteBackupRequest) DeepCopy() *DeleteBackupRequest {
	if in == nil {
		return nil
	}
	out := new(DeleteBackupRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DeleteBackupRequest) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeleteBackupRequestList) DeepCopyInto(out *DeleteBackupRequestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DeleteBackupRequest, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeleteBackupRequestList.
func (in *DeleteBackupRequestList) DeepCopy() *DeleteBackupRequestList {
	if in == nil {
		return nil
	}
	out := new(DeleteBackupRequestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DeleteBackupRequestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeleteBackupRequestSpec) DeepCopyInto(out *DeleteBackupRequestSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeleteBackupRequestSpec.
func (in *DeleteBackupRequestSpec) DeepCopy() *DeleteBackupRequestSpec {
	if in == nil {
		return nil
	}
	out := new(DeleteBackupRequestSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeleteBackupRequestStatus) DeepCopyInto(out *DeleteBackupRequestStatus) {
	*out = *in
	if in.Errors != nil {
		in, out := &in.Errors, &out.Errors
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeleteBackupRequestStatus.
func (in *DeleteBackupRequestStatus) DeepCopy() *DeleteBackupRequestStatus {
	if in == nil {
		return nil
	}
	out := new(DeleteBackupRequestStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DownloadRequest) DeepCopyInto(out *DownloadRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DownloadRequest.
func (in *DownloadRequest) DeepCopy() *DownloadRequest {
	if in == nil {
		return nil
	}
	out := new(DownloadRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DownloadRequest) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DownloadRequestList) DeepCopyInto(out *DownloadRequestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DownloadRequest, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DownloadRequestList.
func (in *DownloadRequestList) DeepCopy() *DownloadRequestList {
	if in == nil {
		return nil
	}
	out := new(DownloadRequestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DownloadRequestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DownloadRequestSpec) DeepCopyInto(out *DownloadRequestSpec) {
	*out = *in
	out.Target = in.Target
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DownloadRequestSpec.
func (in *DownloadRequestSpec) DeepCopy() *DownloadRequestSpec {
	if in == nil {
		return nil
	}
	out := new(DownloadRequestSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DownloadRequestStatus) DeepCopyInto(out *DownloadRequestStatus) {
	*out = *in
	if in.Expiration != nil {
		in, out := &in.Expiration, &out.Expiration
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DownloadRequestStatus.
func (in *DownloadRequestStatus) DeepCopy() *DownloadRequestStatus {
	if in == nil {
		return nil
	}
	out := new(DownloadRequestStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DownloadTarget) DeepCopyInto(out *DownloadTarget) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DownloadTarget.
func (in *DownloadTarget) DeepCopy() *DownloadTarget {
	if in == nil {
		return nil
	}
	out := new(DownloadTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExecHook) DeepCopyInto(out *ExecHook) {
	*out = *in
	if in.Command != nil {
		in, out := &in.Command, &out.Command
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.Timeout = in.Timeout
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExecHook.
func (in *ExecHook) DeepCopy() *ExecHook {
	if in == nil {
		return nil
	}
	out := new(ExecHook)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PluginInfo) DeepCopyInto(out *PluginInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PluginInfo.
func (in *PluginInfo) DeepCopy() *PluginInfo {
	if in == nil {
		return nil
	}
	out := new(PluginInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeBackup) DeepCopyInto(out *PodVolumeBackup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeBackup.
func (in *PodVolumeBackup) DeepCopy() *PodVolumeBackup {
	if in == nil {
		return nil
	}
	out := new(PodVolumeBackup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodVolumeBackup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeBackupList) DeepCopyInto(out *PodVolumeBackupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PodVolumeBackup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeBackupList.
func (in *PodVolumeBackupList) DeepCopy() *PodVolumeBackupList {
	if in == nil {
		return nil
	}
	out := new(PodVolumeBackupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodVolumeBackupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeBackupSpec) DeepCopyInto(out *PodVolumeBackupSpec) {
	*out = *in
	out.Pod = in.Pod
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeBackupSpec.
func (in *PodVolumeBackupSpec) DeepCopy() *PodVolumeBackupSpec {
	if in == nil {
		return nil
	}
	out := new(PodVolumeBackupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeBackupStatus) DeepCopyInto(out *PodVolumeBackupStatus) {
	*out = *in
	if in.StartTimestamp != nil {
		in, out := &in.StartTimestamp, &out.StartTimestamp
		*out = (*in).DeepCopy()
	}
	if in.CompletionTimestamp != nil {
		in, out := &in.CompletionTimestamp, &out.CompletionTimestamp
		*out = (*in).DeepCopy()
	}
	out.Progress = in.Progress
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeBackupStatus.
func (in *PodVolumeBackupStatus) DeepCopy() *PodVolumeBackupStatus {
	if in == nil {
		return nil
	}
	out := new(PodVolumeBackupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeOperationProgress) DeepCopyInto(out *PodVolumeOperationProgress) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeOperationProgress.
func (in *PodVolumeOperationProgress) DeepCopy() *PodVolumeOperationProgress {
	if in == nil {
		return nil
	}
	out := new(PodVolumeOperationProgress)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeRestore) DeepCopyInto(out *PodVolumeRestore) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeRestore.
func (in *PodVolumeRestore) DeepCopy() *PodVolumeRestore {
	if in == nil {
		return nil
	}
	out := new(PodVolumeRestore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodVolumeRestore) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeRestoreList) DeepCopyInto(out *PodVolumeRestoreList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PodVolumeRestore, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeRestoreList.
func (in *PodVolumeRestoreList) DeepCopy() *PodVolumeRestoreList {
	if in == nil {
		return nil
	}
	out := new(PodVolumeRestoreList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodVolumeRestoreList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeRestoreSpec) DeepCopyInto(out *PodVolumeRestoreSpec) {
	*out = *in
	out.Pod = in.Pod
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeRestoreSpec.
func (in *PodVolumeRestoreSpec) DeepCopy() *PodVolumeRestoreSpec {
	if in == nil {
		return nil
	}
	out := new(PodVolumeRestoreSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodVolumeRestoreStatus) DeepCopyInto(out *PodVolumeRestoreStatus) {
	*out = *in
	if in.StartTimestamp != nil {
		in, out := &in.StartTimestamp, &out.StartTimestamp
		*out = (*in).DeepCopy()
	}
	if in.CompletionTimestamp != nil {
		in, out := &in.CompletionTimestamp, &out.CompletionTimestamp
		*out = (*in).DeepCopy()
	}
	out.Progress = in.Progress
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodVolumeRestoreStatus.
func (in *PodVolumeRestoreStatus) DeepCopy() *PodVolumeRestoreStatus {
	if in == nil {
		return nil
	}
	out := new(PodVolumeRestoreStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResticRepository) DeepCopyInto(out *ResticRepository) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResticRepository.
func (in *ResticRepository) DeepCopy() *ResticRepository {
	if in == nil {
		return nil
	}
	out := new(ResticRepository)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ResticRepository) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResticRepositoryList) DeepCopyInto(out *ResticRepositoryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ResticRepository, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResticRepositoryList.
func (in *ResticRepositoryList) DeepCopy() *ResticRepositoryList {
	if in == nil {
		return nil
	}
	out := new(ResticRepositoryList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ResticRepositoryList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResticRepositorySpec) DeepCopyInto(out *ResticRepositorySpec) {
	*out = *in
	out.MaintenanceFrequency = in.MaintenanceFrequency
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResticRepositorySpec.
func (in *ResticRepositorySpec) DeepCopy() *ResticRepositorySpec {
	if in == nil {
		return nil
	}
	out := new(ResticRepositorySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResticRepositoryStatus) DeepCopyInto(out *ResticRepositoryStatus) {
	*out = *in
	if in.LastMaintenanceTime != nil {
		in, out := &in.LastMaintenanceTime, &out.LastMaintenanceTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResticRepositoryStatus.
func (in *ResticRepositoryStatus) DeepCopy() *ResticRepositoryStatus {
	if in == nil {
		return nil
	}
	out := new(ResticRepositoryStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Restore) DeepCopyInto(out *Restore) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Restore.
func (in *Restore) DeepCopy() *Restore {
	if in == nil {
		return nil
	}
	out := new(Restore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Restore) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RestoreList) DeepCopyInto(out *RestoreList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Restore, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RestoreList.
func (in *RestoreList) DeepCopy() *RestoreList {
	if in == nil {
		return nil
	}
	out := new(RestoreList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RestoreList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RestoreSpec) DeepCopyInto(out *RestoreSpec) {
	*out = *in
	if in.IncludedNamespaces != nil {
		in, out := &in.IncludedNamespaces, &out.IncludedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedNamespaces != nil {
		in, out := &in.ExcludedNamespaces, &out.ExcludedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IncludedResources != nil {
		in, out := &in.IncludedResources, &out.IncludedResources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedResources != nil {
		in, out := &in.ExcludedResources, &out.ExcludedResources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.NamespaceMapping != nil {
		in, out := &in.NamespaceMapping, &out.NamespaceMapping
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.RestorePVs != nil {
		in, out := &in.RestorePVs, &out.RestorePVs
		*out = new(bool)
		**out = **in
	}
	if in.IncludeClusterResources != nil {
		in, out := &in.IncludeClusterResources, &out.IncludeClusterResources
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RestoreSpec.
func (in *RestoreSpec) DeepCopy() *RestoreSpec {
	if in == nil {
		return nil
	}
	out := new(RestoreSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RestoreStatus) DeepCopyInto(out *RestoreStatus) {
	*out = *in
	if in.ValidationErrors != nil {
		in, out := &in.ValidationErrors, &out.ValidationErrors
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RestoreStatus.
func (in *RestoreStatus) DeepCopy() *RestoreStatus {
	if in == nil {
		return nil
	}
	out := new(RestoreStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Schedule) DeepCopyInto(out *Schedule) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Schedule.
func (in *Schedule) DeepCopy() *Schedule {
	if in == nil {
		return nil
	}
	out := new(Schedule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Schedule) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ScheduleList) DeepCopyInto(out *ScheduleList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Schedule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ScheduleList.
func (in *ScheduleList) DeepCopy() *ScheduleList {
	if in == nil {
		return nil
	}
	out := new(ScheduleList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ScheduleList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ScheduleSpec) DeepCopyInto(out *ScheduleSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ScheduleSpec.
func (in *ScheduleSpec) DeepCopy() *ScheduleSpec {
	if in == nil {
		return nil
	}
	out := new(ScheduleSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ScheduleStatus) DeepCopyInto(out *ScheduleStatus) {
	*out = *in
	if in.LastBackup != nil {
		in, out := &in.LastBackup, &out.LastBackup
		*out = (*in).DeepCopy()
	}
	if in.ValidationErrors != nil {
		in, out := &in.ValidationErrors, &out.ValidationErrors
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ScheduleStatus.
func (in *ScheduleStatus) DeepCopy() *ScheduleStatus {
	if in == nil {
		return nil
	}
	out := new(ScheduleStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerStatusRequest) DeepCopyInto(out *ServerStatusRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerStatusRequest.
func (in *ServerStatusRequest) DeepCopy() *ServerStatusRequest {
	if in == nil {
		return nil
	}
	out := new(ServerStatusRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ServerStatusRequest) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerStatusRequestList) DeepCopyInto(out *ServerStatusRequestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ServerStatusRequest, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerStatusRequestList.
func (in *ServerStatusRequestList) DeepCopy() *ServerStatusRequestList {
	if in == nil {
		return nil
	}
	out := new(ServerStatusRequestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ServerStatusRequestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerStatusRequestSpec) DeepCopyInto(out *ServerStatusRequestSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerStatusRequestSpec.
func (in *ServerStatusRequestSpec) DeepCopy() *ServerStatusRequestSpec {
	if in == nil {
		return nil
	}
	out := new(ServerStatusRequestSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerStatusRequestStatus) DeepCopyInto(out *ServerStatusRequestStatus) {
	*out = *in
	if in.ProcessedTimestamp != nil {
		in, out := &in.ProcessedTimestamp, &out.ProcessedTimestamp
		*out = (*in).DeepCopy()
	}
	if in.Plugins != nil {
		in, out := &in.Plugins, &out.Plugins
		*out = make([]PluginInfo, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerStatusRequestStatus.
func (in *ServerStatusRequestStatus) DeepCopy() *ServerStatusRequestStatus {
	if in == nil {
		return nil
	}
	out := new(ServerStatusRequestStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeSnapshotLocation) DeepCopyInto(out *VolumeSnapshotLocation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeSnapshotLocation.
func (in *VolumeSnapshotLocation) DeepCopy() *VolumeSnapshotLocation {
	if in == nil {
		return nil
	}
	out := new(VolumeSnapshotLocation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VolumeSnapshotLocation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeSnapshotLocationList) DeepCopyInto(out *VolumeSnapshotLocationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VolumeSnapshotLocation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeSnapshotLocationList.
func (in *VolumeSnapshotLocationList) DeepCopy() *VolumeSnapshotLocationList {
	if in == nil {
		return nil
	}
	out := new(VolumeSnapshotLocationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VolumeSnapshotLocationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeSnapshotLocationSpec) DeepCopyInto(out *VolumeSnapshotLocationSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeSnapshotLocationSpec.
func (in *VolumeSnapshotLocationSpec) DeepCopy() *VolumeSnapshotLocationSpec {
	if in == nil {
		return nil
	}
	out := new(VolumeSnapshotLocationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeSnapshotLocationStatus) DeepCopyInto(out *VolumeSnapshotLocationStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeSnapshotLocationStatus.
func (in *VolumeSnapshotLocationStatus) DeepCopy() *VolumeSnapshotLocationStatus {
	if in == nil {
		return nil
	}
	out := new(VolumeSnapshotLocationStatus)
	in.DeepCopyInto(out)
	return out
}
