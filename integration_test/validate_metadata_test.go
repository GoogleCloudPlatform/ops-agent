package integration

import (
	"embed"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/common"
	"gopkg.in/yaml.v2"
)

//go:embed agent_metrics/metadata.yaml
var agentMetricsMetadata []byte

//go:embed third_party_apps_data/applications
var thirdPartyDataDir embed.FS

func TestValidateMetadataOfThirdPartyApps(t *testing.T) {
	err := fs.WalkDir(thirdPartyDataDir, ".", func(path string, info fs.DirEntry, err error) error {
		if info.Name() != "metadata.yaml" {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return validateMetadata(contents, &common.IntegrationMetadata{})
	})
	if err != nil {
		t.Error(err)
	}
}

func TestValidateMetadataOfAgentMetric(t *testing.T) {

	err := validateMetadata(agentMetricsMetadata, &common.ExpectedMetricsContainer{})
	if err != nil {
		t.Error(err)
	}
}

func validateMetadata(bytes []byte, i interface{}) error {
	yamlStr := strings.ReplaceAll(string(bytes), "\r\n", "\n")

	v := common.NewIntegrationMetadataValidator()
	err := yaml.UnmarshalStrict([]byte(yamlStr), i)
	if err != nil {
		return err
	}
	return v.Struct(i)
}
