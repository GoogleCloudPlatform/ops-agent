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
				Receivers: map[string]LoggingReceiver{
					"syslog": &LoggingReceiverFiles{
						configComponent: configComponent{ComponentType: "files"},
						IncludePaths:    []string{"/var/log/messages", "/var/log/syslog"},
					},
				},
				Processors: map[string]*LoggingProcessor{},
				Service: &LoggingService{
					Pipelines: map[string]*LoggingPipeline{
						"default_pipeline": &LoggingPipeline{
							ReceiverIDs: []string{"syslog"},
						},
					},
				},
			},
			Metrics: &Metrics{
				Receivers: map[string]*MetricsReceiver{
					"hostmetrics": &MetricsReceiver{
						configComponent:    configComponent{ComponentType: "hostmetrics"},
						CollectionInterval: "60s",
					},
				},
				Processors: map[string]*MetricsProcessor{
					"metrics_filter": &MetricsProcessor{
						configComponent: configComponent{ComponentType: "exclude_metrics"},
					},
				},
				Service: &MetricsService{
					Pipelines: map[string]*MetricsPipeline{
						"default_pipeline": &MetricsPipeline{
							ReceiverIDs:  []string{"hostmetrics"},
							ProcessorIDs: []string{"metrics_filter"},
						},
					},
				},
			},
		},
		"windows": &UnifiedConfig{
			Logging: &Logging{
				Receivers: map[string]LoggingReceiver{
					"windows_event_log": &LoggingReceiverWinevtlog{
						configComponent: configComponent{ComponentType: "windows_event_log"},
						Channels:        []string{"System", "Application", "Security"},
					},
				},
				Processors: map[string]*LoggingProcessor{},
				Service: &LoggingService{
					Pipelines: map[string]*LoggingPipeline{
						"default_pipeline": &LoggingPipeline{
							ReceiverIDs: []string{"windows_event_log"},
						},
					},
				},
			},
			Metrics: &Metrics{
				Receivers: map[string]*MetricsReceiver{
					"hostmetrics": &MetricsReceiver{
						configComponent:    configComponent{ComponentType: "hostmetrics"},
						CollectionInterval: "60s",
					},
					"iis": &MetricsReceiver{
						configComponent:    configComponent{ComponentType: "iis"},
						CollectionInterval: "60s",
					},
					"mssql": &MetricsReceiver{
						configComponent:    configComponent{ComponentType: "mssql"},
						CollectionInterval: "60s",
					},
				},
				Processors: map[string]*MetricsProcessor{
					"metrics_filter": &MetricsProcessor{
						configComponent: configComponent{ComponentType: "exclude_metrics"},
					},
				},
				Service: &MetricsService{
					Pipelines: map[string]*MetricsPipeline{
						"default_pipeline": &MetricsPipeline{
							ReceiverIDs:  []string{"hostmetrics", "iis", "mssql"},
							ProcessorIDs: []string{"metrics_filter"},
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
		// Skip deprecated logging.exporters.
		// Override logging.service.pipelines
		if overrides.Logging.Service != nil {
			for name, pipeline := range overrides.Logging.Service.Pipelines {
				// skips logging.service.pipelines.*.exporters
				pipeline.ExporterIDs = nil
				if name == "default_pipeline" {
					// overrides logging.service.pipelines.default_pipeline.receivers
					if ids := pipeline.ReceiverIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ReceiverIDs = ids
					}

					// overrides logging.service.pipelines.default_pipeline.processors
					if ids := pipeline.ProcessorIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ProcessorIDs = ids
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
		for k, v := range overrides.Metrics.Processors {
			original.Metrics.Processors[k] = v
		}

		// Overrides metrics.exporters.
		original.Metrics.Exporters = map[string]*MetricsExporter{}
		for k, v := range overrides.Metrics.Exporters {
			original.Metrics.Exporters[k] = v
		}

		if overrides.Metrics.Service != nil {
			for name, pipeline := range overrides.Metrics.Service.Pipelines {
				// skips metrics.service.pipelines.*.exporters
				pipeline.ExporterIDs = nil
				// Overrides metrics.service.pipelines.*
				original.Metrics.Service.Pipelines[name] = pipeline
			}
		}
	}
}
