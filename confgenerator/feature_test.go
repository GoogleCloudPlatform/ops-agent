package confgenerator_test

import (
	"errors"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/google/go-cmp/cmp"
)

var emptyUc = confgenerator.UnifiedConfig{}

var expectedFeatureBase = []confgenerator.Feature{
	{
		Module: "logging",
		Kind:   "service",
		Type:   "pipelines",
		Key:    "default_pipeline_overridden",
		Value:  "false",
	},
	{
		Module: "metrics",
		Kind:   "service",
		Type:   "pipelines",
		Key:    "default_pipeline_overridden",
		Value:  "false",
	},
}

func TestEmptyStruct(t *testing.T) {
	features, err := confgenerator.ExtractFeatures(&emptyUc)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(features, expectedFeatureBase) {
		t.Fatalf("expected: %v, actual: %v", expectedFeatureBase, features)
	}
}

type MetricsReceiverFoo struct {
	MetricsReceiverInlineFoo `yaml:",inline"`
	MetricsReceiverInlineGoo `yaml:",inline"`
}

func (m MetricsReceiverFoo) Type() string {
	return "metricsReceiverFoo"
}

func (m MetricsReceiverFoo) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInlineFoo struct {
	Foo string `yaml:"foo" tracking:"foo"`
}

type MetricsReceiverInlineGoo struct {
	Goo string `yaml:"goo"`
}

func TestValidInlineStruct(t *testing.T) {
	ic := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverFoo"] = MetricsReceiverFoo{
		MetricsReceiverInlineFoo{
			Foo: "foo",
		},
		MetricsReceiverInlineGoo{
			Goo: "goo",
		},
	}
	ic.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&ic)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   confgenerator.MetricsReceiverTypes.Kind,
		Type:   "metricsReceiverFoo",
		Key:    "foo",
		Value:  "foo",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverInvalid struct {
	MetricsReceiverInlineInvalid `yaml:",inline" tracking:"metrics_receiver_inline_error"`
}

func (m MetricsReceiverInvalid) Type() string {
	return "metricsReceiverFoo"
}

func (m MetricsReceiverInvalid) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInlineInvalid struct {
	Foo string `yaml:"foo" tracking:"foo"`
}

func TestInValidInlineStruct(t *testing.T) {
	ic := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverInlineInvalid"] = MetricsReceiverInvalid{
		MetricsReceiverInlineInvalid{
			Foo: "foo",
		},
	}
	ic.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	_, err := confgenerator.ExtractFeatures(&ic)
	if !errors.Is(err, confgenerator.ErrTrackingInlineStruct) {
		t.Fatal(err)
	}
}

type MetricsReceiverPrefix struct {
	MetricsReceiverInnerPrefix `yaml:"metrics_receiver_prefix" tracking:"metrics_receiver_prefix"`
}

func (m MetricsReceiverPrefix) Type() string {
	return "metricsReceiverPrefix"
}

func (m MetricsReceiverPrefix) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInnerPrefix struct {
	Foo string `yaml:"foo" tracking:"foo"`
}

func TestStructPrefix(t *testing.T) {
	ic := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverPrefix"] = MetricsReceiverPrefix{
		MetricsReceiverInnerPrefix{
			Foo: "foo",
		},
	}
	ic.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&ic)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   confgenerator.MetricsReceiverTypes.Kind,
		Type:   "metricsReceiverPrefix",
		Key:    "metrics_receiver_prefix.foo",
		Value:  "foo",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverBlankPrefix struct {
	MetricsReceiverInnerBlankPrefix `yaml:"metrics_receiver_prefix"`
}

func (m MetricsReceiverBlankPrefix) Type() string {
	return "metricsReceiverBlankPrefix"
}

func (m MetricsReceiverBlankPrefix) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInnerBlankPrefix struct {
	Foo string `yaml:"foo" tracking:"foo"`
}

func TestStructBlankPrefix(t *testing.T) {
	ic := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverBlankPrefix"] = MetricsReceiverBlankPrefix{
		MetricsReceiverInnerBlankPrefix{
			Foo: "foo",
		},
	}
	ic.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&ic)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   confgenerator.MetricsReceiverTypes.Kind,
		Type:   "metricsReceiverBlankPrefix",
		Key:    "foo",
		Value:  "foo",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverOverride struct {
	MetricsReceiverInnerOverride `yaml:",inline"`
}

func (m MetricsReceiverOverride) Type() string {
	return "metricsReceiverOverride"
}

func (m MetricsReceiverOverride) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInnerOverride struct {
	Foo string `yaml:"foo" tracking:"foo,goo"`
}

func TestOverride(t *testing.T) {
	ic := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverOverride"] = MetricsReceiverOverride{
		MetricsReceiverInnerOverride{
			Foo: "foo",
		},
	}
	ic.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&ic)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   confgenerator.MetricsReceiverTypes.Kind,
		Type:   "metricsReceiverOverride",
		Key:    "foo",
		Value:  "goo",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverPointer struct {
	MetricsReceiverInnerPointer `yaml:",inline"`
}

func (m MetricsReceiverPointer) Type() string {
	return "metricsReceiverPointer"
}

func (m MetricsReceiverPointer) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInnerPointer struct {
	Foo *bool `yaml:"foo" tracking:"foo"`
}

func TestPointer(t *testing.T) {
	ic := emptyUc
	foo := true
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverPointer"] = MetricsReceiverPointer{
		MetricsReceiverInnerPointer{
			Foo: &foo,
		},
	}
	ic.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&ic)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   confgenerator.MetricsReceiverTypes.Kind,
		Type:   "metricsReceiverPointer",
		Key:    "foo",
		Value:  "true",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

func TestOverrideDefaultPipeline(t *testing.T) {
	uc := emptyUc
	uc.Metrics = &confgenerator.Metrics{
		Service: &confgenerator.MetricsService{
			Pipelines: map[string]*confgenerator.MetricsPipeline{
				"default_pipeline": {
					ReceiverIDs: []string{"foo", "goo", "bar"},
				},
			},
		},
	}

	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected[1].Value = "true"

	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}
