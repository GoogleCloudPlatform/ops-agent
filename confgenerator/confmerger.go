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
	"path"

	"gopkg.in/yaml.v2"
)

var (
	// TODO(lingshi): Move this to a in-memory representation.
	builtInConf = map[string]string{
		"linux": `logging:
  receivers:
    syslog:
      type: files
      include_paths:
      - /var/log/messages
      - /var/log/syslog
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      default_pipeline:
        receivers: [syslog]
        exporters: [google]

metrics:
  receivers:
    hostmetrics:
      type: hostmetrics
      collection_interval: 60s
  exporters:
    google:
      type: google_cloud_monitoring
  service:
    pipelines:
      default_pipeline:
        receivers: [hostmetrics]
        exporters: [google]`,

		"windows": `logging:
  receivers:
    windows_event_log:
      type: windows_event_log
      channels: [System,Application,Security]
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      default_pipeline:
        receivers: [windows_event_log]
        exporters: [google]

metrics:
  receivers:
    hostmetrics:
      type: hostmetrics
      collection_interval: 60s
    mssql:
      type: mssql
      collection_interval: 60s
    iis:
      type: iis
      collection_interval: 60s
  exporters:
    google:
      type: google_cloud_monitoring
  service:
    pipelines:
      default_pipeline:
        receivers: [hostmetrics,mssql,iis]
        exporters: [google]`,
	}
)

func MergeConfFiles(builtInConfPath, userConfPath, mergedConfPath, platform string) error {

	// Write the built-in conf to disk for debugging purpose.
	if err := os.MkdirAll(path.Dir(builtInConfPath), 0777); err != nil {
		return fmt.Errorf("failed to create the folder for the built-in config file: %w \n", err)
	}
	f, err := os.Create(builtInConfPath)
	if err != nil {
		return fmt.Errorf("failed to create the built-in config file %q: %w \n", builtInConfPath, err)
	}
	defer f.Close()
	if _, err = f.WriteString(builtInConf[platform]); err != nil {
		return fmt.Errorf("failed to write to the built-in config file %q: %w \n", builtInConfPath, err)
	}

	// Read the built-in config file.
	original, err := ReadUnifiedConfigFromFile(builtInConfPath)
	if err != nil {
		return fmt.Errorf("failed to read config from the built-in config file %q: %w \n", builtInConfPath, err)
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

// TODO(lingshi): Refactor this part to de-duplicate repeated code.
func mergeConfigs(original, overrides *UnifiedConfig) {
	if overrides.Logging != nil {
		if overrides.Logging.Receivers != nil {
			for k, v := range overrides.Logging.Receivers {
				original.Logging.Receivers[k] = v
			}
		}
		if overrides.Logging.Processors != nil {
			if original.Logging.Processors == nil {
				original.Logging.Processors = map[string]*LoggingProcessor{}
			}
			for k, v := range overrides.Logging.Processors {
				original.Logging.Processors[k] = v
			}
		}
		if overrides.Logging.Exporters != nil {
			if original.Logging.Exporters == nil {
				original.Logging.Exporters = map[string]*LoggingExporter{}
			}
			for k, v := range overrides.Logging.Exporters {
				original.Logging.Exporters[k] = v
			}
		}
		if overrides.Logging.Service != nil {
			for name, pipeline := range overrides.Logging.Service.Pipelines {
				if name == "default_pipeline" {
					// If "receivers/processors/exporters" is not nil, overwrite the built-in instead of merging.
					// This covers 2 cases:
					// 1. "<entity>:[]" is specified explicitly in user config. We want to clear the entity.
					// 2. "<entity>" is a non-empty list. We want to overwrite the built-in instead of merging.
					rIDs := pipeline.ReceiverIDs
					pIDs := pipeline.ProcessorIDs
					eIDs := pipeline.ExporterIDs
					if rIDs != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ReceiverIDs = rIDs
					}
					if pIDs != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ProcessorIDs = pIDs
					}
					if eIDs != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ExporterIDs = eIDs
					}
				} else {
					original.Logging.Service.Pipelines[name] = pipeline
				}
			}
		}
	}
	if overrides.Metrics != nil {
		if overrides.Metrics.Receivers != nil {
			if original.Metrics.Receivers == nil {
				original.Metrics.Receivers = map[string]*MetricsReceiver{}
			}
			for k, v := range overrides.Metrics.Receivers {
				original.Metrics.Receivers[k] = v
			}
		}
		if overrides.Metrics.Processors != nil {
			if original.Metrics.Processors == nil {
				original.Metrics.Processors = map[string]*MetricsProcessor{}
			}
			for k, v := range overrides.Metrics.Processors {
				original.Metrics.Processors[k] = v
			}
		}
		if overrides.Metrics.Exporters != nil {
			if original.Metrics.Exporters == nil {
				original.Metrics.Exporters = map[string]*MetricsExporter{}
			}
			for k, v := range overrides.Metrics.Exporters {
				original.Metrics.Exporters[k] = v
			}
		}
		if overrides.Metrics.Service != nil {
			for name, pipeline := range overrides.Metrics.Service.Pipelines {
				if name == "default_pipeline" {
					// If "receivers/processors/exporters" is not nil, overwrite the built-in instead of merging.
					// This covers 2 cases:
					// 1. "<entity>:[]" is specified explicitly in user config. We want to clear the entity.
					// 2. "<entity>" is a non-empty list. We want to overwrite the built-in instead of merging.
					rIDs := pipeline.ReceiverIDs
					pIDs := pipeline.ProcessorIDs
					eIDs := pipeline.ExporterIDs
					if rIDs != nil {
						original.Metrics.Service.Pipelines["default_pipeline"].ReceiverIDs = rIDs
					}
					if pIDs != nil {
						original.Metrics.Service.Pipelines["default_pipeline"].ProcessorIDs = pIDs
					}
					if eIDs != nil {
						original.Metrics.Service.Pipelines["default_pipeline"].ExporterIDs = eIDs
					}
				} else {
					original.Metrics.Service.Pipelines[name] = pipeline
				}
			}
		}
	}
}
