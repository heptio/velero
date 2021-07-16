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
package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormattedGitSHA(t *testing.T) {
	tests := []struct {
		name     string
		sha      string
		state    string
		expected string
	}{
		{
			"Clean git state has no suffix",
			"abc123",
			"clean",
			"abc123",
		},
		{
			"Dirty git status includes suffix",
			"abc123",
			"dirty",
			"abc123-dirty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			GitSHA = test.sha
			GitTreeState = test.state
			assert.Equal(t, FormattedGitSHA(), test.expected)
		})
	}

}
