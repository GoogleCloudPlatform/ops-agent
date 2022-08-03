//go:build ci

package ci

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"gopkg.in/yaml.v2"
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

	if strings.Contains(*labels, "release") {
		return
	}

	addedMetadataFilePaths := filterForMetadata(*added)
	modifiedMetadataFilePaths := filterForMetadata(*modified)

	if len(addedMetadataFilePaths) != 0 {
		testAddedMetadata(t, addedMetadataFilePaths)
	}

	if len(modifiedMetadataFilePaths) != 0 {
		testModifiedMetadata(t, modifiedMetadataFilePaths)
	}
}

func filterForMetadata(filePaths string) []string {
	metadataFilePaths := make([]string, 0)
	for _, filePath := range strings.Split(filePaths, " ") {
		if strings.Index(filePath, "metadata.yaml") > -1 {
			metadataFilePaths = append(metadataFilePaths, filePath)
		}
	}
	return metadataFilePaths
}

func testAddedMetadata(t *testing.T, dirs []string) {
	for _, dir := range dirs {
		dir := dir
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			m := metadata.IntegrationMetadata{}
			if err := unmarshalYamlFromFile(path.Join(basePath, dir), &m); err != nil {
				t.Fatal(err)
			}

			var emptyMetadata metadata.MinimumSupportedAgentVersion
			if m.MinimumSupportedAgentVersion != emptyMetadata {
				t.Fatal("Cannot add metadata.MinimumSupportedAgentVersion")
			}
		})
	}
}

func testModifiedMetadata(t *testing.T, dirs []string) {
	for _, dir := range dirs {
		dir := dir
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			var originalMetadata metadata.IntegrationMetadata
			var modifiedMetadata metadata.IntegrationMetadata

			if err := unmarshalYamlFromFile(path.Join(basePath, dir), &modifiedMetadata); err != nil {
				t.Fatal(err)
			}

			if err := unmarshalYamlFromFile(path.Join(basePath, *baseDir, dir), &originalMetadata); err != nil {
				t.Fatal(err)
			}

			if modifiedMetadata.MinimumSupportedAgentVersion != originalMetadata.MinimumSupportedAgentVersion {
				t.Fatal(fmt.Errorf("minimum_supported_agent_version has been modified for path: %s", dir))
			}
		})
	}
}

func unmarshalYamlFromFile(dir string, i interface{}) error {
	bytes, err := os.ReadFile(dir)
	if err != nil {
		return err
	}
	yml := strings.ReplaceAll(string(bytes), "\r\n", "\n")
	return yaml.Unmarshal([]byte(yml), i)
}
