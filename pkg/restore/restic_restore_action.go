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

package restore

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	veleroimage "github.com/vmware-tanzu/velero/internal/velero"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	velerov1client "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/typed/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/plugin/framework"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

const (
	defaultCPURequestLimit = "100m"
	defaultMemRequestLimit = "128Mi"
	defaultCommand         = "/velero-restic-restore-helper"
)

type ResticRestoreAction struct {
	logger                logrus.FieldLogger
	client                corev1client.ConfigMapInterface
	podVolumeBackupClient velerov1client.PodVolumeBackupInterface
}

func NewResticRestoreAction(logger logrus.FieldLogger, client corev1client.ConfigMapInterface, podVolumeBackupClient velerov1client.PodVolumeBackupInterface) *ResticRestoreAction {
	return &ResticRestoreAction{
		logger:                logger,
		client:                client,
		podVolumeBackupClient: podVolumeBackupClient,
	}
}

func (a *ResticRestoreAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"pods"},
	}, nil
}

func (a *ResticRestoreAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	a.logger.Info("Executing ResticRestoreAction")
	defer a.logger.Info("Done executing ResticRestoreAction")

	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pod); err != nil {
		return nil, errors.Wrap(err, "unable to convert pod from runtime.Unstructured")
	}

	// At the point when this function is called, the namespace mapping for the restore
	// has not yet been applied to `input.Item` so we can't perform a reverse-lookup in
	// the namespace mapping in the restore spec. Instead, use the pod from the backup
	// so that if the mapping is applied earlier, we still use the correct namespace.
	var podFromBackup corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.ItemFromBackup.UnstructuredContent(), &podFromBackup); err != nil {
		return nil, errors.Wrap(err, "unable to convert source pod from runtime.Unstructured")
	}

	log := a.logger.WithField("pod", kube.NamespaceAndName(&pod))

	opts := label.NewListOptionsForBackup(input.Restore.Spec.BackupName)
	podVolumeBackupList, err := a.podVolumeBackupClient.List(context.TODO(), opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var podVolumeBackups []*velerov1api.PodVolumeBackup
	for i := range podVolumeBackupList.Items {
		podVolumeBackups = append(podVolumeBackups, &podVolumeBackupList.Items[i])
	}
	volumeSnapshots := restic.GetVolumeBackupsForPod(podVolumeBackups, &pod, podFromBackup.Namespace)
	if len(volumeSnapshots) == 0 {
		log.Debug("No restic backups found for pod")
		return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
	}

	log.Info("Restic backups for pod found")

	// TODO we might want/need to get plugin config at the top of this method at some point; for now, wait
	// until we know we're doing a restore before getting config.
	log.Debugf("Getting plugin config")
	config, err := getPluginConfig(framework.PluginKindRestoreItemAction, "velero.io/restic", a.client)
	if err != nil {
		return nil, err
	}

	image := getImage(log, config)
	log.Infof("Using image %q", image)

	cpuRequest, memRequest := getResourceRequests(log, config)
	cpuLimit, memLimit := getResourceLimits(log, config)
	if cpuRequest == "" {
		cpuRequest = defaultCPURequestLimit
	}
	if cpuLimit == "" {
		cpuLimit = defaultCPURequestLimit
	}
	if memRequest == "" {
		memRequest = defaultMemRequestLimit
	}
	if memLimit == "" {
		memLimit = defaultMemRequestLimit
	}

	resourceReqs, err := kube.ParseResourceRequirements(cpuRequest, memRequest, cpuLimit, memLimit)
	if err != nil {
		log.Errorf("Using default resource values, couldn't parse resource requirements: %s.", err)
		resourceReqs, _ = kube.ParseResourceRequirements(
			defaultCPURequestLimit, defaultMemRequestLimit, // requests
			defaultCPURequestLimit, defaultMemRequestLimit, // limits
		)
	}

	runAsRoot, runAsGroup, allowPrivilegeEscalation := getSecurityContext(log, config)

	securityContext, err := kube.ParseSecurityContext(runAsRoot, runAsGroup, allowPrivilegeEscalation)
	if err != nil {
		log.Errorf("Using default resource values, couldn't parse resource requirements: %s.", err)
	}

	initContainerBuilder := newResticInitContainerBuilder(image, string(input.Restore.UID))
	initContainerBuilder.Resources(&resourceReqs)
	initContainerBuilder.SecurityContext(&securityContext)

	for volumeName := range volumeSnapshots {
		mount := &corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/restores/" + volumeName,
		}
		initContainerBuilder.VolumeMounts(mount)
	}
	initContainerBuilder.Command(getCommand(log, config))

	initContainer := *initContainerBuilder.Result()
	if len(pod.Spec.InitContainers) == 0 || pod.Spec.InitContainers[0].Name != restic.InitContainer {
		pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
	} else {
		pod.Spec.InitContainers[0] = initContainer
	}

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
	if err != nil {
		return nil, errors.Wrap(err, "unable to convert pod to runtime.Unstructured")
	}

	return velero.NewRestoreItemActionExecuteOutput(&unstructured.Unstructured{Object: res}), nil
}

