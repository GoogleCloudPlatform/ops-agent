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

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// Ops Agent config.
type UnifiedConfig struct {
	Logging *Logging `yaml:"logging"`
	Metrics *Metrics `yaml:"metrics"`
}

func (uc *UnifiedConfig) HasLogging() bool {
	return uc.Logging != nil
}

func (uc *UnifiedConfig) HasMetrics() bool {
	return uc.Metrics != nil
}

func ParseUnifiedConfig(input []byte) (UnifiedConfig, error) {
	config := UnifiedConfig{}
	err := yaml.UnmarshalStrict(input, &config)
	if err != nil {
		return UnifiedConfig{}, fmt.Errorf("the agent config file is not valid YAML. detailed error: %s", err)
	}
	return config, nil
}

type configComponent struct {
	Type string `yaml:"type"`
}

// Ops Agent logging config.
type Logging struct {
	Receivers  map[string]*LoggingReceiver  `yaml:"receivers"`
	Processors map[string]*LoggingProcessor `yaml:"processors"`
	Exporters  map[string]*LoggingExporter  `yaml:"exporters"`
	Service    *LoggingService              `yaml:"service"`
}

type LoggingReceiverFiles struct {
	IncludePaths []string `yaml:"include_paths"`
	ExcludePaths []string `yaml:"exclude_paths"` // optional
}

type LoggingReceiverSyslog struct {
	TransportProtocol string `yaml:"transport_protocol"` // one of "tcp" or "udp"
	ListenHost        string `yaml:"listen_host"`
	ListenPort        uint16 `yaml:"listen_port"`
}

type LoggingReceiverWinevtlog struct {
	Channels []string `yaml:"channels"`
}

type LoggingReceiver struct {
	configComponent `yaml:",inline"`

	LoggingReceiverFiles     `yaml:",inline"` // Type "files"
	LoggingReceiverSyslog    `yaml:",inline"` // Type "syslog"
	LoggingReceiverWinevtlog `yaml:",inline"` // Type "windows_event_log"
}

type LoggingProcessorParseJson struct {
	Field      string `yaml:"field"`       // optional, default to "message"
	TimeKey    string `yaml:"time_key"`    // optional, by default does not parse timestamp
	TimeFormat string `yaml:"time_format"` // optional, must be provided if time_key is present
}

type LoggingProcessorParseRegex struct {
	Regex string `yaml:"regex"`

	LoggingProcessorParseJson `yaml:",inline"` // Type "parse_json"
}

type LoggingProcessor struct {
	configComponent `yaml:",inline"`

	LoggingProcessorParseRegex `yaml:",inline"` // Type "parse_json" or "parse_regex"
}

type LoggingExporter struct {
	configComponent `yaml:",inline"`
}

type LoggingService struct {
	Pipelines map[string]*LoggingPipeline
}

type LoggingPipeline struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}

// Ops Agent metrics config.
type Metrics struct {
	Receivers map[string]*MetricsReceiver `yaml:"receivers"`
	Exporters map[string]*MetricsExporter `yaml:"exporters"`
	Service   *MetricsService             `yaml:"service"`
}

type MetricsReceiver struct {
	configComponent `yaml:",inline"`

	CollectionInterval string `yaml:"collection_interval"` // time.Duration format
}

type MetricsExporter struct {
	configComponent `yaml:",inline"`
}

type MetricsService struct {
	Pipelines map[string]*MetricsPipeline `yaml:"pipelines"`
}

type MetricsPipeline struct {
	ReceiverIDs []string `yaml:"receivers"`
	ExporterIDs []string `yaml:"exporters"`
}

func ValidateCollectionInterval(receiverID string, collectionInterval string) (float64, error) {
	t, err := time.ParseDuration(collectionInterval)
	if err != nil {
		return math.NaN(), fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %q that is not an interval (e.g. \"60s\"). Detailed error: %s", receiverID, collectionInterval, err)
	}
	interval := t.Seconds()
	if interval < 10 {
		return math.NaN(), fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value \"%vs\" that is below the minimum threshold of \"10s\".", receiverID, interval)
	}
	return interval, nil
}

// mapKeys returns keys from a map[string]Any as a map[string]interface{}.
func mapKeys(m interface{}) map[string]interface{} {
	keys := map[string]interface{}{}
	for iter := reflect.ValueOf(m).MapRange(); iter.Next(); {
		k := iter.Key()
		if k.Kind() != reflect.String {
			panic(fmt.Sprintf("key %v not a string", k))
		}
		keys[k.String()] = nil
	}
	return keys
}

// SortedKeys returns keys from a map[string]Any as a sorted string slice.
func SortedKeys(m interface{}) []string {
	var r []string
	for k := range mapKeys(m) {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

func ValidateComponentIds(components interface{}, subagent string, component string) error {
	for _, id := range SortedKeys(components) {
		if strings.HasPrefix(id, "lib:") {
			return reservedIdPrefixError(subagent, component, id)
		}
	}
	return nil
}

// reservedIdPrefixError returns an error message when users specify a id that starts with "lib:" which is reserved.
// id is the id of the pipeline, receiver, processor, or exporter.
func reservedIdPrefixError(subagent string, component string, id string) error {
	// e.g. logging receiver id "lib:abc" is not allowed because prefix 'lib:' is reserved for pre-defined receivers.
	return fmt.Errorf(`%s %s id %q is not allowed because prefix 'lib:' is reserved for pre-defined %ss.`,
		subagent, component, id, component)
}
