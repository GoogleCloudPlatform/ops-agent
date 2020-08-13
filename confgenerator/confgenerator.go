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

	"github.com/Stackdriver/unified_agents/fluentbit/conf"

	yaml "gopkg.in/yaml.v2"
)

type unifiedConfig struct {
	Logs       *logs       `yaml:"logs"`
	LogsModule *logsModule `yaml:"logs_module"`
}

type logs struct {
	Syslogs []*syslog `yaml:"syslogs"`
	Files   []*file   `yaml:"files"`
}

type syslog struct {
	Mode        string `yaml:"mode"`
	Listen      string `yaml:"listen"`
	Port        uint16 `yaml:"port"`
	LogSourceID string `yaml:"log_source_id"`
	LogName     string `yaml:"log_name"`
	Parser      string `yaml:"parser"`
}

type file struct {
	Paths        []string `yaml:"paths"`
	LogSourceID  string   `yaml:"log_source_id"`
	LogName      string   `yaml:"log_name"`
	ExcludePaths []string `yaml:"exclude_paths"`
	ParserID     string   `yaml:"parser_id"`
}

type logsModule struct {
	Enable  bool      `yaml:"enable"`
	Sources []*source `yaml:"sources"`
}

type source struct {
	Name             string            `yaml:"name"`
	Type             string            `yaml:"type"`
	FileSourceConfig *fileSourceConfig `yaml:"file_source_config"`
}

type fileSourceConfig struct {
	Path            string   `yaml:"path"`
	CheckpointName  string   `yaml:"checkpoint_name"`
	ExcludePath     []string `yaml:"exclude_path"`
	Parser          *parser  `yaml:"parser"`
	RefreshInterval uint64   `yaml:"refresh_interval"` // in seconds
	RotateWait      uint64   `yaml:"rotate_wait"`      // in seconds
	PathFieldName   string   `yaml:"path_field_name"`
}

type parser struct {
	Type              string             `yaml:"type"`
	RegexParserConfig *regexParserConfig `yaml:"regex_parser_config"`
	TimeKey           string             `yaml:"time_key"`
	TimeFormat        string             `yaml:"time_format"`
}

type regexParserConfig struct {
	Expression string `yaml:"expression"`
}

// GenerateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. GenerateFluentBitConfigs returns empty configurations without an error if `logs`
// does not exist as a top-level field in the input yaml format.
func GenerateFluentBitConfigs(input []byte) (mainConfig string, parserConfig string, err error) {
	unifiedConfig, err := unifiedConfigReader(input)
	if err != nil {
		return "", "", err
	}
	if unifiedConfig.Logs == nil {
		return "", "", nil
	}
	return generateFluentBitConfigs(unifiedConfig.Logs.Syslogs, unifiedConfig.Logs.Files)
}

func unifiedConfigReader(input []byte) (unifiedConfig, error) {
	config := unifiedConfig{}
	err := yaml.Unmarshal(input, &config)
	if err != nil {
		return unifiedConfig{}, err
	}
	return config, nil
}

func generateFluentBitConfigs(syslogs []*syslog, files []*file) (string, string, error) {
	fbSyslogs, err := extractFluentBitSyslogs(syslogs)
	if err != nil {
		return "", "", err
	}
	fbTails, err := extractFluentBitTails(files)
	if err != nil {
		return "", "", err
	}
	mainConfig, err := conf.GenerateFluentBitMainConfig(fbTails, fbSyslogs)
	if err != nil {
		return "", "", err
	}
	// TODO: Implement the parser part when the parser design is finalized.
	// For now, we don't generate json/regex parsers into parserConfig.
	jsonParsers := []*conf.ParserJSON{}
	regexParsers := []*conf.ParserRegex{}
	parserConfig, err := conf.GenerateFluentBitParserConfig(jsonParsers, regexParsers)
	if err != nil {
		return "", "", err
	}
	return mainConfig, parserConfig, nil
}

func extractFluentBitSyslogs(syslogs []*syslog) ([]*conf.Syslog, error) {
	fbSyslogs := []*conf.Syslog{}
	for _, s := range syslogs {
		fbSyslog, err := extractFluentBitSyslog(*s)
		if err != nil {
			return nil, err
		}
		fbSyslogs = append(fbSyslogs, fbSyslog)
	}
	return fbSyslogs, nil
}

func extractFluentBitSyslog(s syslog) (*conf.Syslog, error) {
	if s.LogSourceID == "" {
		return nil, fmt.Errorf(`syslog cannot have empty log_source_id`)
	}
	fbTail := conf.Syslog{
		Tag:    s.LogSourceID,
		Listen: s.Listen,
		Port:   s.Port,
	}
	switch m := s.Mode; m {
	case "tcp", "udp":
		fbTail.Mode = m
	case "unix_tcp", "unix_udp":
		// TODO: pending decision on setting up unix_tcp, unix_udp
		fallthrough
	default:
		return nil, fmt.Errorf(`syslog LogSourceID=%q should have the mode as one of the \"tcp\", \"udp\"`, s.LogSourceID)
	}
	switch p := s.Parser; p {
	case "syslog-rfc5424", "syslog-rfc3164":
		fbTail.Parser = p
	default:
		return nil, fmt.Errorf(`Syslog LogSourceID=%q should have the parser as one of the \"syslog-rfc5424\", \"syslog-rfc3164\"`, s.LogSourceID)
	}
	if s.LogName != "" {
		fbTail.Tag = s.LogName
	}
	return &fbTail, nil
}

func extractFluentBitTails(files []*file) ([]*conf.Tail, error) {
	fbTails := []*conf.Tail{}
	for _, s := range files {
		f, err := extractFluentBitTail(*s)
		if err != nil {
			return nil, err
		}
		fbTails = append(fbTails, f)
	}
	return fbTails, nil
}

func extractFluentBitTail(f file) (*conf.Tail, error) {
	if f.LogSourceID == "" {
		return nil, fmt.Errorf(`file cannot have empty log_source_id`)
	}
	if len(f.Paths) == 0 {
		return nil, fmt.Errorf(`file LogSourceID=%q should have the at least one paths specified`, f.LogSourceID)
	}
	fbTail := conf.Tail{
		Tag:  f.LogSourceID,
		DB:   f.LogSourceID,
		Path: strings.Join(f.Paths, ","),
	}

	if len(f.ExcludePaths) != 0 {
		fbTail.ExcludePath = strings.Join(f.ExcludePaths, ",")
	}
	if f.LogName != "" {
		fbTail.Tag = f.LogName
	}
	if f.ParserID != "" {
		fbTail.Parser = f.ParserID
	}
	return &fbTail, nil
}
