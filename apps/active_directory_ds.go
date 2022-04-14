package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingReceiverActiveDirectoryDS struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (r LoggingReceiverActiveDirectoryDS) Type() string {
	return "active_directory_ds"
}

func (r LoggingReceiverActiveDirectoryDS) Components(tag string) []fluentbit.Component {
	l := confgenerator.LoggingReceiverWindowsEventLog{
		Channels: []string{"Directory Service", "Active Directory Web Services"},
	}

	return l.Components(tag)
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverActiveDirectoryDS{} }, "windows")
}
