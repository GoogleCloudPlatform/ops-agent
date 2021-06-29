// Copyright 2021 Google LLC
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

// Package confgenerator provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

var (
	builtInConfStructs = map[string]*UnifiedConfig{
		"linux": &UnifiedConfig{
			Logging: &Logging{
				Receivers: map[string]*LoggingReceiver{
					"syslog": &LoggingReceiver{
						configComponent: configComponent{Type: "files"},
						LoggingReceiverFiles: LoggingReceiverFiles{
							IncludePaths: []string{"/var/log/messages", "/var/log/syslog"},
						},
					},
				},
				Processors: map[string]*LoggingProcessor{},
				Exporters: map[string]*LoggingExporter{
					"google": &LoggingExporter{
						configComponent: configComponent{Type: "google_cloud_logging"},
					},
				},
				Service: &LoggingService{
					Pipelines: map[string]*LoggingPipeline{
						"default_pipeline": &LoggingPipeline{
							ReceiverIDs: []string{"syslog"},
							ExporterIDs: []string{"google"},
						},
					},
				},
			},
			Metrics: &Metrics{
				Receivers: map[string]*MetricsReceiver{
					"hostmetrics": &MetricsReceiver{
						configComponent:    configComponent{Type: "hostmetrics"},
						CollectionInterval: "60s",
					},
				},
				Exporters: map[string]*MetricsExporter{
					"google": &MetricsExporter{
						configComponent: configComponent{Type: "google_cloud_monitoring"},
					},
				},
				Service: &MetricsService{
					Pipelines: map[string]*MetricsPipeline{
						"default_pipeline": &MetricsPipeline{
							ReceiverIDs: []string{"hostmetrics"},
							ExporterIDs: []string{"google"},
						},
					},
				},
			},
		},
		"windows": &UnifiedConfig{
			Logging: &Logging{
				Receivers: map[string]*LoggingReceiver{
					"windows_event_log": &LoggingReceiver{
						configComponent: configComponent{Type: "windows_event_log"},
						LoggingReceiverWinevtlog: LoggingReceiverWinevtlog{
							Channels: []string{"System", "Application", "Security"},
						},
					},
				},
				Processors: map[string]*LoggingProcessor{},
				Exporters: map[string]*LoggingExporter{
					"google": &LoggingExporter{
						configComponent: configComponent{Type: "google_cloud_logging"},
					},
				},
				Service: &LoggingService{
					Pipelines: map[string]*LoggingPipeline{
						"default_pipeline": &LoggingPipeline{
							ReceiverIDs: []string{"windows_event_log"},
							ExporterIDs: []string{"google"},
						},
					},
				},
			},
			Metrics: &Metrics{
				Receivers: map[string]*MetricsReceiver{
					"hostmetrics": &MetricsReceiver{
						configComponent:    configComponent{Type: "hostmetrics"},
						CollectionInterval: "60s",
					},
					"iis": &MetricsReceiver{
						configComponent:    configComponent{Type: "iis"},
						CollectionInterval: "60s",
					},
					"mssql": &MetricsReceiver{
						configComponent:    configComponent{Type: "mssql"},
						CollectionInterval: "60s",
					},
				},
				Exporters: map[string]*MetricsExporter{
					"google": &MetricsExporter{
						configComponent: configComponent{Type: "google_cloud_monitoring"},
					},
				},
				Service: &MetricsService{
					Pipelines: map[string]*MetricsPipeline{
						"default_pipeline": &MetricsPipeline{
							ReceiverIDs: []string{"hostmetrics", "iis", "mssql"},
							ExporterIDs: []string{"google"},
						},
					},
				},
			},
		},
	}
)

func MergeConfFiles(userConfPath, confDebugFolder, platform string) error {
	builtInConfPath := filepath.Join(confDebugFolder, "built-in-config.yaml")
	mergedConfPath := filepath.Join(confDebugFolder, "merged-config.yaml")
	return mergeConfFiles(builtInConfPath, userConfPath, mergedConfPath, platform)
}

