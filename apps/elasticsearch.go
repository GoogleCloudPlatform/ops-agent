package apps

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingProcessorElasticsearchJson struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorElasticsearchJson) Type() string {
	return "elasticsearch_json"
}

func (p LoggingProcessorElasticsearchJson) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	// When Elasticsearch emits stack traces, the json log may be spread across multiple lines,
	// this parser handles that case.
	multilineParser := &confgenerator.LoggingProcessorParseMultiline{
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `^{.*`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^[^{].*[,}]$`,
			},
		},
	}

	jsonParser := &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%dT%H:%M:%S,%L%z",
		},
	}

	c = append(c, multilineParser.Components(tag, uid)...)
	c = append(c, jsonParser.Components(tag, uid)...)
	c = append(c, p.severityParser(tag, uid)...)
	c = append(c, p.nestingProcessors(tag, uid)...)

	return c
}

func (p LoggingProcessorElasticsearchJson) severityParser(tag, uid string) []fluentbit.Component {
	severityKey := "logging.googleapis.com/severity"
	return fluentbit.TranslationMoveComponents(tag, "level", severityKey, []struct {
		SrcVal  string
		DestVal string
	}{
		{
			SrcVal:  "TRACE",
			DestVal: "DEBUG",
		},
		{
			SrcVal:  "DEBUG",
			DestVal: "DEBUG",
		},
		{
			SrcVal:  "INFO",
			DestVal: "INFO",
		},
		{
			SrcVal:  "WARN",
			DestVal: "WARNING",
		},
		{
			SrcVal:  "DEPRECATION",
			DestVal: "WARNING",
		},
		{
			SrcVal:  "ERROR",
			DestVal: "ERROR",
		},
		{
			SrcVal:  "CRITICAL",
			DestVal: "ERROR",
		},
		{
			SrcVal:  "FATAL",
			DestVal: "FATAL",
		},
	})
}

func (p LoggingProcessorElasticsearchJson) nestingProcessors(tag, uid string) []fluentbit.Component {
	// The majority of these prefixes come from here:
	// https://www.elastic.co/guide/en/elasticsearch/reference/7.16/audit-event-types.html#audit-event-attributes
	// Non-audit logs are formatted using the layout documented here, giving the "cluster" prefix:
	// https://artifacts.elastic.co/javadoc/org/elasticsearch/elasticsearch/7.16.2/org/elasticsearch/common/logging/ESJsonLayout.html
	prefixes := []string{
		"user.run_by",
		"user.run_as",
		"authentication.token",
		"node",
		"event",
		"authentication",
		"user",
		"origin",
		"request",
		"url",
		"host",
		"apikey",
		"cluster",
	}

	c := make([]fluentbit.Component, 0, len(prefixes))
	for _, prefix := range prefixes {
		nestProcessor := confgenerator.LoggingProcessorNestWildcard{
			Wildcard:     fmt.Sprintf("%s.*", prefix),
			NestUnder:    prefix,
			RemovePrefix: fmt.Sprintf("%s.", prefix),
		}
		c = append(c, nestProcessor.Components(tag, uid)...)
	}

	return c
}

type LoggingProcessorElasticsearchGC struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorElasticsearchGC) Type() string {
	return "elasticsearch_gc"
}

func (p LoggingProcessorElasticsearchGC) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	regexParser := confgenerator.LoggingProcessorParseRegex{
		Regex: `\[(?<time>\d+-\d+-\d+T\d+:\d+:\d+.\d+\+\d+)\]\[\d+\]\[(?<type>[A-z,]+)\s*\]\s*(?:GC\((?<gc_run>\d+)\))?\s*(?<message>.*)`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
			Types: map[string]string{
				"gc_run": "integer",
			},
		},
	}

	c = append(c, regexParser.Components(tag, uid)...)

	return c
}

type LoggingReceiverElasticsearchJson struct {
	LoggingProcessorElasticsearchJson       `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline"`
}

func (r LoggingReceiverElasticsearchJson) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		// Default JSON logs for Elasticsearch
		r.IncludePaths = []string{
			"/var/log/elasticsearch/*_server.json",
			"/var/log/elasticsearch/*_deprecation.json",
			"/var/log/elasticsearch/*_index_search_slowlog.json",
			"/var/log/elasticsearch/*_index_indexing_slowlog.json",
			"/var/log/elasticsearch/*_audit.json",
		}
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	return append(c, r.LoggingProcessorElasticsearchJson.Components(tag, "elasticsearch_json")...)
}

type LoggingReceiverElasticsearchGC struct {
	LoggingProcessorElasticsearchGC         `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline"`
}

func (r LoggingReceiverElasticsearchGC) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		// Default GC log for Elasticsearch
		r.IncludePaths = []string{
			"/var/log/elasticsearch/gc.log",
		}
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	return append(c, r.LoggingProcessorElasticsearchGC.Components(tag, "elasticsearch_gc")...)
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverElasticsearchJson{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverElasticsearchGC{} })
}
