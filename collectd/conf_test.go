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

package collectd

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	defaultLogsDir = "/var/log/google-cloud-ops-agent/subagents"
)

var (
	updateGolden = flag.Bool("update_golden", false, "Whether to update the golden files if they differ or don't exist.")
)

var validConfigTests = map[string]Metrics{
	"empty": {},
	"custom_interval": {
		Receivers: map[string]Receiver{
			"custom_interval_receiver": {
				Type:               "hostmetrics",
				CollectionInterval: "30s",
			},
		},
		Exporters: map[string]Exporter{
			"google_exporter": {
				Type: "google_cloud_monitoring",
			},
		},
		Service: Service{
			Pipelines: map[string]Pipeline{
				"pipeline_id": Pipeline{
					ReceiverIDs: []string{
						"custom_interval_receiver",
					},
					ExporterIDs: []string{
						"google_exporter",
					},
				},
			},
		},
	},
	"all_hostmetrics": {
		Receivers: map[string]Receiver{
			"all_hostmetrics_exporter": {
				Type: "hostmetrics",
			},
		},
		Exporters: map[string]Exporter{
			"google_exporter": {
				Type: "google_cloud_monitoring",
			},
		},
		Service: Service{
			Pipelines: map[string]Pipeline{
				"pipeline_id": Pipeline{
					ReceiverIDs: []string{
						"all_hostmetrics_exporter",
					},
					ExporterIDs: []string{
						"google_exporter",
					},
				},
			},
		},
	},
}

func TestValidInput(t *testing.T) {
	for testName, metricsConfig := range validConfigTests {
		t.Run(testName, func(t *testing.T) {
			goldenFilepath := filepath.Join("testdata", testName+".golden")

			conf, err := GenerateCollectdConfig(metricsConfig, defaultLogsDir)
			if err != nil {
				t.Errorf("error running GenerateCollectdConfig(%+v): %s", metricsConfig, err)
				return
			}

			if result := compareWithGolden(goldenFilepath, conf); result != "" {
				t.Errorf("GenerateCollectdConfig(%+v) failed unexpectedly: %s", metricsConfig, result)
			}
		})
	}
}

func TestOutOfBoundsScrapeInterval(t *testing.T) {
	invalidScrapeIntervalConfig := Metrics{
		Receivers: map[string]Receiver{
			"custom_interval_receiver": {
				Type:               "hostmetrics",
				CollectionInterval: "2s",
			},
		},
		Exporters: map[string]Exporter{
			"google_exporter": {
				Type: "google_cloud_monitoring",
			},
		},
		Service: Service{
			Pipelines: map[string]Pipeline{
				"pipeline_id": Pipeline{
					ReceiverIDs: []string{
						"custom_interval_receiver",
					},
					ExporterIDs: []string{
						"google_exporter",
					},
				},
			},
		},
	}

	conf, err := GenerateCollectdConfig(invalidScrapeIntervalConfig, defaultLogsDir)
	if err == nil {
		fmt.Printf("Unexpected successful result:\n%s", conf)
		t.Errorf("GenerateCollectdConfig(%+v): got err == nil, want err != nil.", invalidScrapeIntervalConfig)
	}
}

func TestInvalidScrapeInterval(t *testing.T) {
	scrapeIntervalMissingSuffixConfig := Metrics{
		Receivers: map[string]Receiver{
			"custom_interval_receiver": {
				Type:               "hostmetrics",
				CollectionInterval: "24",
			},
		},
		Exporters: map[string]Exporter{
			"google_exporter": {
				Type: "google_cloud_monitoring",
			},
		},
		Service: Service{
			Pipelines: map[string]Pipeline{
				"pipeline_id": Pipeline{
					ReceiverIDs: []string{
						"custom_interval_receiver",
					},
					ExporterIDs: []string{
						"google_exporter",
					},
				},
			},
		},
	}

	conf, err := GenerateCollectdConfig(scrapeIntervalMissingSuffixConfig, defaultLogsDir)
	if err == nil {
		fmt.Printf("Unexpected successful result:\n%s", conf)
		t.Errorf("GenerateCollectdConfig(%+v): got err == nil, want err != nil.", scrapeIntervalMissingSuffixConfig)
	}
}

func compareWithGolden(goldenFilepath string, actual string) string {
	if *updateGolden {
		if err := ioutil.WriteFile(goldenFilepath, []byte(actual), 0644); err != nil {
			return fmt.Sprintf("error updating golden file (%s): %s", goldenFilepath, err)
		}
		return ""
	}

	expected, err := ioutil.ReadFile(goldenFilepath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Sprintf("Golden file (%s) doesn't exist. Run test with `--update_golden` to create.", goldenFilepath)
		}
		return fmt.Sprintf("error reading golden file (%s): %s", goldenFilepath, err)
	}

	if diff := cmp.Diff(string(expected), actual); diff != "" {
		return fmt.Sprintf("generated collectd config differs from golden (-want +got):\n%s\nRun test with `--update_golden` to update.", diff)
	}
	return ""
}
