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

//go:embed third_party_apps_data/applications
var thirdPartyDataDir embed.FS

func Test_ValidateMetadataOfThirdPartyApps(t *testing.T) {
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

func Test_ValidateMetadataOfAgentMetric(t *testing.T) {

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
