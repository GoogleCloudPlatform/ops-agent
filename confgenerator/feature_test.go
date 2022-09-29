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
		Key:    []string{"default_pipeline_overridden"},
		Value:  "false",
	},
	{
		Module: "metrics",
		Kind:   "service",
		Type:   "pipelines",
		Key:    []string{"default_pipeline_overridden"},
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
	confgenerator.ConfigComponent `yaml:",inline"`
	MetricsReceiverInlineFoo      `yaml:",inline"`
	MetricsReceiverInlineGoo      `yaml:",inline"`
	MetricsReceiverInlineBar      `yaml:",inline"`
}

func (m MetricsReceiverFoo) Type() string {
	return "metricsReceiverFoo"
}

func (m MetricsReceiverFoo) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverGoo struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	MetricsReceiverInlineGoo      `yaml:",inline"`
	MetricsReceiverInlineBar      `yaml:",inline"`
}

func (m MetricsReceiverGoo) Type() string {
	return "metricsReceiverGoo"
}

func (m MetricsReceiverGoo) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInlineFoo struct {
	Foo string `yaml:"foo" tracking:""`
}

type MetricsReceiverInlineGoo struct {
	Goo string `yaml:"goo"`
}

type MetricsReceiverInlineBar struct {
	Bar string `yaml:"bar" tracking:""`
}

func TestValidInlineStruct(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverFoo"] = MetricsReceiverFoo{
		confgenerator.ConfigComponent{
			Type: "MetricsReceiverFoo",
		},
		MetricsReceiverInlineFoo{
			Foo: "foo",
		},
		MetricsReceiverInlineGoo{
			Goo: "goo",
		},
		MetricsReceiverInlineBar{
			Bar: "baz",
		},
	}
	receivers["metricsReceiverGoo"] = MetricsReceiverGoo{
		confgenerator.ConfigComponent{
			Type: "MetricsReceiverGoo",
		},
		MetricsReceiverInlineGoo{
			Goo: "goo",
		},
		MetricsReceiverInlineBar{
			Bar: "baz",
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"0", "enabled"},
		Value:  "true",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"0", "foo"},
		Value:  "foo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"0", "bar"},
		Value:  "baz",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverGoo",
		Key:    []string{"1", "enabled"},
		Value:  "true",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverGoo",
		Key:    []string{"1", "bar"},
		Value:  "baz",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverFooMap struct {
	MetricsReceiverInlineFooMap `yaml:",inline"`
}

func (m MetricsReceiverFooMap) Type() string {
	return "metricsReceiverFooMap"
}

func (m MetricsReceiverFooMap) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInlineFooMap struct {
	Foo map[string]string `yaml:"fooMap" tracking:""`
}

func TestValidInlineStructWithMapValue(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverFoo"] = MetricsReceiverFooMap{
		MetricsReceiverInlineFooMap{
			Foo: map[string]string{
				"foo": "goo",
				"bar": "baz",
			},
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"0", "fooMap", "0"},
		Value:  "goo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"0", "fooMap", "1"},
		Value:  "baz",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverFooSlice struct {
	MetricsReceiverInlineFooSlice `yaml:",inline"`
}

func (m MetricsReceiverInlineFooSlice) Type() string {
	return "metricsReceiverInlineFooSlice"
}

func (m MetricsReceiverInlineFooSlice) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInlineFooSlice struct {
	Foo []string `yaml:"foo" tracking:""`
}

func TestValidInlineStructWithSliceValue(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverFoo"] = MetricsReceiverFooSlice{
		MetricsReceiverInlineFooSlice{
			Foo: []string{"foo", "goo", "bar", "baz"},
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverInlineFooSlice",
		Key:    []string{"0", "foo"},
		Value:  "[foo goo bar baz]",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

// tracking tag is not allowed on inline struct
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
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverInlineInvalid"] = MetricsReceiverInvalid{
		MetricsReceiverInlineInvalid{
			Foo: "foo",
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	_, err := confgenerator.ExtractFeatures(&uc)
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
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverPrefix"] = MetricsReceiverPrefix{
		MetricsReceiverInnerPrefix{
			Foo: "foo",
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverPrefix",
		Key:    []string{"0", "metrics_receiver_prefix", "foo"},
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
	Foo string `yaml:"foo" tracking:"goo"`
}

func TestOverride(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverOverride"] = MetricsReceiverOverride{
		MetricsReceiverInnerOverride{
			Foo: "foo",
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverOverride",
		Key:    []string{"0", "foo"},
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
	Foo *bool `yaml:"foo" tracking:""`
}

func TestPointer(t *testing.T) {
	uc := emptyUc
	foo := true
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverPointer"] = MetricsReceiverPointer{
		MetricsReceiverInnerPointer{
			Foo: &foo,
		},
	}
	uc.Metrics = &confgenerator.Metrics{
		Receivers: receivers,
	}
	features, err := confgenerator.ExtractFeatures(&uc)
	if err != nil {
		t.Fatal(err)
	}

	expected := expectedFeatureBase
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverPointer",
		Key:    []string{"0", "foo"},
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
