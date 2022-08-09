package integration

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"go.uber.org/multierr"
	"gopkg.in/yaml.v2"
)

//go:embed agent_metrics/metadata.yaml
var agentMetricsMetadata []byte

//go:embed third_party_apps_data/applications
var thirdPartyDataDir embed.FS

func TestValidateMetadataOfThirdPartyApps(t *testing.T) {
	err := walkThirdPartyApps(func(contents []byte) error {
		return parseAndValidateMetadata(contents, &metadata.IntegrationMetadata{})
	})
	if err != nil {
		t.Error(err)
	}
}

func TestMissingMetadataForThirdPartyApps(t *testing.T) {
	parentDirectory := "third_party_apps_data/applications"
	dirs, err := thirdPartyDataDir.ReadDir(parentDirectory)
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range dirs {
		_, fileErr := os.Stat(path.Join(parentDirectory, dir.Name(), "metadata.yaml"))
		if fileErr != nil {
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
		err := parseAndValidateMetadata(contents, integrationMetadata)
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

	err := parseAndValidateMetadata(agentMetricsMetadata, &metadata.ExpectedMetricsContainer{})
	if err != nil {
		t.Error(err)
	}
}

func parseAndValidateMetadata(bytes []byte, i interface{}) error {
	yamlStr := strings.ReplaceAll(string(bytes), "\r\n", "\n")

	v := metadata.NewIntegrationMetadataValidator()
	err := yaml.UnmarshalStrict([]byte(yamlStr), i)
	if err != nil {
		return err
	}
	return v.Struct(i)
}
