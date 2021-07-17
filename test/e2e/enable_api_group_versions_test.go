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
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/velero/pkg/builder"
	veleroexec "github.com/vmware-tanzu/velero/pkg/util/exec"
)

var _ = Describe("[APIGroup] Velero tests with various CRD API group versions", func() {
	var (
		resource, group string
		err             error
		ctx             = context.Background()
	)

	client, err := newTestClient()
	Expect(err).To(Succeed(), "Failed to instantiate cluster client for group version tests")

	BeforeEach(func() {
		resource = "rockbands"
		group = "music.example.io"

		uuidgen, err = uuid.NewRandom()
		Expect(err).NotTo(HaveOccurred())

		// TODO: install Velero once for the test suite once feature flag is
		// removed and velero installation becomes the same as other e2e tests.
		if installVelero {
			err = veleroInstall(
				context.Background(),
				veleroImage,
				veleroNamespace,
				cloudProvider,
				objectStoreProvider,
				false,
				cloudCredentialsFile,
				bslBucket,
				bslPrefix,
				bslConfig,
				vslConfig,
				"EnableAPIGroupVersions", // TODO: remove when feature flag is removed
			)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		fmt.Printf("Clean up resource: kubectl delete crd %s.%s\n", resource, group)
		cmd := exec.CommandContext(ctx, "kubectl", "delete", "crd", resource+"."+group)
		_, stderr, err := veleroexec.RunCommand(cmd)
		if strings.Contains(stderr, "NotFound") {
			fmt.Printf("Ignore error: %v\n", stderr)
			err = nil
		}
		Expect(err).NotTo(HaveOccurred())

		if installVelero {
			err = veleroUninstall(ctx, client.kubebuilder, installVelero, veleroNamespace)
			Expect(err).NotTo(HaveOccurred())
		}

	})

	Context("When EnableAPIGroupVersions flag is set", func() {
		It("Should back up API group version and restore by version priority", func() {
			Expect(runEnableAPIGroupVersionsTests(
				ctx,
				client,
				resource,
				group,
			)).To(Succeed(), "Failed to successfully backup and restore multiple API Groups")
		})
	})
})

func runEnableAPIGroupVersionsTests(ctx context.Context, client testClient, resource, group string) error {
	tests := []struct {
		name       string
		namespaces []string
		srcCrdYaml string
		srcCRs     map[string]string
		tgtCrdYaml string
		tgtVer     string
		cm         *corev1api.ConfigMap
		gvs        map[string][]string
		want       map[string]map[string]string
	}{
		{
			name:       "Target and source cluster preferred versions match; Preferred version v1 is restored (Priority 1, Case A).",
			srcCrdYaml: "testdata/enable_api_group_versions/case-a-source.yaml",
			srcCRs: map[string]string{
				"v1":       "testdata/enable_api_group_versions/music_v1_rockband.yaml",
				"v1alpha1": "testdata/enable_api_group_versions/music_v1alpha1_rockband.yaml",
			},
			tgtCrdYaml: "testdata/enable_api_group_versions/case-a-target.yaml",
			tgtVer:     "v1",
			cm:         nil,
			want: map[string]map[string]string{
				"annotations": {
					"rockbands.music.example.io/originalVersion": "v1",
				},
				"specs": {
					"genre": "60s rock",
				},
			},
		},
		{
			name:       "Latest common non-preferred supported version v2beta2 is restored (Priority 3, Case D).",
			srcCrdYaml: "testdata/enable_api_group_versions/case-b-source-manually-added-mutations.yaml",
			srcCRs: map[string]string{
				"v2beta2": "testdata/enable_api_group_versions/music_v2beta2_rockband.yaml",
				"v2beta1": "testdata/enable_api_group_versions/music_v2beta1_rockband.yaml",
				"v1":      "testdata/enable_api_group_versions/music_v1_rockband.yaml",
			},
			tgtCrdYaml: "testdata/enable_api_group_versions/case-d-target-manually-added-mutations.yaml",
			tgtVer:     "v2beta2",
			cm:         nil,
			want: map[string]map[string]string{
				"annotations": {
					"rockbands.music.example.io/originalVersion": "v2beta2",
				},
				"specs": {
					"genre": "60s rock",
				},
			},
		},
		{
			name:       "No common supported versions means no rockbands custom resource is restored.",
			srcCrdYaml: "testdata/enable_api_group_versions/case-a-source.yaml",
			srcCRs: map[string]string{
				"v1":       "testdata/enable_api_group_versions/music_v1_rockband.yaml",
				"v1alpha1": "testdata/enable_api_group_versions/music_v1alpha1_rockband.yaml",
			},
			tgtCrdYaml: "testdata/enable_api_group_versions/case-b-target-manually-added-mutations.yaml",
			tgtVer:     "",
			cm:         nil,
			want:       nil,
		},
		{
			name:       "User config map overrides Priority 3, Case D and restores v2beta1",
			srcCrdYaml: "testdata/enable_api_group_versions/case-b-source-manually-added-mutations.yaml",
			srcCRs: map[string]string{
				"v2beta2": "testdata/enable_api_group_versions/music_v2beta2_rockband.yaml",
				"v2beta1": "testdata/enable_api_group_versions/music_v2beta1_rockband.yaml",
				"v1":      "testdata/enable_api_group_versions/music_v1_rockband.yaml",
			},
			tgtCrdYaml: "testdata/enable_api_group_versions/case-d-target-manually-added-mutations.yaml",
			tgtVer:     "v2beta1",
			cm: builder.ForConfigMap(veleroNamespace, "enableapigroupversions").Data(
				"restoreResourcesVersionPriority",
				`rockbands.music.example.io=v2beta1,v2beta2,v2`,
			).Result(),
			want: map[string]map[string]string{
				"annotations": {
					"rockbands.music.example.io/originalVersion": "v2beta1",
				},
				"specs": {
					"genre": "60s rock",
				},
			},
		},
		{
			name:       "Restore successful when CRD doesn't (yet) exist in target",
			srcCrdYaml: "testdata/enable_api_group_versions/case-a-source.yaml",
			srcCRs: map[string]string{
				"v1": "testdata/enable_api_group_versions/music_v1_rockband.yaml",
			},
			tgtCrdYaml: "",
			tgtVer:     "v1",
			cm:         nil,
			want: map[string]map[string]string{
				"annotations": {
					"rockbands.music.example.io/originalVersion": "v1",
				},
				"specs": {
					"genre": "60s rock",
				},
			},
		},
	}

	for i, tc := range tests {
		fmt.Printf("\n====== Test Case %d: %s ======\n", i, tc.name)

		err := installCRD(ctx, tc.srcCrdYaml)
		if err != nil {
			return errors.Wrap(err, "install music-system CRD on source cluster")
		}

		for version, cr := range tc.srcCRs {
			ns := resource + "-src-" + version

			if err := createNamespace(ctx, client, ns); err != nil {
				return errors.Wrapf(err, "create %s namespace", ns)
			}

			if err := installCR(ctx, cr, ns); err != nil {
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.Wrapf(err, "install %s custom resource on source cluster in namespace %s", cr, ns)
			}

			tc.namespaces = append(tc.namespaces, ns)
		}

		// Restart Velero pods in order to recognize music-system CRD right away
		// instead of waiting for discovery helper to refresh. See
		// https://github.com/vmware-tanzu/velero/issues/3471.
		if err := restartPods(ctx, veleroNamespace); err != nil {
			deleteNamespacesOnErr(ctx, tc.namespaces)
			return errors.Wrapf(err, "restart Velero pods")
		}

		backup := "backup-rockbands-" + uuidgen.String() + "-" + strconv.Itoa(i)
		namespacesStr := strings.Join(tc.namespaces, ",")

		err = veleroBackupNamespace(ctx, veleroCLI, veleroNamespace, backup, namespacesStr, "", false)
		if err != nil {
			veleroBackupLogs(ctx, veleroCLI, veleroNamespace, backup)
			deleteNamespacesOnErr(ctx, tc.namespaces)
			return errors.Wrapf(err, "back up %s namespaces on source cluster", namespacesStr)
		}

		if err := deleteCRD(ctx, tc.srcCrdYaml); err != nil {
			deleteNamespacesOnErr(ctx, tc.namespaces)
			return errors.Wrapf(err, "delete music-system CRD from source cluster")
		}

		for _, ns := range tc.namespaces {
			if err := deleteNamespace(ctx, ns); err != nil {
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.Wrapf(err, "delete %s namespace from source cluster", ns)
			}
		}

		// Install music-system CRD for target cluster.
		if tc.tgtCrdYaml != "" {
			if err := installCRD(ctx, tc.tgtCrdYaml); err != nil {
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.Wrapf(err, "install music-system CRD on target cluster")
			}
		}

		// Apply config map if there is one.
		if tc.cm != nil {
			_, err := client.clientGo.CoreV1().ConfigMaps(veleroNamespace).Create(ctx, tc.cm, metav1.CreateOptions{})
			if err != nil {
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.Wrap(err, "create config map with user version priorities")
			}
		}

		// Reset Velero to recognize music-system CRD.
		if err := restartPods(ctx, veleroNamespace); err != nil {
			deleteNamespacesOnErr(ctx, tc.namespaces)
			return errors.Wrapf(err, "restart Velero pods")
		}

		// Restore rockbands namespaces.
		restore := "restore-rockbands-" + uuidgen.String() + "-" + strconv.Itoa(i)

		if tc.want != nil {
			if err := veleroRestore(ctx, veleroCLI, veleroNamespace, restore, backup); err != nil {
				veleroRestoreLogs(ctx, veleroCLI, veleroNamespace, restore)
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.Wrapf(err, "restore %s namespaces on target cluster", namespacesStr)
			}

			annoSpec, err := resourceInfo(ctx, group, tc.tgtVer, resource)
			if err != nil {
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.Wrapf(
					err,
					"get annotation and spec from %s.%s/%s object",
					resource,
					group,
					tc.tgtVer,
				)
			}

			// Assertion
			if containsAll(annoSpec["annotations"], tc.want["annotations"]) != true {
				msg := fmt.Sprintf(
					"actual annotations: %v, expected annotations: %v",
					annoSpec["annotations"],
					tc.want["annotations"],
				)
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.New(msg)
			}

			// Assertion
			if containsAll(annoSpec["specs"], tc.want["specs"]) != true {
				msg := fmt.Sprintf(
					"actual specs: %v, expected specs: %v",
					annoSpec["specs"],
					tc.want["specs"],
				)
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.New(msg)
			}

		} else {
			// No custom resource should have been restored. Expect "no resource found"
			// error during restore.
			err := veleroRestore(ctx, veleroCLI, veleroNamespace, restore, backup)

			if err.Error() != "Unexpected restore phase got PartiallyFailed, expecting Completed" {
				deleteNamespacesOnErr(ctx, tc.namespaces)
				return errors.New("expected error but not none")
			}
		}

		// Clean up.
		for _, ns := range tc.namespaces {
			fmt.Println("Delete namespace", ns)
			deleteNamespace(ctx, ns)
		}
		_ = deleteCRD(ctx, tc.srcCrdYaml)
		if tc.tgtCrdYaml != "" {
			_ = deleteCRD(ctx, tc.tgtCrdYaml)
		}
	}

	return nil
}

func installCRD(ctx context.Context, yaml string) error {
	fmt.Printf("Install CRD with %s.\n", yaml)
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", yaml)

	_, stderr, err := veleroexec.RunCommand(cmd)
	if err != nil {
		return errors.Wrap(err, stderr)
	}

	return nil
}

func deleteCRD(ctx context.Context, yaml string) error {
	fmt.Println("Delete CRD", yaml)
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "-f", yaml, "--wait")

	_, stderr, err := veleroexec.RunCommand(cmd)
	if strings.Contains(stderr, "not found") {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, stderr)
	}

	return nil
}

