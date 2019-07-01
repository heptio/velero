/*
Copyright 2018 the Velero contributors.

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
package restic

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/heptio/velero/pkg/test"
)

func Test_validatePodVolumesHostPath(t *testing.T) {
	tests := []struct {
		name    string
		pods    []*corev1.Pod
		dirs    []string
		wantErr bool
	}{
		{
			name: "no error when pod volumes are present",
			pods: []*corev1.Pod{
				test.NewPod("foo", "bar", test.WithUID("foo")),
				test.NewPod("zoo", "raz", test.WithUID("zoo")),
			},
			dirs:    []string{"foo", "zoo"},
			wantErr: false,
		},
		{
			name: "no error when pod volumes are present and there are mirror pods",
			pods: []*corev1.Pod{
				test.NewPod("foo", "bar", test.WithUID("foo")),
				test.NewPod("zoo", "raz", test.WithUID("zoo"), test.WithAnnotations(v1.MirrorPodAnnotationKey, "baz")),
			},
			dirs:    []string{"foo", "baz"},
			wantErr: false,
		},
		{
			name: "error when pod volumes missing",
			pods: []*corev1.Pod{
				test.NewPod("foo", "bar", test.WithUID("foo")),
				test.NewPod("zoo", "raz", test.WithUID("zoo")),
			},
			dirs:    []string{"unexpected-dir"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", "host_pods")
			if err != nil {
				t.Error(err)
			}
			defer os.RemoveAll(tmpDir)

			for _, dir := range tt.dirs {
				err := os.Mkdir(filepath.Join(tmpDir, dir), os.ModePerm)
				if err != nil {
					t.Error(err)
				}
			}

			kubeClient := fake.NewSimpleClientset()
			for _, pod := range tt.pods {
				_, err := kubeClient.CoreV1().Pods(pod.GetNamespace()).Create(pod)
				if err != nil {
					t.Error(err)
				}
			}

			err = validatePodVolumesHostPath(kubeClient, tmpDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
