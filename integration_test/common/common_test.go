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

func getYaml(t *testing.T, dirName string) []byte {
	dir := path.Join(testdataDir, dirName, inputYamlName)
	contents, err := ioutil.ReadFile(dir)
	if err != nil {
		t.Fatal("could not read dirName: " + dir)
	}
	str := strings.ReplaceAll(string(contents), "\r\n", "\n")

	return []byte(str)
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
			"pass",
			nil,
		},
		{
			"integration-metadata_required_app-url",
			map[fieldError]struct{}{
				{"AppUrl", "required"}: {},
			},
		},
		{
			"configuration-options_required_without_metrics-configuration",
			map[fieldError]struct{}{
				{"LogsConfiguration", "required_without"}:    {},
				{"MetricsConfiguration", "required_without"}: {},
			},
		},
		{
			"expected-metric_one_of_value-type",
			map[fieldError]struct{}{
				{"ValueType", "oneof"}: {},
			},
		},
		{
			"expected-metric_excluded_with_optional",
			map[fieldError]struct{}{
				{"Representative", "excluded_with"}: {},
				{"Optional", "excluded_with"}:       {},
			},
		},
		{
			"integration-metadata_unique_supported-app-version",
			map[fieldError]struct{}{
				{"SupportedAppVersion", "unique"}: {},
			},
		},
		{
			"integration-metadata_unique_supported-app-version",
			map[fieldError]struct{}{
				{"SupportedAppVersion", "unique"}: {},
			},
		},
	}

	for _, test := range table {
		t.Run(test.dirName, func(t *testing.T) {
			testDir(t, test)
		})
	}
}

func testDir(t *testing.T, test testCase) {
	t.Parallel()
	fmt.Println("test.dirName: " + test.dirName)
	bytes := getYaml(t, test.dirName)
	actualErrors := UnmarshallAndValidate(t, bytes, &IntegrationMetadata{})

	//testdata/pass
	if test.expectedErrors == nil {
		if actualErrors == nil {
			return
		} else {
			t.Fatal("Expecting no errors")
		}
	}

	if actualErrors != nil {
		fieldErrors := actualErrors.(validator.ValidationErrors)
		if len(fieldErrors) != len(test.expectedErrors) {
			t.Fatal("Expecting validation errors to equal expected errors")
		}
		expectedErrorsMap := test.expectedErrors
		actualErrorsMap := map[fieldError]struct{}{}
		for _, err := range fieldErrors {
			actualError := err.(validator.FieldError)
			actualErrorsMap[fieldError{actualError.Field(), actualError.Tag()}] = struct{}{}
		}

		for i := range actualErrorsMap {
			delete(expectedErrorsMap, i)
		}

		if len(expectedErrorsMap) > 0 {
			t.Fatal(fmt.Sprintf("Unexcpected errors detected: \n Expected error: %v but got %v", test.expectedErrors, actualErrorsMap))
		}
	} else {
		t.Fatal("Expecting validation to fail for test: " + test.dirName)
	}
}
