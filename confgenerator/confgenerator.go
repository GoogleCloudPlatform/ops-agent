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

// Package confgenerator provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"fmt"
	"strings"

	"github.com/Stackdriver/unified_agents/collectd"
	"github.com/Stackdriver/unified_agents/fluentbit/conf"

	yaml "gopkg.in/yaml.v2"
)

type unifiedConfig struct {
	Logging *logging         `yaml:"logging"`
	Metrics collectd.Metrics `yaml:"metrics"`
}

type logging struct {
	Input      []*input     `yaml:"input"`
	Processors []*processor `yaml:"processors"`
	Outputs    []*output    `yaml:"output"`
}

type input struct {
	LogSourceID  string   `yaml:"log_source_id"`
	File         *file    `yaml:"file"`
	Syslog       *syslog  `yaml:"syslog"`
	ProcessorIDs []string `yaml:"processor_ids"`
	OutputIDs    []string `yaml:"output_ids"`
}

type syslog struct {
	Mode   string `yaml:"mode"`
	Listen string `yaml:"listen"`
	Port   uint16 `yaml:"port"`
}

type file struct {
	Paths        []string `yaml:"paths"`
	ExcludePaths []string `yaml:"exclude_paths"`
	ParserID     string   `yaml:"parser_id"`
}

type processor struct {
	ID         string      `yaml:"id"`
	ParseJSON  *parseJSON  `yaml:"parse_json"`
	ParseRegex *parseRegex `yaml:"parse_regex"`
}

type parseJSON struct {
	Field      string `yaml:"field"`
	TimeKey    string `yaml:"time_key"`
	TimeFormat string `yaml:"time_format"`
}

type parseRegex struct {
	Field      string `yaml:"field"`
	Regex      string `yaml:"regex"`
	TimeKey    string `yaml:"time_key"`
	TimeFormat string `yaml:"time_format"`
}

type output struct {
	ID     string  `yaml:"id"`
}

func GenerateCollectdConfig(input []byte, logsDir string) (config string, err error) {
	unifiedConfig, err := unifiedConfigReader(input)
	if err != nil {
		return "", err
	}
	collectdConfig, err := collectd.GenerateCollectdConfig(unifiedConfig.Metrics, logsDir)
	if err != nil {
		return "", err
	}
	return collectdConfig, nil
}

// GenerateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. GenerateFluentBitConfigs returns empty configurations without an error if `logs`
// does not exist as a top-level field in the input yaml format.
func GenerateFluentBitConfigs(input []byte, logsDir string, stateDir string) (mainConfig string, parserConfig string, err error) {
	unifiedConfig, err := unifiedConfigReader(input)
	if err != nil {
		return "", "", err
	}
	if unifiedConfig.Logging == nil {
		return "", "", nil
	}
	if unifiedConfig.Logging.Input == nil {
		return "", "", nil
	}
	return generateFluentBitConfigs(unifiedConfig.Logging.Input, unifiedConfig.Logging.Processors, unifiedConfig.Logging.Outputs, logsDir, stateDir)
}

func unifiedConfigReader(input []byte) (unifiedConfig, error) {
	config := unifiedConfig{}
	err := yaml.UnmarshalStrict(input, &config)
	if err != nil {
		return unifiedConfig{}, err
	}
	return config, nil
}

// defaultTails returns the default Tail sections for the agents' own logs.
func defaultTails(logsDir string, stateDir string) (tails []*conf.Tail) {
	return []*conf.Tail {
		{
			Tag:  "ops-agent-fluent-bit",
			DB:   fmt.Sprintf("%s/buffers/fluent-bit/ops-agent-fluent-bit", stateDir),
			Path: fmt.Sprintf("%s/fluent-bit.log", logsDir),
		},
		{
			Tag:  "ops-agent-collectd",
			DB:   fmt.Sprintf("%s/buffers/fluent-bit/ops-agent-collectd", stateDir),
			Path: fmt.Sprintf("%s/collectd.log", logsDir),
		},
	}
}