func getCommand(log logrus.FieldLogger, config *corev1.ConfigMap) []string {
	if config == nil {
		log.Debug("No config found for plugin")
		return []string{defaultCommand}
	}

	if config.Data["command"] == "" {
		log.Debugf("No custom command configured")
		return []string{defaultCommand}
	}

	log.Debugf("Using custom command %s", config.Data["command"])
	return []string{config.Data["command"]}
}

func getImage(log logrus.FieldLogger, config *corev1.ConfigMap) string {
	if config == nil {
		log.Debug("No config found for plugin")
		return veleroimage.DefaultResticRestoreHelperImage()
	}

	image := config.Data["image"]
	if image == "" {
		log.Debugf("No custom image configured")
		return veleroimage.DefaultResticRestoreHelperImage()
	}

	log = log.WithField("image", image)

	parts := strings.Split(image, "/")

	if len(parts) == 1 {
		defaultImage := veleroimage.DefaultResticRestoreHelperImage()
		// Image supplied without registry part
		log.Infof("Plugin config contains image name without registry name. Using default init container image: %q", defaultImage)
		return defaultImage
	}

	if !(strings.Contains(parts[len(parts)-1], ":")) {
		tag := veleroimage.ImageTag()
		// tag-less image name: add default image tag for this version of Velero
		log.Infof("Plugin config contains image name without tag. Adding tag: %q", tag)
		return fmt.Sprintf("%s:%s", image, tag)
	} else {
		// tagged image name
		log.Debugf("Plugin config contains image name with tag")
		return image
	}
}

// getResourceRequests extracts the CPU and memory requests from a ConfigMap.
// The 0 values are valid if the keys are not present
func getResourceRequests(log logrus.FieldLogger, config *corev1.ConfigMap) (string, string) {
	if config == nil {
		log.Debug("No config found for plugin")
		return "", ""
	}

	return config.Data["cpuRequest"], config.Data["memRequest"]
}

// getResourceLimits extracts the CPU and memory limits from a ConfigMap.
// The 0 values are valid if the keys are not present
func getResourceLimits(log logrus.FieldLogger, config *corev1.ConfigMap) (string, string) {
	if config == nil {
		log.Debug("No config found for plugin")
		return "", ""
	}

	return config.Data["cpuLimit"], config.Data["memLimit"]
}

// getSecurityContext extracts securityContext runAsUser, runAsGroup, and allowPrivilegeEscalation from a ConfigMap.
func getSecurityContext(log logrus.FieldLogger, config *corev1.ConfigMap) (string, string, string) {
	if config == nil {
		log.Debug("No config found for plugin")
		return "", "", ""
	}

	return config.Data["secCtxRunAsUser"], config.Data["secCtxRunAsGroup"], config.Data["secCtxAllowPrivilegeEscalation"]
}

// TODO eventually this can move to pkg/plugin/framework since it'll be used across multiple
// plugins.
func getPluginConfig(kind framework.PluginKind, name string, client corev1client.ConfigMapInterface) (*corev1.ConfigMap, error) {
	opts := metav1.ListOptions{
		// velero.io/plugin-config: true
		// velero.io/restic: RestoreItemAction
		LabelSelector: fmt.Sprintf("velero.io/plugin-config,%s=%s", name, kind),
	}

	list, err := client.List(context.TODO(), opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(list.Items) == 0 {
		return nil, nil
	}

	if len(list.Items) > 1 {
		var items []string
		for _, item := range list.Items {
			items = append(items, item.Name)
		}
		return nil, errors.Errorf("found more than one ConfigMap matching label selector %q: %v", opts.LabelSelector, items)
	}

	return &list.Items[0], nil
}

func newResticInitContainerBuilder(image, restoreUID string) *builder.ContainerBuilder {
	return builder.ForContainer(restic.InitContainer, image).
		Args(restoreUID).
		Env([]*corev1.EnvVar{
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
		}...)
}
