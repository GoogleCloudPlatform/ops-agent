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
	"flag"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"github.com/goccy/go-yaml"
)

const (
	// relative path to testdata folder
	testdataDir     = "testdata"
	inputYamlName   = "input.yaml"
	goldenErrorName = "golden_error"
)

var (
	updateGolden = flag.Bool("update_golden", false, "Whether to update the expected golden confs if they differ from the actual generated confs.")
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
			if *updateGolden {
				generateNewGolden(t, dirName)
				return
			}

			testMetadataValidation(t, dirName)
		})
	}
}

func generateNewGolden(t *testing.T, dir string) {
	t.Parallel()
	goldenPath := path.Join(testdataDir, dir, goldenErrorName)
	yamlStr := getTestFile(t, dir, inputYamlName)
	err := metadata.UnmarshalAndValidate([]byte(yamlStr), &metadata.IntegrationMetadata{})

	errStr := ""
	if err != nil {
		errStr = yaml.FormatError(err, false, false)
	}

	if err = os.WriteFile(goldenPath, []byte(errStr), 0644); err != nil {
		t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
	}
}

func testMetadataValidation(t *testing.T, dir string) {
	t.Parallel()

	yamlStr := getTestFile(t, dir, inputYamlName)
	goldenErrStr := getTestFile(t, dir, goldenErrorName)

	var md metadata.IntegrationMetadata
	actualError := metadata.UnmarshalAndValidate([]byte(yamlStr), &md)

	if actualError == nil {
		if goldenErrStr == "" {
			return
		}
		t.Fatal("Expecting validation to fail for test: " + dir)
	}
	actualErrorStr := yaml.FormatError(actualError, false, false)

	if actualErrorStr != goldenErrStr {
		t.Fatalf("Unexpected errors detected: \nExpected error: \n%s\nActual error:  \n%s\n", goldenErrStr, actualErrorStr)
	}
}