func generateFluentBitConfigs(inputs []*input, processors []*processor, outputs []*output, logsDir string, stateDir string) (string, string, error) {
	fbSyslogs, err := extractFluentBitSyslogs(inputs)
	if err != nil {
		return "", "", err
	}
	extractedTails, err := extractFluentBitTails(inputs)
	if err != nil {
		return "", "", err
	}
	fbTails := defaultTails(logsDir, stateDir)
	fbTails = append(fbTails, extractedTails...)
	fbFilterParsers, err := extractFluentBitFilters(inputs, processors)
	if err != nil {
		return "", "", err
	}
	fbStackdrivers, err := extractFluentBitOutputs(inputs, outputs)
	if err != nil {
		return "", "", err
	}
	mainConfig, err := conf.GenerateFluentBitMainConfig(fbTails, fbSyslogs, fbFilterParsers, fbStackdrivers)
	if err != nil {
		return "", "", err
	}
	jsonParsers, regexParsers, err := extractFluentBitParsers(processors)
	if err != nil {
		return "", "", err
	}
	parserConfig, err := conf.GenerateFluentBitParserConfig(jsonParsers, regexParsers)
	if err != nil {
		return "", "", err
	}
	return mainConfig, parserConfig, nil
}

func extractFluentBitSyslogs(inputs []*input) ([]*conf.Syslog, error) {
	fbSyslogs := []*conf.Syslog{}
	for _, i := range inputs {
		fbSyslog, err := extractFluentBitSyslog(*i)
		if err != nil {
			return nil, err
		}
		if fbSyslog == nil {
			continue
		}
		fbSyslogs = append(fbSyslogs, fbSyslog)
	}
	return fbSyslogs, nil
}

func extractFluentBitSyslog(i input) (*conf.Syslog, error) {
	if i.Syslog == nil {
		return nil, nil
	}
	if i.LogSourceID == "" {
		return nil, fmt.Errorf(`syslog cannot have empty log_source_id`)
	}
	fbSyslog := conf.Syslog{
		Tag:    i.LogSourceID,
		Listen: i.Syslog.Listen,
		Port:   i.Syslog.Port,
	}
	switch m := i.Syslog.Mode; m {
	case "tcp", "udp":
		fbSyslog.Mode = m
	case "unix_tcp", "unix_udp":
		// TODO: pending decision on setting up unix_tcp, unix_udp
		fallthrough
	default:
		return nil, fmt.Errorf(`syslog LogSourceID=%q should have the mode as one of the \"tcp\", \"udp\"`, i.LogSourceID)
	}
	return &fbSyslog, nil
}

func extractFluentBitTails(inputs []*input) ([]*conf.Tail, error) {
	fbTails := []*conf.Tail{}
	for _, i := range inputs {
		fbTail, err := extractFluentBitTail(*i)
		if err != nil {
			return nil, err
		}
		if fbTail == nil {
			continue
		}
		fbTails = append(fbTails, fbTail)
	}
	return fbTails, nil
}

func extractFluentBitTail(i input) (*conf.Tail, error) {
	if i.File == nil {
		return nil, nil
	}
	if i.LogSourceID == "" {
		return nil, fmt.Errorf(`file cannot have empty log_source_id`)
	}
	if len(i.File.Paths) == 0 {
		return nil, fmt.Errorf(`file LogSourceID=%q should have at least one path specified`, i.LogSourceID)
	}
	fbTail := conf.Tail{
		Tag:  i.LogSourceID,
		// TODO(ycchou): Pass in directory prefix set by Systemd.
		DB:   fmt.Sprintf("/var/lib/google-cloud-ops-agent/buffers/fluent-bit/%s", i.LogSourceID),
		Path: strings.Join(i.File.Paths, ","),
	}
	if len(i.File.ExcludePaths) != 0 {
		fbTail.ExcludePath = strings.Join(i.File.ExcludePaths, ",")
	}
	return &fbTail, nil
}

