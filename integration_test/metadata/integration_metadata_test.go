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

package metadata_test

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"github.com/goccy/go-yaml"
	"gotest.tools/v3/golden"
)

const (
	// relative path to testdata folder
	testdataDir     = "testdata"
	inputYamlName   = "input.yaml"
	goldenErrorName = "golden_error"
)

func getTestFile(t *testing.T, dirName, fileName string) string {
	filePath := path.Join(testdataDir, dirName, fileName)
	contents, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal("could not read dirName: " + filePath)
	}
	return strings.ReplaceAll(string(contents), "\r\n", "\n")
}

func TestMetadataValidation(t *testing.T) {
	dirs, err := os.ReadDir(testdataDir)

	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range dirs {
		dirName := dir.Name()
		t.Run(dirName, func(t *testing.T) {
			// We want to parallelize the test cases, so we pass the test case into a
			// separate function
			testMetadataValidation(t, dirName)
		})
	}
}

func testMetadataValidation(t *testing.T, dir string) {
	t.Parallel()
	yamlStr := getTestFile(t, dir, inputYamlName)

	var md metadata.IntegrationMetadata
	actualError := metadata.UnmarshalAndValidate([]byte(yamlStr), &md)

	actualErrorStr := ""
	if actualError != nil {
		actualErrorStr = actualError.Error()
		actualErrorStr = yaml.FormatError(actualError, false, false)
	}
	golden.Assert(t, actualErrorStr, path.Join(dir, goldenErrorName))
}
