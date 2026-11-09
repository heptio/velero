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

package velero

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/velero-io/velero/test"
)

func Test_getVersionWithoutPatch(t *testing.T) {
	versionTests := []struct {
		caseName string
		version  string
		result   string
	}{
		{
			caseName: "main version",
			version:  "main",
			result:   "main",
		},
		{
			caseName: "release version",
			version:  "release-1.18-dev",
			result:   "v1.18",
		},
		{
			caseName: "tag version",
			version:  "v1.17.2",
			result:   "v1.17",
		},
	}

	for _, test := range versionTests {
		t.Run(test.caseName, func(t *testing.T) {
			res := getVersionWithoutPatch(test.version)
			require.Equal(t, test.result, res)
		})
	}
}

func TestVSpherePluginSelection(t *testing.T) {
	for _, version := range []string{"v1.13.2", "v1.17.2", "v1.18.4", "main"} {
		t.Run(version, func(t *testing.T) {
			images := ImagesMatrix[getVersionWithoutPatch(version)]
			plugins, err := getPluginsByVersion(version, test.Vsphere)
			require.NoError(t, err)
			require.Equal(t, images[test.AWS], plugins)

			for _, hasVSpherePlugin := range []bool{false, true} {
				for _, moveData := range []bool{false, true} {
					config, err := SetImagesToDefaultValues(test.VeleroConfig{
						CloudProvider:    test.Vsphere,
						HasVspherePlugin: hasVSpherePlugin,
						SnapshotMoveData: moveData,
					}, version)
					require.NoError(t, err)
					require.Equal(t, images[test.AWS][0], config.Plugins)
				}
			}
		})
	}
}

func TestV118PluginSelection(t *testing.T) {
	for provider, image := range map[string]string{
		test.AWS:   "velero/velero-plugin-for-aws:v1.14.4",
		test.Azure: "velero/velero-plugin-for-microsoft-azure:v1.14.4",
		test.GCP:   "velero/velero-plugin-for-gcp:v1.14.4",
	} {
		t.Run(provider, func(t *testing.T) {
			plugins, err := getPluginsByVersion("v1.18.4", provider)
			require.NoError(t, err)
			require.Equal(t, []string{image}, plugins)
		})
	}
}
