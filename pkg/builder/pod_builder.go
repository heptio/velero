/*
Copyright The Velero Contributors.

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

package builder

import (
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodBuilder builds Pod objects.
type PodBuilder struct {
	object *corev1api.Pod
}

// ForPod is the constructor for a PodBuilder.
func ForPod(ns, name string) *PodBuilder {
	return &PodBuilder{
		object: &corev1api.Pod{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1api.SchemeGroupVersion.String(),
				Kind:       "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		},
	}
}

// Result returns the built Pod.
func (b *PodBuilder) Result() *corev1api.Pod {
	return b.object
}

// ObjectMeta applies functional options to the Pod's ObjectMeta.
func (b *PodBuilder) ObjectMeta(opts ...ObjectMetaOpt) *PodBuilder {
	for _, opt := range opts {
		opt(b.object)
	}

	return b
}

// ServiceAccount sets serviceAccounts on pod.
func (b *PodBuilder) ServiceAccount(sa string) *PodBuilder {
	b.object.Spec.ServiceAccountName = sa
	return b
}

// Volumes appends to the pod's volumes
func (b *PodBuilder) Volumes(volumes ...*corev1api.Volume) *PodBuilder {
	for _, v := range volumes {
		b.object.Spec.Volumes = append(b.object.Spec.Volumes, *v)
	}
	return b
}

// NodeName sets the pod's node name
func (b *PodBuilder) NodeName(val string) *PodBuilder {
	b.object.Spec.NodeName = val
	return b
}

func (b *PodBuilder) InitContainers(containers ...*corev1api.Container) *PodBuilder {
	for _, c := range containers {
		b.object.Spec.InitContainers = append(b.object.Spec.InitContainers, *c)
	}
	return b
}

func (b *PodBuilder) Containers(containers ...*corev1api.Container) *PodBuilder {
	for _, c := range containers {
		b.object.Spec.Containers = append(b.object.Spec.Containers, *c)
	}
	return b
}

func (b *PodBuilder) ContainerStatuses(containerStatuses ...*corev1api.ContainerStatus) *PodBuilder {
	for _, c := range containerStatuses {
		b.object.Status.ContainerStatuses = append(b.object.Status.ContainerStatuses, *c)
	}
	return b
}
