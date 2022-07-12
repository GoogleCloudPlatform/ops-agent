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
		v := common.NewIntegrationMetadataValidator()

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		yamlStr := strings.ReplaceAll(string(contents), "\r\n", "\n")

		integrationMetadata := &common.IntegrationMetadata{}

		err = yaml.UnmarshalStrict([]byte(yamlStr), integrationMetadata)
		if err != nil {
			return err
		}
		return v.Struct(integrationMetadata)
	})

	if err != nil {
		t.Error(err)
	}
}
