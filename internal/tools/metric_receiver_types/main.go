package main

import (
	"fmt"
	"sort"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

func main() {
	_ = apps.BuiltInConfStructs
	registry := confgenerator.MetricsReceiverTypes.GetComponentsFromRegistry()
	var types []string
	for _, r := range registry {
		types = append(types, r.Type())
	}
	sort.Strings(types)
	for _, r := range types {
		fmt.Println(fmt.Sprintf(`'%s',`, r))
	}
}
