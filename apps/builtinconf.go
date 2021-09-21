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

import cg "github.com/GoogleCloudPlatform/ops-agent/confgenerator"

var (
	BuiltInConfStructs = map[string]*cg.UnifiedConfig{
		"linux": &cg.UnifiedConfig{
			Logging: &cg.Logging{
				Receivers: map[string]cg.LoggingReceiver{
					"syslog": &cg.LoggingReceiverFiles{
						ConfigComponent: cg.ConfigComponent{Type: "files"},
						IncludePaths:    []string{"/var/log/messages", "/var/log/syslog"},
					},
				},
				Processors: map[string]cg.LoggingProcessor{},
				Service: &cg.LoggingService{
					Pipelines: map[string]*cg.LoggingPipeline{
						"default_pipeline": &cg.LoggingPipeline{
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
					Pipelines: map[string]*cg.MetricsPipeline{
						"default_pipeline": &cg.MetricsPipeline{
							ReceiverIDs:  []string{"hostmetrics"},
							ProcessorIDs: []string{"metrics_filter"},
						},
					},
				},
			},
		},
		"windows": &cg.UnifiedConfig{
			Logging: &cg.Logging{
				Receivers: map[string]cg.LoggingReceiver{
					"windows_event_log": &cg.LoggingReceiverWindowsEventLog{
						ConfigComponent: cg.ConfigComponent{Type: "windows_event_log"},
						Channels:        []string{"System", "Application", "Security"},
					},
				},
				Processors: map[string]cg.LoggingProcessor{},
				Service: &cg.LoggingService{
					Pipelines: map[string]*cg.LoggingPipeline{
						"default_pipeline": &cg.LoggingPipeline{
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
					Pipelines: map[string]*cg.MetricsPipeline{
						"default_pipeline": &cg.MetricsPipeline{
							ReceiverIDs:  []string{"hostmetrics", "iis", "mssql"},
							ProcessorIDs: []string{"metrics_filter"},
						},
					},
				},
			},
		},
	}
)
