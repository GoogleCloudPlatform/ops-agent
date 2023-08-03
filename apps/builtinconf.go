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

package apps

import (
	"context"
	"fmt"

	cg "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

var (
	builtInConfStructs = map[string]*cg.UnifiedConfig{
		"linux": {
			Logging: &cg.Logging{
				Receivers: map[string]cg.LoggingReceiver{
					"syslog": &cg.LoggingReceiverFiles{
						ConfigComponent: cg.ConfigComponent{Type: "files"},
						IncludePaths:    []string{"/var/log/messages", "/var/log/syslog"},
					},
				},
				Processors: map[string]cg.LoggingProcessor{},
				Service: &cg.LoggingService{
					Pipelines: map[string]*cg.Pipeline{
						"default_pipeline": {
							ReceiverIDs: []string{"syslog"},
						},
					},
				},
			},
			Metrics: &cg.Metrics{
				Receivers: map[string]cg.MetricsReceiver{
					"hostmetrics": &MetricsReceiverHostmetrics{
						ConfigComponent:       cg.ConfigComponent{Type: "hostmetrics"},
						MetricsReceiverShared: cg.MetricsReceiverShared{CollectionInterval: "60s"},
					},
				},
				Processors: map[string]cg.MetricsProcessor{
					"metrics_filter": &MetricsProcessorExcludeMetrics{
						ConfigComponent: cg.ConfigComponent{Type: "exclude_metrics"},
					},
				},
				Service: &cg.MetricsService{
					Pipelines: map[string]*cg.Pipeline{
						"default_pipeline": {
							ReceiverIDs:  []string{"hostmetrics"},
							ProcessorIDs: []string{"metrics_filter"},
						},
					},
				},
			},
		},
		"linux_gpu": {
			Logging: &cg.Logging{
				Receivers: map[string]cg.LoggingReceiver{
					"syslog": &cg.LoggingReceiverFiles{
						ConfigComponent: cg.ConfigComponent{Type: "files"},
						IncludePaths:    []string{"/var/log/messages", "/var/log/syslog"},
					},
				},
				Processors: map[string]cg.LoggingProcessor{},
				Service: &cg.LoggingService{
					Pipelines: map[string]*cg.Pipeline{
						"default_pipeline": {
							ReceiverIDs: []string{"syslog"},
						},
					},
				},
			},
			Metrics: &cg.Metrics{
				Receivers: map[string]cg.MetricsReceiver{
					"hostmetrics": &MetricsReceiverHostmetrics{
						ConfigComponent:       cg.ConfigComponent{Type: "hostmetrics"},
						MetricsReceiverShared: cg.MetricsReceiverShared{CollectionInterval: "60s"},
					},
					"nvidia_gpu": &MetricsReceiverNvml{
						ConfigComponent:       cg.ConfigComponent{Type: "nvml"},
						MetricsReceiverShared: cg.MetricsReceiverShared{CollectionInterval: "60s"},
					},
				},
				Processors: map[string]cg.MetricsProcessor{
					"metrics_filter": &MetricsProcessorExcludeMetrics{
						ConfigComponent: cg.ConfigComponent{Type: "exclude_metrics"},
					},
				},
				Service: &cg.MetricsService{
					Pipelines: map[string]*cg.Pipeline{
						"default_pipeline": {
							ReceiverIDs:  []string{"hostmetrics", "nvidia_gpu"},
							ProcessorIDs: []string{"metrics_filter"},
						},
					},
				},
			},
		},
		"windows": {
			Logging: &cg.Logging{
				Receivers: map[string]cg.LoggingReceiver{
					"windows_event_log": &cg.LoggingReceiverWindowsEventLog{
						ConfigComponent: cg.ConfigComponent{Type: "windows_event_log"},
						Channels:        []string{"System", "Application", "Security"},
					},
				},
				Processors: map[string]cg.LoggingProcessor{},
				Service: &cg.LoggingService{
					Pipelines: map[string]*cg.Pipeline{
						"default_pipeline": {
							ReceiverIDs: []string{"windows_event_log"},
						},
					},
				},
			},
			Metrics: &cg.Metrics{
				Receivers: map[string]cg.MetricsReceiver{
					"hostmetrics": &MetricsReceiverHostmetrics{
						ConfigComponent:       cg.ConfigComponent{Type: "hostmetrics"},
						MetricsReceiverShared: cg.MetricsReceiverShared{CollectionInterval: "60s"},
					},
					"iis": &MetricsReceiverIis{
						ConfigComponent:       cg.ConfigComponent{Type: "iis"},
						MetricsReceiverShared: cg.MetricsReceiverShared{CollectionInterval: "60s"},
					},
					"mssql": &MetricsReceiverMssql{
						ConfigComponent:       cg.ConfigComponent{Type: "mssql"},
						MetricsReceiverShared: cg.MetricsReceiverShared{CollectionInterval: "60s"},
					},
				},
				Processors: map[string]cg.MetricsProcessor{
					"metrics_filter": &MetricsProcessorExcludeMetrics{
						ConfigComponent: cg.ConfigComponent{Type: "exclude_metrics"},
					},
				},
				Service: &cg.MetricsService{
					Pipelines: map[string]*cg.Pipeline{
						"default_pipeline": {
							ReceiverIDs:  []string{"hostmetrics", "iis", "mssql"},
							ProcessorIDs: []string{"metrics_filter"},
						},
					},
				},
			},
		},
	}
)

// BuiltInConf returns the correct built-in config struct for the given platform
// from the ctx
func BuiltInConf(ctx context.Context) *cg.UnifiedConfig {
	p := platform.FromContext(ctx)
	configKey := p.Name()
	if p.HasNvidiaGpu {
		configKey = fmt.Sprintf("%s_gpu", configKey)
	}
	return builtInConfStructs[configKey]
}
