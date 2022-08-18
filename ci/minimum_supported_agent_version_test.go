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

			var emptyMetadata metadata.MinimumSupportedAgentVersion
			if m.MinimumSupportedAgentVersion != emptyMetadata {
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

			if modifiedMetadata.MinimumSupportedAgentVersion != originalMetadata.MinimumSupportedAgentVersion {
				t.Fatal(fmt.Errorf("minimum_supported_agent_version has been modified for path: %s", filePath))
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
