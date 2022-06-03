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
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
)

const (
	//relative path to testdata folder
	testdataDir   = "testdata"
	inputYamlName = "input.yaml"
)

type testCase struct {
	dirName        string
	expectedErrors map[fieldError]struct{}
}

type fieldError struct {
	field string
	tag   string
}

func getInputYamlBytes(t *testing.T, dirName string) []byte {
	yamlFilePath := path.Join(testdataDir, dirName, inputYamlName)
	contents, err := ioutil.ReadFile(yamlFilePath)
	if err != nil {
		t.Fatal("could not read dirName: " + yamlFilePath)
	}
	escapedYamlStr := strings.ReplaceAll(string(contents), "\r\n", "\n")

	return []byte(escapedYamlStr)
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
	table := []testCase{
		{
			dirName:        "pass",
			expectedErrors: nil,
		},
		{
			dirName: "integration-metadata_required_app-url",
			expectedErrors: map[fieldError]struct{}{
				{field: "AppUrl", tag: "required"}: {},
			},
		},
		{
			dirName: "configuration-options_required_without_metrics-configuration",
			expectedErrors: map[fieldError]struct{}{
				{field: "LogsConfiguration", tag: "required_without"}:    {},
				{field: "MetricsConfiguration", tag: "required_without"}: {},
			},
		},
		{
			dirName: "expected-metric_one_of_value-type",
			expectedErrors: map[fieldError]struct{}{
				{field: "ValueType", tag: "oneof"}: {},
			},
		},
		{
			dirName: "expected-metric_excluded_with_optional",
			expectedErrors: map[fieldError]struct{}{
				{field: "Representative", tag: "excluded_with"}: {},
				{field: "Optional", tag: "excluded_with"}:       {},
			},
		},
		{
			dirName: "integration-metadata_unique_supported-app-version",
			expectedErrors: map[fieldError]struct{}{
				{field: "SupportedAppVersion", tag: "unique"}: {},
			},
		},
		{
			dirName: "integration-metadata_unique_supported-app-version",
			expectedErrors: map[fieldError]struct{}{
				{field: "SupportedAppVersion", tag: "unique"}: {},
			},
		},
	}

	for _, test := range table {
		t.Run(test.dirName, func(t *testing.T) {
			//We want to parallelize the test cases, so we pass the test case into a
			//separate function
			testMetadataValidation(t, test)
		})
	}
}

func testMetadataValidation(t *testing.T, test testCase) {
	t.Parallel()
	bytes := getInputYamlBytes(t, test.dirName)
	actualErrors := UnmarshallAndValidate(t, bytes, &IntegrationMetadata{})

	//testdata/pass
	if test.expectedErrors == nil {
		if actualErrors == nil {
			return
		}
		t.Fatal("Expecting no errors")
	}

	if actualErrors == nil {
		t.Fatal("Expecting validation to fail for test: " + test.dirName)
	}

	fieldErrors, ok := actualErrors.(validator.ValidationErrors)

	if !ok {
		t.Fatal("Expected validation error to of type validator.ValidationErrors")
	}

	if len(fieldErrors) != len(test.expectedErrors) {
		t.Fatal("Expecting validation errors to equal expected errors")
	}
	expectedErrorsMap := test.expectedErrors
	var actualErrorsSlice []fieldError
	for _, fieldErr := range fieldErrors {
		actualError := fieldError{fieldErr.Field(), fieldErr.Tag()}
		actualErrorsSlice = append(actualErrorsSlice, actualError)
		delete(expectedErrorsMap, actualError)
	}

	if len(expectedErrorsMap) > 0 {
		var expectedErrorsSlice []fieldError
		for k, _ := range expectedErrorsMap {
			expectedErrorsSlice = append(expectedErrorsSlice, k)
		}
		t.Fatal(fmt.Sprintf("Unexpected errors detected: \n Expected error: %v but got %v", expectedErrorsSlice, actualErrorsSlice))
	}

}
