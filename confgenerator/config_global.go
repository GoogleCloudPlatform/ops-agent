// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package confgenerator

type Global struct {
	DefaultLogFileRotation *LogFileRotation `yaml:"default_self_log_file_rotation,omitempty"`
}

type LogFileRotation struct {
	Enabled     *bool `yaml:"enabled"`
	MaxFileSize *int  `yaml:"max_file_size_megabytes" validate:"omitempty,gte=1"`
	BackupCount *int  `yaml:"backup_count" validate:"omitempty,gte=0"`
}

// Get whether log rotation should be enabled. Defaults to true if unset.
func (c *LogFileRotation) GetEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// Get the maximum file size for logs. If not set or non-positive,
// defaults to 400 MB.
func (c *LogFileRotation) GetMaxFileSize() int {
	if c.MaxFileSize == nil || *c.MaxFileSize <= 0 {
		return 400
	}
	return *c.MaxFileSize
}

// Get the maximum number of backups for logs. If not set or negative,
// defaults to 1 backup (2 files including the file that is being logged
// to).
func (c *LogFileRotation) GetBackupCount() int {
	if c.BackupCount == nil || *c.BackupCount < 0 {
		return 1
	}
	return *c.BackupCount
}
