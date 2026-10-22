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

package confgenerator

import (
	"context"
	"log"

	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

func mergeConfigs(original, overrides *UnifiedConfig) {
	original.Combined = overrides.Combined
	original.Traces = overrides.Traces
	original.Global = overrides.Global

	if overrides.Logging != nil {
		for k, v := range overrides.Logging.Receivers {
			original.Logging.Receivers[k] = v
		}

		original.Logging.Processors = map[string]LoggingProcessor{}
		for k, v := range overrides.Logging.Processors {
			original.Logging.Processors[k] = v
		}
		if overrides.Logging.Service != nil {
			if overrides.Logging.Service.LogLevel != "info" {
				original.Logging.Service.LogLevel = overrides.Logging.Service.LogLevel
			}
			original.Logging.Service.OTelLogging = overrides.Logging.Service.OTelLogging
			if overrides.Logging.Service.Compress != "" {
				original.Logging.Service.Compress = overrides.Logging.Service.Compress
			}
			for name, pipeline := range overrides.Logging.Service.Pipelines {
				pipeline.ExporterIDs = nil
				if name == "default_pipeline" {
					if ids := pipeline.ReceiverIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ReceiverIDs = ids
					}
					if ids := pipeline.ProcessorIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ProcessorIDs = ids
					}
				} else {
					original.Logging.Service.Pipelines[name] = pipeline
				}
			}
		}
	}
	if overrides.Metrics != nil {
		for k, v := range overrides.Metrics.Receivers {
			original.Metrics.Receivers[k] = v
		}
		for k, v := range overrides.Metrics.Processors {
			original.Metrics.Processors[k] = v
		}

		if overrides.Metrics.Service != nil {
			if overrides.Metrics.Service.LogLevel != "info" {
				original.Metrics.Service.LogLevel = overrides.Metrics.Service.LogLevel
			}
			for name, pipeline := range overrides.Metrics.Service.Pipelines {
				pipeline.ExporterIDs = nil
				original.Metrics.Service.Pipelines[name] = pipeline
			}
		}
	}
}

type AgentDefaults struct {
	DisableRubyRegex              bool
	EnableOtlpExporterByDefault   bool
	EnableOpsAgentHealthExtension bool
}

func (uc *UnifiedConfig) ApplyDefaults(defaults AgentDefaults) {
	if uc.Global == nil {
		uc.Global = &Global{}
	}
	if uc.Global.DisableRubyRegex == nil {
		b := defaults.DisableRubyRegex
		uc.Global.DisableRubyRegex = &b
	}
	if uc.Global.OtlpExporter == nil {
		b := defaults.EnableOtlpExporterByDefault
		uc.Global.OtlpExporter = &b
	}
	if uc.Global.EnableOpsAgentHealthExtension == nil {
		b := defaults.EnableOpsAgentHealthExtension
		uc.Global.EnableOpsAgentHealthExtension = &b
	}
}

func BuildUnifiedConfig(ctx context.Context, userConf []byte, defaults ...AgentDefaults) (*UnifiedConfig, error) {
	builtInStruct := BuiltInConfStructs[platform.FromContext(ctx).Name()]

	result, err := builtInStruct.DeepCopy(ctx)
	if err != nil {
		return nil, err
	}

	var overrides *UnifiedConfig
	if len(userConf) > 0 {
		overrides, err = UnmarshalYamlToUnifiedConfig(ctx, userConf)
		if err != nil {
			return nil, err
		}
	}

	if overrides != nil {
		mergeConfigs(result, overrides)
	}

	var defaultOpts AgentDefaults
	if len(defaults) > 0 {
		defaultOpts = defaults[0]
	}
	result.ApplyDefaults(defaultOpts)
	if err := result.Validate(ctx); err != nil {
		return nil, err
	}

	v := newValidator()
	if err := v.StructCtx(ctx, result); err != nil {
		log.Fatalf("merged config failed to validate: %v", err)
	}
	return result, nil
}
