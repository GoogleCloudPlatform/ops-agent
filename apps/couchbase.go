package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// LoggingReceiverCouchbase is a struct used for generating the fluentbit component for couchbase logs
type LoggingReceiverCouchbase struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the string identifier for the general couchbase logs
func (lr LoggingReceiverCouchbase) Type() string {
	return "couchbase_general"
}

// Components returns the logging components of the couchbase access logs
func (lr LoggingReceiverCouchbase) Components(tag string) []fluentbit.Component {
	if len(lr.IncludePaths) == 0 {
		lr.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/couchdb.log",
			"/opt/couchbase/var/lib/couchbase/logs/info.log",
			"/opt/couchbase/var/lib/couchbase/logs/debug.log",
			"/opt/couchbase/var/lib/couchbase/logs/error.log",
			"/opt/couchbase/var/lib/couchbase/logs/babysitter.log",
		}
	}
	lr.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^\[([^\s+:]*):`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^(?!\[([^\s+:]*):).*$`,
		},
	}
	components := lr.LoggingReceiverFilesMixin.Components(tag)
	components = append(components, confgenerator.LoggingProcessorParseRegex{
		Regex: `^\[(?<type>[^:]*):(?<level>[^,]*),(?<timestamp>\d+-\d+-\d+T\d+:\d+:\d+.\d+Z),(?<node_name>[^:]*):(?<module_name>[^\<]+)(?<source>[^\]]+)\](?<message>.*)$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
		},
	}.Components(tag, "couchbase_default")...)

	components = append(components,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"debug": "DEBUG",
						"info":  "INFO",
						"warn":  "WARNING",
						"error": "ERROR",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(lr.Type()),
			},
		}.Components(tag, lr.Type())...)
	return components
}

// LoggingProcessorCouchbaseHTTPAccess is a struct that will generate the fluentbit components for the http access logs
type LoggingProcessorCouchbaseHTTPAccess struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the string for the couchbase http access logs
func (lp LoggingProcessorCouchbaseHTTPAccess) Type() string {
	return "couchbase_http_access"
}

// Components returns the fluentbit components for the http access logs of couchbase
func (lp LoggingProcessorCouchbaseHTTPAccess) Components(tag string) []fluentbit.Component {
	if len(lp.IncludePaths) == 0 {
		lp.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/http_access.log",
			"/opt/couchbase/var/lib/couchbase/logs/http_access_internal.log",
		}
	}
	c := lp.LoggingReceiverFilesMixin.Components(tag)
	c = append(c,
		confgenerator.LoggingProcessorParseRegex{
			Regex: `^(?<client_ip>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<timestamp>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<status_code>[^ ]*) (?<response_size>[^ ]*) - (?<message>.*)$`,
			ParserShared: confgenerator.ParserShared{
				TimeKey:    "timestamp",
				TimeFormat: `%d/%b/%Y:%H:%M:%S %z`,
				Types: map[string]string{
					"size": "integer",
					"code": "integer",
				},
			},
		}.Components(tag, "couchbase_http_access")...,
	)
	c = append(c, confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			InstrumentationSourceLabel: instrumentationSourceValue(lp.Type()),
		},
	}.Components(tag, lp.Type())...)
	return c
}

// LoggingProcessorCouchbaseGOXDCR is a struct that iwll generate the fluentbit components for the goxdcr logs
type LoggingProcessorCouchbaseGOXDCR struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the type string for the cross datacenter logs of couchbase
func (lg LoggingProcessorCouchbaseGOXDCR) Type() string {
	return "couchbase_xdcr"
}

// Components returns the fluentbit components for the couchbase goxdcr logs
func (lg LoggingProcessorCouchbaseGOXDCR) Components(tag string) []fluentbit.Component {
	if len(lg.IncludePaths) == 0 {
		lg.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/goxdcr.log",
		}
	}

	lg.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^(?!\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})`,
		},
	}
	c := lg.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d*Z) (?<level>\w+) (?<log_type>\w+.\w+): (?<message>.*)$`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
					},
				},
			},
		},
	}.Components(tag, lg.Type())...)
	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"DEBUG": "DEBUG",
						"INFO":  "INFO",
						"WARN":  "WARNING",
						"ERROR": "ERROR",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(lg.Type()),
			},
		}.Components(tag, lg.Type())...,
	)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverCouchbase{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCouchbaseHTTPAccess{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCouchbaseGOXDCR{} })
}