func restartPods(ctx context.Context, ns string) error {
	fmt.Printf("Restart pods in %s namespace.\n", ns)
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "pod", "--all", "-n", ns, "--wait=true")

	_, stderr, err := veleroexec.RunCommand(cmd)
	if strings.Contains(stderr, "not found") {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, stderr)
	}
	return nil
}

func deleteNamespace(ctx context.Context, ns string) error {
	fmt.Println("Delete namespace", ns)
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "ns", ns, "--wait")

	_, stderr, err := veleroexec.RunCommand(cmd)
	if strings.Contains(stderr, "not found") {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, stderr)
	}

	return nil
}

// DeleteNamespacesOnErr cleans up the namespaces created for a test cast after an
// error interrupts a test case.
func deleteNamespacesOnErr(ctx context.Context, namespaces []string) {
	if len(namespaces) > 0 {
		fmt.Println("An error has occurred. Cleaning up test case namespaces.")
	}

	for _, ns := range namespaces {
		deleteNamespace(ctx, ns)
	}
}

func installCR(ctx context.Context, crFile, ns string) error {
	retries := 5
	var stderr string
	var err error

	for i := 0; i < retries; i++ {
		fmt.Printf("Attempt %d: Install custom resource %s\n", i+1, crFile)
		cmd := exec.CommandContext(ctx, "kubectl", "apply", "-n", ns, "-f", crFile)
		_, stderr, err = veleroexec.RunCommand(cmd)
		if err == nil {
			fmt.Printf("Successfully installed CR on %s.\n", ns)
			return nil
		}

		fmt.Printf("Sleep for %ds before next attempt.\n", 20*i)
		time.Sleep(time.Second * time.Duration(i) * 20)
	}
	return errors.Wrap(err, stderr)
}

