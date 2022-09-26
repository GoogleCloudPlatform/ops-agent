// Copyright 2022 Google LLC
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

//go:build ci

package ci

import (
	"flag"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
)

var (
	added    = flag.String("added", "", "Added files")
	modified = flag.String("modified", "", "Modified files")
	labels   = flag.String("labels", "", "Pull request labels")
	baseDir  = flag.String("base-dir", "", "Directory path for the base of the pull request")
)
var basePath = os.Getenv("GITHUB_WORKSPACE")

func TestMinimumSupportedAgentVersionIsModified(t *testing.T) {
	if basePath == "" {
		t.Fatal("Must be run within git action")
	}

	// "Release" label is required to edit `minimum_supported_agent_version`.
	// Stackdriver-instrumentation-release is configured to automatically add the
	// label when creating a pull request, but it may also be added manually by permitted contributors
	if strings.Contains(*labels, "release") {
		return
	}

	addedMetadataFilePaths, err := filterForMetadata(*added)
	if err != nil {
		t.Fatal(err)
	}

	modifiedMetadataFilePaths, err := filterForMetadata(*modified)
	if err != nil {
		t.Fatal(err)
	}

	if len(addedMetadataFilePaths) != 0 {
		testAddedMetadata(t, addedMetadataFilePaths)
	}

	if len(modifiedMetadataFilePaths) != 0 {
		testModifiedMetadata(t, modifiedMetadataFilePaths)
	}
}

func filterForMetadata(filePaths string) ([]string, error) {
	metadataFilePaths := make([]string, 0)
	for _, filePath := range strings.Split(filePaths, " ") {
		matches, err := regexp.MatchString("integration_test/third_party_apps_data/applications/.*/metadata.yaml", filePath)
		if err != nil {
			return nil, err
		}
		if matches {
			metadataFilePaths = append(metadataFilePaths, filePath)
		}
	}
	return metadataFilePaths, nil
}

func testAddedMetadata(t *testing.T, filePaths []string) {
	for _, filePath := range filePaths {
		filePath := filePath
		t.Run(filePath, func(t *testing.T) {
			t.Parallel()
			m := metadata.IntegrationMetadata{}
			if err := unmarshalYamlFromFile(path.Join(basePath, filePath), &m); err != nil {
				t.Fatal(err)
			}

			if m.MinimumSupportedAgentVersion != nil {
				t.Fatal("Cannot add metadata.MinimumSupportedAgentVersion")
			}
		})
	}
}

func testModifiedMetadata(t *testing.T, filePaths []string) {
	for _, filePath := range filePaths {
		filePath := filePath
		t.Run(filePath, func(t *testing.T) {
			t.Parallel()
			var originalMetadata metadata.IntegrationMetadata
			var modifiedMetadata metadata.IntegrationMetadata

			if err := unmarshalYamlFromFile(path.Join(basePath, filePath), &modifiedMetadata); err != nil {
				t.Fatal(err)
			}

			if err := unmarshalYamlFromFile(path.Join(basePath, *baseDir, filePath), &originalMetadata); err != nil {
				t.Fatal(err)
			}

			if *modifiedMetadata.MinimumSupportedAgentVersion != *originalMetadata.MinimumSupportedAgentVersion {
				t.Fatal(fmt.Errorf("minimum_supported_agent_version has been modified for path: %s\n modified: %+v\n original: %+v", filePath, modifiedMetadata.MinimumSupportedAgentVersion, originalMetadata.MinimumSupportedAgentVersion))
			}
		})
	}
}

func unmarshalYamlFromFile(dir string, i interface{}) error {
	bytes, err := os.ReadFile(dir)
	if err != nil {
		return err
	}
	return metadata.UnmarshalAndValidate(bytes, i)
}
