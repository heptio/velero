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

package restic

import (
	"github.com/sirupsen/logrus"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// BackupExec is able to run backups.
type BackupExec struct{}

// RunBackup is a wrapper for the restic.RunBackup function in order to be able
// to use interfaces (and swap out objects for testing purposes).
func (exec BackupExec) RunBackup(cmd *Command, log logrus.FieldLogger, updateFn func(velerov1api.PodVolumeOperationProgress)) (string, string, error) {
	return RunBackup(cmd, log, updateFn)
}

// GetSnapshotID gets the Restic snapshot ID.
func (exec BackupExec) GetSnapshotID(snapshotIdCmd *Command) (string, error) {
	return GetSnapshotID(snapshotIdCmd)
}
