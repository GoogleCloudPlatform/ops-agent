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

//Package otel provides data structures to represent and generate otel configuration.
package otel

import (
	"fmt"
//	"net"
	"strings"
	"text/template"
)

const (
	confTemplate = `receivers:
  {{range .ReceiversConfigSection -}}
  {{.}}
  {{end}}
processors:
  {{range .ProcessorsConfigSection -}}
  {{.}}
  {{end}}
exporters:
  {{range .ExportersConfigSection -}}
  {{.}}
  {{end}}
extensions:
  {{range .ExtensionsConfigSection -}}
  {{.}}
  {{end}}
service:
  {{range .ServiceConfigSection -}}
  {{.}}
  {{end}}`

	hostmetricsConf = `  hostmetrics/{{.HostMetricsID}}:
    collection_interval: {{.CollectionInterval}}
    scrapers:
      cpu:
      load:
      memory:
      disk:
      filesystem:
      network:
      swap:
      process:`

	stackdriverConf = `  stackdriver/{{.StackdriverID}}:
	  user_agent: {{.UserAgent}}
	  metric:
        prefix: {{.Prefix}}`
)

type configSections struct {
	ReceiversConfigSection		[]string
	ProcessorsConfigSection		[]string
	ExportersConfigSection		[]string
	ExtensionsConfigSection		[]string
	ServiceConfigSection		[]string
}

type emptyFieldErr struct {
	plugin string
	field  string
}

func (e emptyFieldErr) Error() string {
	return fmt.Sprintf("%q plugin should not have empty field: %q", e.plugin, e.field)
}

type HostMetrics struct {
	HostMetricsID string
	CollectionInterval string
}

var hostMetricsTemplate = template.Must(template.New("hostmetrics").Parse(hostmetricsConf))

func (h HostMetrics) renderConfig() (string, error) {
	if h.CollectionInterval == "" {
		return "", emptyFieldErr{
			plugin: "hostmetrics",
			field:  "collection_interval",
		}
	}

	var renderedHostMetricsConfig strings.Builder
	if err := hostMetricsTemplate.Execute(&renderedHostMetricsConfig, h); err != nil {
		return "", err
	}
	return renderedHostMetricsConfig.String(), nil
}

type Stackdriver struct {
	UserAgent string
	Prefix string
}

var stackdriverTemplate = template.Must(template.New("stackdriver").Parse(stackdriverConf))

func (s Stackdriver) renderConfig() (string, error) {
	var renderedStackdriverConfig strings.Builder
	if err := stackdriverTemplate.Execute(&renderedStackdriverConfig, s); err != nil {
		return "", err
	}
	return renderedStackdriverConfig.String(), nil
}

func GenerateOtelConfig(hostMetricsList []*HostMetrics, stackdriverList []*Stackdriver) (string, error) {
	receiversConfigSection := []string{}
	exportersConfigSection := []string{}
	for _, h := range hostMetricsList {
		configSection, err := h.renderConfig()
		if err != nil {
			return "", err
		}
		receiversConfigSection = append(receiversConfigSection, configSection)
	}

	for _, s := range stackdriverList {
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		exportersConfigSection = append(exportersConfigSection, configSection)
	}

	configSections := configSections{
		ReceiversConfigSection: receiversConfigSection,
		ExportersConfigSection: exportersConfigSection,
		//ServiceConfigSection: serviceConfigSection,
	}
	conf, err := template.New("otelConf").Parse(confTemplate)
	if err != nil {
		return "", err
	}

	var configBuilder strings.Builder
	if err := conf.Execute(&configBuilder, configSections); err != nil {
		return "", err
	}
	return configBuilder.String(), nil
}
