// Package confgenerator provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"errors"
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
}

type syslog struct {
	Mode        string `yaml:"mode"`
	Listen      string `yaml:"listen"`
	Port        uint16 `yaml:"port"`
	LogSourceID string `yaml:"log_source_id"`
	LogName     string `yaml:"log_name"`
	Parser      string `yaml:"parser"`
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
	if unifiedConfig.LogsModule == nil {
		return "", "", errors.New("logsModule does not exist")
	}
	return generateFluentBitConfigs(unifiedConfig.Logs.Syslogs, unifiedConfig.LogsModule.Sources)
}

func unifiedConfigReader(input []byte) (unifiedConfig, error) {
	config := unifiedConfig{}
	err := yaml.Unmarshal(input, &config)
	if err != nil {
		return unifiedConfig{}, err
	}
	return config, nil
}

func generateFluentBitConfigs(syslogs []*syslog, sources []*source) (string, string, error) {
	fbSyslogs, err := extractFluentBitSyslogs(syslogs)
	if err != nil {
		return "", "", err
	}
	tails, regexParsers, jsonParsers, err := mapFluentBitConfig(sources)
	if err != nil {
		return "", "", err
	}
	mainConfig, err := conf.GenerateFluentBitMainConfig(tails, fbSyslogs)
	if err != nil {
		return "", "", err
	}
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

func mapFluentBitConfig(sources []*source) ([]*conf.Tail, []*conf.ParserRegex, []*conf.ParserJSON, error) {
	tails, err := extractFluentBitTails(sources)
	if err != nil {
		return nil, nil, nil, err
	}
	regexParsers := extractFluentBitRegexParsers(sources)
	jsonParsers := extractFluentBitJSONParsers(sources)
	return tails, regexParsers, jsonParsers, nil
}

func extractFluentBitTails(sources []*source) ([]*conf.Tail, error) {
	fbTails := []*conf.Tail{}
	for _, s := range sources {
		f, err := extractFluentBitTail(*s)
		if err != nil {
			return nil, err
		}
		fbTails = append(fbTails, f)
	}
	return fbTails, nil
}

func parserName(sourceType string, sourceName string, parserType string) string {
	return fmt.Sprintf("%s_%s_%s", sourceType, sourceName, parserType)
}

func extractFluentBitTail(s source) (*conf.Tail, error) {
	if s.Type != "file" {
		return nil, fmt.Errorf("source type %q is not allowed", s.Type)
	}
	c := s.FileSourceConfig
	if c == nil {
		return nil, fmt.Errorf("file type source %q should have file_source_config", s.Name)
	}
	rotateWait := uint64(5)
	if c.RotateWait != 0 {
		rotateWait = c.RotateWait
	}
	refreshInterval := uint64(60)
	if c.RefreshInterval != 0 {
		refreshInterval = c.RefreshInterval
	}
	fbTail := conf.Tail{
		Tag:             s.Name,
		Path:            c.Path,
		DB:              c.CheckpointName,
		ExcludePath:     strings.Join(c.ExcludePath, ","),
		RotateWait:      rotateWait,
		RefreshInterval: refreshInterval,
		PathKey:         c.PathFieldName,
	}
	if c.Parser != nil {
		var parser string
		switch p := c.Parser.Type; p {
		case "json", "regex":
			parser = parserName(s.Type, s.Name, p)
		case "":
			// no parser is specified, leave the parser as empty string.
		default:
			return nil, fmt.Errorf("parser type %q is not allowed", p)
		}
		fbTail.Parser = parser
	}
	return &fbTail, nil
}

func extractFluentBitRegexParsers(sources []*source) []*conf.ParserRegex {
	fbRegexParsers := []*conf.ParserRegex{}
	for _, s := range sources {
		if parser, ok := extractFluentBitRegexParser(*s); ok {
			fbRegexParsers = append(fbRegexParsers, parser)
		}
	}
	return fbRegexParsers
}

func extractFluentBitRegexParser(s source) (*conf.ParserRegex, bool) {
	c := s.FileSourceConfig
	if c == nil {
		return nil, false
	}
	p := c.Parser
	if p == nil {
		return nil, false
	}
	if s.Type != "file" || p.Type != "regex" || p.RegexParserConfig == nil {
		return nil, false
	}
	parser := conf.ParserRegex{
		Name:       parserName(s.Type, s.Name, p.Type),
		Regex:      p.RegexParserConfig.Expression,
		TimeKey:    p.TimeKey,
		TimeFormat: p.TimeFormat,
	}
	return &parser, true
}

func extractFluentBitJSONParsers(sources []*source) []*conf.ParserJSON {
	fbJSONParsers := []*conf.ParserJSON{}
	for _, s := range sources {
		if parser, ok := extractFluentBitJSONParser(*s); ok {
			fbJSONParsers = append(fbJSONParsers, parser)
		}
	}
	return fbJSONParsers
}

func extractFluentBitJSONParser(s source) (*conf.ParserJSON, bool) {
	c := s.FileSourceConfig
	if c == nil {
		return nil, false
	}
	p := c.Parser
	if p == nil {
		return nil, false
	}
	if s.Type != "file" || p.Type != "json" {
		return nil, false
	}

	parser := conf.ParserJSON{
		Name:       parserName(s.Type, s.Name, p.Type),
		TimeKey:    p.TimeKey,
		TimeFormat: p.TimeFormat,
	}
	return &parser, true
}
