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

package integration

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"go.uber.org/multierr"
)

//go:embed agent_metrics/metadata.yaml
var agentMetricsMetadata []byte

//go:embed third_party_apps_data/applications
var thirdPartyDataDir embed.FS

func TestValidateMetadataOfThirdPartyApps(t *testing.T) {
	err := walkThirdPartyApps(func(contents []byte) error {
		return metadata.UnmarshalAndValidate(contents, &metadata.IntegrationMetadata{})
	})
	if err != nil {
		t.Error(err)
	}
}

func TestRequireMetadataForAllThirdPartyApps(t *testing.T) {
	parentDirectory := "third_party_apps_data/applications"
	dirs, err := thirdPartyDataDir.ReadDir(parentDirectory)
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range dirs {
		if _, fileErr := os.Stat(path.Join(parentDirectory, dir.Name(), "metadata.yaml")); fileErr != nil {
			err = multierr.Append(err, fileErr)
		}
	}

	if err != nil {
		t.Error(err)
	}
}

func TestThirdPartyPublicUrls(t *testing.T) {
	err := walkThirdPartyApps(func(contents []byte) error {
		integrationMetadata := &metadata.IntegrationMetadata{}
		err := metadata.UnmarshalAndValidate(contents, integrationMetadata)
		if integrationMetadata.PublicUrl == "" {
			// The public doc isn't available yet.
			return nil
		}
		if err != nil {
			return err
		}
		t.Run(integrationMetadata.ShortName, func(t *testing.T) {
			t.Parallel()
			r, err := http.Get(integrationMetadata.PublicUrl)
			if err != nil {
				t.Error(err)
			}
			if r.StatusCode != 200 {
				t.Error(fmt.Sprintf("Invalid public url: %s", integrationMetadata.PublicUrl))
			}
		})
		return nil
	})

	if err != nil {
		t.Error(err)
	}
}

func walkThirdPartyApps(fn func(contents []byte) error) error {
	return fs.WalkDir(thirdPartyDataDir, ".", func(path string, info fs.DirEntry, err error) error {
		if info.Name() != "metadata.yaml" {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return fn(contents)
	})
}

func TestValidateMetadataOfAgentMetric(t *testing.T) {

	err := metadata.UnmarshalAndValidate(agentMetricsMetadata, &metadata.ExpectedMetricsContainer{})
	if err != nil {
		t.Error(err)
	}
}