func resourceInfo(ctx context.Context, g, v, r string) (map[string]map[string]string, error) {
	rvg := r + "." + v + "." + g
	ns := r + "-src-" + v
	cmd := exec.CommandContext(ctx, "kubectl", "get", rvg, "-n", ns, "-o", "json")

	stdout, errMsg, err := veleroexec.RunCommand(cmd)
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}

	var info map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return nil, errors.Wrap(err, "unmarshal resource info JSON")
	}
	items := info["items"].([]interface{})

	if len(items) < 1 {
		return nil, errors.New("resource info is empty")
	}

	item := items[0].(map[string]interface{})
	metadata := item["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	specs := item["spec"].(map[string]interface{})

	annoSpec := make(map[string]map[string]string)

	for k, v := range annotations {
		if annoSpec["annotations"] == nil {
			annoSpec["annotations"] = map[string]string{
				k: v.(string),
			}
		} else {
			annoSpec["annotations"][k] = v.(string)
		}
	}

	for k, v := range specs {
		if val, ok := v.(string); ok {
			if annoSpec["specs"] == nil {
				annoSpec["specs"] = map[string]string{
					k: val,
				}
			} else {
				annoSpec["specs"][k] = val
			}
		}
	}

	return annoSpec, nil
}

// containsAll returns true if all the map values in the needles argument
// are found in the haystack argument values.
func containsAll(haystack, needles map[string]string) bool {
	for nkey, nval := range needles {

		hval, ok := haystack[nkey]
		if !ok {
			return false
		}

		if hval != nval {
			return false
		}
	}
	return true
}
