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

//go:build integration_test

package common

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
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
	contents, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatal("could not read dirName: " + filePath)
	}
	return strings.ReplaceAll(string(contents), "\r\n", "\n")
}

func UnmarshallAndValidate(t *testing.T, bytes []byte, i interface{}) error {
	v := validator.New()
	err := yaml.UnmarshalStrict(bytes, i)
	if err != nil {
		t.Fatal(err)
	}
	return v.Struct(i)
}

func TestAll(t *testing.T) {
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
	err := UnmarshallAndValidate(t, []byte(yamlStr), &IntegrationMetadata{})

	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	if err = ioutil.WriteFile(goldenPath, []byte(errStr), 0644); err != nil {
		t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
	}
}

func testMetadataValidation(t *testing.T, dir string) {
	t.Parallel()

	yamlStr := getTestFile(t, dir, inputYamlName)
	goldenErrStr := getTestFile(t, dir, goldenErrorName)

	actualError := UnmarshallAndValidate(t, []byte(yamlStr), &IntegrationMetadata{})

	if actualError == nil {
		if goldenErrStr == "" {
			return
		}
		t.Fatal("Expecting validation to fail for test: " + dir)
	}

	if actualError.Error() != goldenErrStr {
		t.Fatal(fmt.Sprintf("Unexpected errors detected: \n Expected error: \n%s\n Actual error:  \n%s\n", goldenErrStr, actualError.Error()))
	}
}
