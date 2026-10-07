package confgenerator

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

func (p PipelineInstance) FluentBitComponents(ctx context.Context) (fbSource, error) {
	tag := fmt.Sprintf("%s.%s", p.PID, p.RID)

	// For fluent_forward we create the tag in the following format:
	// <hash_string>.<pipeline_id>.<receiver_id>.<existing_tag>
	//
	// hash_string: Deterministic unique identifier for the pipeline_id + receiver_id.
	//   This is needed to prevent collisions between receivers in the same
	//   pipeline when using the glob syntax for matching (using wildcards).
	// pipeline_id: User defined pipeline_id but with the "." replaced with "_"
	//   since the "." character is reserved to be used as a delimiter in the
	//   Lua script.
	// receiver_id: User defined receiver_id but with the "." replaced with "_"
	//   since the "." character is reserved to be used as a delimiter in the
	//   Lua script.
	//  existing_tag: Tag associated with the record prior to ingesting.
	//
	// For an example testing collisions in receiver_ids, see:
	//
	// testdata/valid/linux/logging-receiver_forward_multiple_receivers_conflicting_id
	if p.Receiver.Type() == "fluent_forward" {
		hashString := getMD5Hash(tag)

		// Note that we only update the tag for the tag. The LogName will still
		// use the user defined receiver_id without this replacement.
		pipelineIdCleaned := strings.ReplaceAll(p.PID, ".", "_")
		receiverIdCleaned := strings.ReplaceAll(p.RID, ".", "_")
		tag = fmt.Sprintf("%s.%s.%s", hashString, pipelineIdCleaned, receiverIdCleaned)
	}
	receiver, processors, err := p.simplifiedLoggingComponents(ctx)
	if err != nil {
		return fbSource{}, err
	}
	var components []fluentbit.Component
	receiverComponents := receiver.Components(ctx, tag)
	components = append(components, receiverComponents...)

	// To match on fluent_forward records, we need to account for the addition
	// of the existing tag (unknown during config generation) as the suffix
	// of the tag.
	globSuffix := ""
	regexSuffix := ""
	if p.Receiver.Type() == "fluent_forward" {
		regexSuffix = `\..*`
		globSuffix = `.*`
	}
	tagRegex := regexp.QuoteMeta(tag) + regexSuffix
	tag = tag + globSuffix

	for i, processor := range processors {
		processorComponents := processor.Components(ctx, tag, strconv.Itoa(i))
		components = append(components, processorComponents...)
	}
	components = append(components, setLogNameComponents(ctx, tag, p.RID, p.Receiver.Type())...)

	// Logs ingested using the fluent_forward receiver must add the existing_tag
	// on the record to the LogName. This is done with a Lua filter.
	if p.Receiver.Type() == "fluent_forward" {
		components = append(components, fluentbit.LuaFilterComponents(tag, addLogNameLuaFunction, addLogNameLuaScriptContents)...)
	}
	return fbSource{
		TagRegex:   tagRegex,
		Components: components,
	}, nil
}

func (uc *UnifiedConfig) GenerateFluentBitConfigs(ctx context.Context, logsDir string, stateDir string) (map[string]string, error) {
	userAgent, _ := platform.FromContext(ctx).UserAgent("Google-Cloud-Ops-Agent-Logging")

	components, err := uc.generateFluentbitComponents(ctx, userAgent)
	if err != nil {
		return nil, err
	}

	c := fluentbit.ModularConfig{
		Variables: map[string]string{
			"buffers_dir": path.Join(stateDir, "buffers"),
			"logs_dir":    logsDir,
		},
		Components: components,
	}
	return c.Generate()
}

func (uc *UnifiedConfig) generateFluentbitComponents(ctx context.Context, userAgent string) ([]fluentbit.Component, error) {
	l := uc.Logging
	var out []fluentbit.Component
	if l.Service.LogLevel == "" {
		l.Service.LogLevel = "info"
	}
	service := fluentbit.Service{LogLevel: l.Service.LogLevel}
	out = append(out, service.Component())
	out = append(out, fluentbit.MetricsInputComponent())

	if l != nil && l.Service != nil && (l.Service.OTelLogging == nil || !*l.Service.OTelLogging) {
		// Type for sorting.
		var sources []fbSource
		var tags []string
		pipelines, err := uc.Pipelines(ctx)
		if err != nil {
			return nil, err
		}
		for _, pipeline := range pipelines {
			if pipeline.Backend != BackendFluentBit {
				continue
			}
			source, err := pipeline.FluentBitComponents(ctx)
			if err != nil {
				return nil, err
			}
			sources = append(sources, source)
			tags = append(tags, source.TagRegex)
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i].TagRegex < sources[j].TagRegex })
		sort.Strings(tags)

		for _, s := range sources {
			out = append(out, s.Components...)
		}
		if len(tags) > 0 {
			out = append(out, stackdriverOutputComponent(ctx, strings.Join(tags, "|"), userAgent, "2G", l.Service.Compress))
		}
		out = append(out, addGceMetadataAttributesProcessor(ctx).Components(ctx, "*", "*.default-data-proc.gce_metadata")...)
	}
	out = append(out, uc.generateSelfLogsComponents(ctx, userAgent)...)
	out = append(out, fluentbit.MetricsOutputComponent(int(uc.GetFluentBitMetricsPort())))

	return out, nil
}

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}