func extractFluentBitFilters(inputs []*input, processors []*processor) ([]*conf.FilterParser, error) {
	fbFilterParsers := []*conf.FilterParser{}
	for _, i := range inputs {
		if i.LogSourceID == "" {
			return nil, fmt.Errorf(`input cannot have empty log_source_id`)
		}
		for _, inputProcessorID := range i.ProcessorIDs {
			fbFilterParser := conf.FilterParser{
				Match:  i.LogSourceID,
				Parser: inputProcessorID,
			}
			switch inputProcessorID {
			case "apache", "apache2", "apache_error", "mongodb", "nginx", "syslog-rfc3164", "syslog-rfc5424":
				fbFilterParser.KeyName = "message"
			}
			for _, p := range processors {
				if inputProcessorID != p.ID {
					continue
				}
				if p.ParseJSON != nil {
					if p.ParseJSON.Field == "" {
						fbFilterParser.KeyName = "message"
					}
					fbFilterParser.KeyName = p.ParseJSON.Field
					break
				}
				if p.ParseRegex != nil {
					if p.ParseRegex.Field == "" {
						fbFilterParser.KeyName = "message"
					}
					fbFilterParser.KeyName = p.ParseRegex.Field
					break
				}
			}
			fbFilterParsers = append(fbFilterParsers, &fbFilterParser)
		}
	}
	return fbFilterParsers, nil
}

func extractFluentBitOutputs(inputs []*input, outputs []*output) ([]*conf.Stackdriver, error) {
	fbStackdrivers := []*conf.Stackdriver{}
	for _, i := range inputs {
		if i.LogSourceID == "" {
			return nil, fmt.Errorf(`input cannot have empty log_source_id`)
		}
		for _, outputID := range i.OutputIDs {
			// Process special output ID "google"
			if outputID != "google" {
				return nil, fmt.Errorf(`output ID can only be "google" now.`)
			}
			fbStackdrivers = append(fbStackdrivers, &conf.Stackdriver{
				Match: i.LogSourceID,
			})
		}
	}
	return fbStackdrivers, nil
}

func extractFluentBitParsers(processors []*processor) ([]*conf.ParserJSON, []*conf.ParserRegex, error) {
	fbJSONParsers := []*conf.ParserJSON{}
	fbRegexParsers := []*conf.ParserRegex{}
	for _, p := range processors {
		err := validateProcessor(*p)
		if err != nil {
			return nil, nil, err
		}
		if p.ParseJSON != nil {
			fbJSONParser := conf.ParserJSON{
				Name:       p.ID,
				TimeKey:    p.ParseJSON.TimeKey,
				TimeFormat: p.ParseJSON.TimeFormat,
			}
			fbJSONParsers = append(fbJSONParsers, &fbJSONParser)
		}
		if p.ParseRegex != nil {
			fbRegexParser := conf.ParserRegex{
				Name:       p.ID,
				Regex:      p.ParseRegex.Regex,
				TimeKey:    p.ParseRegex.TimeKey,
				TimeFormat: p.ParseRegex.TimeFormat,
			}
			fbRegexParsers = append(fbRegexParsers, &fbRegexParser)
		}
	}
	return fbJSONParsers, fbRegexParsers, nil
}

func validateProcessor(p processor) error {
	if p.ID == "" {
		return fmt.Errorf(`processor cannot have empty id`)
	}
	typeCount := 0
	if p.ParseJSON != nil {
		typeCount += 1
	}
	if p.ParseRegex != nil {
		typeCount += 1
	}
	if typeCount == 0 {
		return fmt.Errorf(`processor ID=%q should have one of the fields \"parse_json\", \"parse_regex\"`, p.ID)
	}
	if typeCount > 1 {
		return fmt.Errorf(`processor ID=%q should have only one of the fields \"parse_json\", \"parse_regex\"`, p.ID)
	}
	return nil
}
