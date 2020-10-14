// Copyright 2020 Google LLC
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

package confgenerator

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	validTestdataDir   = "testdata/valid"
	invalidTestdataDir = "testdata/invalid"
	defaultGoldenPath  = "default_config"
	defaultLogsDir     = "/var/log/google-cloud-ops-agent/subagents"
	defaultStateDir    = "/var/lib/google-cloud-ops-agent/fluent-bit"
)

var (
	validUnifiedConfigFilepathFormat  = validTestdataDir + "/%s/input.yaml"
	validMainConfigFilepathFormat     = validTestdataDir + "/%s/golden_fluent_bit_main.conf"
	validParserConfigFilepathFormat   = validTestdataDir + "/%s/golden_fluent_bit_parser.conf"
	validCollectdConfigFilepathFormat = validTestdataDir + "/%s/golden_collectd.conf"
	invalidTestdataFilepathFormat     = invalidTestdataDir + "/%s"
)

func TestExtractFluentBitConfValidInput(t *testing.T) {
	dirPath := validTestdataDir
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilepath := fmt.Sprintf(validUnifiedConfigFilepathFormat, testName)
			// Special-case the default config.  It lives directly in the
			// confgenerator directory.  The golden files are still in the
			// testdata directory.
			if d.Name() == "default_config" {
				unifiedConfigFilepath = "default-config.yaml"
			}
			goldenMainConfigFilepath := fmt.Sprintf(validMainConfigFilepathFormat, testName)
			goldenParserConfigFilepath := fmt.Sprintf(validParserConfigFilepathFormat, testName)
			goldenCollectdConfigFilepath := fmt.Sprintf(validCollectdConfigFilepathFormat, testName)

			unifiedConfig, err := ioutil.ReadFile(unifiedConfigFilepath)
			if err != nil {
				t.Errorf("test %q: expect no error, get error %s", testName, err)
				return
			}
			rawExpectedMainConfig, err := ioutil.ReadFile(goldenMainConfigFilepath)
			if err != nil {
				t.Logf("test %q: Fluent Bit main conf not detected at %s. Using the default instead.", testName, goldenMainConfigFilepath)
				rawExpectedMainConfig, err = ioutil.ReadFile(fmt.Sprintf(validMainConfigFilepathFormat, defaultGoldenPath))
				if err != nil {
					t.Errorf("test %q: expect no error, get error %s", testName, err)
					return
				}
			}
			expectedMainConfig := string(rawExpectedMainConfig)
			rawExpectedParserConfig, err := ioutil.ReadFile(goldenParserConfigFilepath)
			if err != nil {
				t.Logf("test %q: Fluent Bit parser conf not detected at %s. Using the default instead.", testName, goldenParserConfigFilepath)
				rawExpectedParserConfig, err = ioutil.ReadFile(fmt.Sprintf(validParserConfigFilepathFormat, defaultGoldenPath))
				if err != nil {
					t.Errorf("test %q: expect no error, get error %s", testName, err)
					return
				}
			}
			expectedParserConfig := string(rawExpectedParserConfig)
			rawExpectedCollectdConfig, err := ioutil.ReadFile(goldenCollectdConfigFilepath)
			if err != nil {
				t.Logf("test %q: Collectd conf not detected at %s. Using the default instead.", testName, goldenCollectdConfigFilepath)
				rawExpectedCollectdConfig, err = ioutil.ReadFile(fmt.Sprintf(validCollectdConfigFilepathFormat, defaultGoldenPath))
				if err != nil {
					t.Errorf("test %q: expect no error, get error %v", testName, err)
					return
				}
			}
			expectedCollectdConfig := string(rawExpectedCollectdConfig)

			mainConf, parserConf, err := GenerateFluentBitConfigs(unifiedConfig, defaultLogsDir, defaultStateDir)
			if err != nil {
				t.Errorf("test %q: expect no error, get error %s", testName, err)
				return
			}
			if diff := cmp.Diff(expectedMainConfig, mainConf); diff != "" {
				t.Errorf("test %q: fluentbit main configuration mismatch (-want +got):\n%s", testName, diff)
			}
			if diff := cmp.Diff(expectedParserConfig, parserConf); diff != "" {
				t.Errorf("test %q: fluentbit parser configuration mismatch (-want +got):\n%s", testName, diff)
			}

			collectdConf, err := GenerateCollectdConfig(unifiedConfig, defaultLogsDir)
			if err != nil {
				t.Errorf("test %q: expect no error, get error %s", testName, err)
				return
			}
			if diff := cmp.Diff(expectedCollectdConfig, collectdConf); diff != "" {
				t.Errorf("test %q: collectd configuration mismatch (-want +got):\n%s", testName, diff)
			}
		})
	}
}

func TestExtractFluentBitConfInvalidInput(t *testing.T) {
	filePath := invalidTestdataDir
	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		testName := f.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilePath := fmt.Sprintf(invalidTestdataFilepathFormat, testName)
			unifiedConfig, err := ioutil.ReadFile(unifiedConfigFilePath)
			if err != nil {
				t.Errorf("test %q: expect no error, get error %s", testName, err)
				return
			}
			if _, _, err := GenerateFluentBitConfigs(unifiedConfig, defaultLogsDir, defaultStateDir); err == nil {
				t.Errorf("test %q: GenerateFluentBitConfigs succeeded, want error. file:\n%s", testName, unifiedConfig)
			}
		})
	}
}
