package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingProcessorVaultJson struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorVaultJson) Type() string {
	return "vault_audit"
}

func (p LoggingProcessorVaultJson) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	// sample log line:
	// {"time":"2022-06-07T20:34:34.392078404Z","type":"request","auth":{"token_type":"default"},"request":{"id":"aa005196-0280-381d-ebeb-1a083bdf5675","operation":"update","namespace":{"id":"root"},"path":"sys/audit/test"}}
	jsonParser := &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
		},
	}

	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		}.Components(tag, uid)...,
	)
	c = append(c, jsonParser.Components(tag, uid)...)
	return c
}

type LoggingReceiverVaultAuditJson struct {
	LoggingProcessorVaultJson               `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline"`
	IncludePaths                            []string `yaml:"include_paths,omitempty" validate:"required"`
}

func (r LoggingReceiverVaultAuditJson) Components(tag string) []fluentbit.Component {
	r.LoggingReceiverFilesMixin.IncludePaths = r.IncludePaths

	r.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^{.*`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^(?!{.*)`,
		},
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	return append(c, r.LoggingProcessorVaultJson.Components(tag, r.LoggingProcessorVaultJson.Type())...)
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverVaultAuditJson{} })
}