func mergeConfFiles(builtInConfPath, userConfPath, mergedConfPath, platform string) error {
	builtInStruct := builtInConfStructs[platform]
	builtInYaml, err := yaml.Marshal(builtInStruct)
	if err != nil {
		return fmt.Errorf("failed to convert the built-in config %q to yaml: %w \n", builtInConfPath, err)
	}

	// Write the built-in conf to disk for debugging purpose.

	if err := writeConfigFile(builtInYaml, builtInConfPath); err != nil {
		return err
	}

	// Read the built-in config file.
	original, err := builtInStruct.DeepCopy()
	if err != nil {
		return err
	}

	// Optionally merge the user config file.
	if _, err = os.Stat(userConfPath); err != nil {
		if os.IsNotExist(err) {
			// Skip the merge if the user config file does not exist.
		} else {
			return fmt.Errorf("failed to retrieve the user config file %q: %w \n", userConfPath, err)
		}
	} else {
		overrides, err := ReadUnifiedConfigFromFile(userConfPath)
		if err != nil {
			return err
		}
		mergeConfigs(&original, &overrides)
	}

	// Write the merged conf file.
	configBytes, err := yaml.Marshal(original)
	if err != nil {
		return fmt.Errorf("failed to convert the merged config %q to yaml: %w \n", mergedConfPath, err)
	}
	if err := ioutil.WriteFile(mergedConfPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write the merged config file %q: %w \n", mergedConfPath, err)
	}
	return nil
}

func mergeConfigs(original, overrides *UnifiedConfig) {
	// For "default_pipeline", we go one level deeper.
	// this covers 2 cases:
	// 1. if "<receivers / processors / exporters>: []" is specified explicitly in user config, the entity gets cleared.
	// 2. if "<receivers / processors / exporters>" is a non-empty list, it overrides the built-in list.
	//    e.g. users might use the config below to turn off iis metrics on windows.
	//    metrics:
	//      service:
	//        pipelines:
	//          default_pipeline:
	//            receivers: [hostmetrics,mssql]
	if overrides.Logging != nil {
		// Overrides logging.receivers.
		for k, v := range overrides.Logging.Receivers {
			original.Logging.Receivers[k] = v
		}

		// Overrides logging.processors.
		original.Logging.Processors = map[string]*LoggingProcessor{}
		for k, v := range overrides.Logging.Processors {
			original.Logging.Processors[k] = v
		}

		// Overrides logging.exporters.
		for k, v := range overrides.Logging.Exporters {
			original.Logging.Exporters[k] = v
		}
		if overrides.Logging.Service != nil {
			for name, pipeline := range overrides.Logging.Service.Pipelines {
				if name == "default_pipeline" {
					// overrides logging.service.pipelines.default_pipeline.receivers
					if ids := pipeline.ReceiverIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ReceiverIDs = ids
					}

					// overrides logging.service.pipelines.default_pipeline.processors
					if ids := pipeline.ProcessorIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ProcessorIDs = ids
					}

					// overrides logging.service.pipelines.default_pipeline.exporters
					if ids := pipeline.ExporterIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ExporterIDs = ids
					}
				} else {
					// Overrides logging.service.pipelines.<non_default_pipelines>
					original.Logging.Service.Pipelines[name] = pipeline
				}
			}
		}
	}
	if overrides.Metrics != nil {
		// Overrides metrics.receivers.
		for k, v := range overrides.Metrics.Receivers {
			original.Metrics.Receivers[k] = v
		}

		// Overrides metrics.processors.
		original.Metrics.Processors = map[string]*MetricsProcessor{}
		for k, v := range overrides.Metrics.Processors {
			original.Metrics.Processors[k] = v
		}

		// Overrides metrics.exporters.
		for k, v := range overrides.Metrics.Exporters {
			original.Metrics.Exporters[k] = v
		}

		if overrides.Metrics.Service != nil {
			for name, pipeline := range overrides.Metrics.Service.Pipelines {
				if name == "default_pipeline" {
					// overrides metrics.service.pipelines.default_pipeline.receivers
					if ids := pipeline.ReceiverIDs; ids != nil {
						original.Metrics.Service.Pipelines["default_pipeline"].ReceiverIDs = ids
					}

					//
					// overrides metrics.service.pipelines.default_pipeline.processors
					if ids := pipeline.ProcessorIDs; ids != nil {
						original.Metrics.Service.Pipelines["default_pipeline"].ProcessorIDs = ids
					}

					// Overrides metrics.service.pipelines.default_pipeline.exporters
					if ids := pipeline.ExporterIDs; ids != nil {
						original.Metrics.Service.Pipelines["default_pipeline"].ExporterIDs = ids
					}
				} else {
					// Overrides metrics.service.pipelines.<non_default_pipelines>
					original.Metrics.Service.Pipelines[name] = pipeline
				}
			}
		}
	}
}
