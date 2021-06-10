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

// Package config represents the Ops Agent configuration.
package config

// Ops Agent metrics config.
type Metrics struct {
	Receivers map[string]*MetricsReceiver `yaml:"receivers"`
	Exporters map[string]*MetricsExporter `yaml:"exporters"`
	Service   *MetricsService             `yaml:"service"`
}

type MetricsReceiver struct {
	Type               string `yaml:"type"`
	CollectionInterval string `yaml:"collection_interval"` // time.Duration format
}

type MetricsExporter struct {
	Type string `yaml:"type"`
}

type MetricsService struct {
	Pipelines map[string]*MetricsPipeline `yaml:"pipelines"`
}

type MetricsPipeline struct {
	ReceiverIDs []string `yaml:"receivers"`
	ExporterIDs []string `yaml:"exporters"`
}
