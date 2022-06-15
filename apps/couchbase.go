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
			Regex:     `^(?!\[([^\s+:]*):).*`,
		},
	}
	components := lr.LoggingReceiverFilesMixin.Components(tag)
	components = append(components, confgenerator.LoggingProcessorParseRegex{
		Regex: `^\[(?<type>[^:]*):(?<severity>[^,]*),(?<timestamp>\d+-\d+-\d+T\d+:\d+:\d+.\d+Z),(?<node>[^@]*)@(?<host>[^:]*):(?<source>[^\]]+)\](?<message>.*)$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
		},
	}.Components(tag, "couchbase_default")...)

	components = append(components,
		fluentbit.TranslationComponents(tag, "severity", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{
					"debug", "DEBUG",
				},
				{
					"info", "INFO",
				},
				{
					"warn", "WARNING",
				},
				{
					"error", "ERROR",
				},
			},
		)...,
	)
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
			Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<timestamp>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*) - (?<client>.*)$`,
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
					Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d*Z) (?<severity>\w+) (?<log_type>\w+.\w+): (?<message>.*)$`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
					},
				},
			},
		},
	}.Components(tag, "couchbase_xdcr")...)
	c = append(c,
		// rename the severities to logging.googleapis.com/severity
		fluentbit.TranslationComponents(tag, "severity", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{
					"DEBUG", "DEBUG",
				},
				{
					"INFO", "INFO",
				},
				{
					"WARN", "WARNING",
				},
				{
					"ERROR", "ERROR",
				},
			},
		)...,
	)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverCouchbase{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCouchbaseHTTPAccess{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCouchbaseGOXDCR{} })
}
