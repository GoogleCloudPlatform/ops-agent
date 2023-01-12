package confgenerator_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/model/relabel"
	"gotest.tools/v3/golden"
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

func (m MetricsReceiverFoo) Pipelines() []otel.ReceiverPipeline {
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

func (m MetricsReceiverGoo) Pipelines() []otel.ReceiverPipeline {
	return nil
}

type MetricsReceiverInlineFoo struct {
	Foo   string   `yaml:"foo" tracking:""`
	Bool  *bool    `yaml:"bool"`
	Int   int      `yaml:"int"`
	Slice []string `yaml:"slice" tracking:""`
	PII   string   `yaml:"pii"`
}

type MetricsReceiverInlineGoo struct {
	Goo string `yaml:"goo"`
}

type MetricsReceiverInlineBar struct {
	Bar string `yaml:"bar" tracking:""`
}

func TestValidInlineStruct(t *testing.T) {
	uc := emptyUc
	b := false
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverFoo"] = MetricsReceiverFoo{
		confgenerator.ConfigComponent{
			Type: "MetricsReceiverFoo",
		},
		MetricsReceiverInlineFoo{
			Foo:   "foo",
			Bool:  &b,
			Int:   0,
			Slice: []string{"foo", "goo", "bar"},
			PII:   "PII",
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
		Key:    []string{"[0]", "enabled"},
		Value:  "true",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "foo"},
		Value:  "foo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "bool"},
		Value:  "false",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "slice", "__length"},
		Value:  "3",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "slice", "[0]"},
		Value:  "foo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "slice", "[1]"},
		Value:  "goo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "slice", "[2]"},
		Value:  "bar",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "bar"},
		Value:  "baz",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverGoo",
		Key:    []string{"[1]", "enabled"},
		Value:  "true",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverGoo",
		Key:    []string{"[1]", "bar"},
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

func (m MetricsReceiverFooMap) Pipelines() []otel.ReceiverPipeline {
	return nil
}

type MetricsReceiverInlineFooMap struct {
	ExampleString map[string]string                   `yaml:"exampleStringMap" tracking:""`
	ExampleStruct map[string]MetricsReceiverMapStruct `yaml:"exampleStructMap" tracking:"overrideValue1"`
	ExampleKeys   map[string]string                   `yaml:"exampleKeysMap" tracking:"overrideValue2,keys"`
}

type MetricsReceiverMapStruct struct {
	Int            int    `yaml:"int"`
	String         string `yaml:"string" tracking:""`
	OverrideString string `yaml:"override" tracking:"overrideString"`
	PII            string `yaml:"string"`
}

func TestValidInlineStructWithMapValue(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverFoo"] = MetricsReceiverFooMap{
		MetricsReceiverInlineFooMap{
			ExampleString: map[string]string{
				"foo": "goo",
				"bar": "baz",
			},
			ExampleStruct: map[string]MetricsReceiverMapStruct{
				"a": {
					Int:            32,
					String:         "abc",
					PII:            "PII",
					OverrideString: "string",
				},
				"b": {
					Int:            64,
					String:         "xyz",
					PII:            "PII2",
					OverrideString: "string",
				},
			},
			ExampleKeys: map[string]string{
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
		Key:    []string{"[0]", "exampleStringMap", "__length"},
		Value:  "2",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStringMap", "[0]"},
		Value:  "goo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStringMap", "[1]"},
		Value:  "baz",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "__length"},
		Value:  "2",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[0]"},
		Value:  "overrideValue1",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[0]", "int"},
		Value:  "32",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[0]", "string"},
		Value:  "abc",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[0]", "override"},
		Value:  "overrideString",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[1]"},
		Value:  "overrideValue1",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[1]", "int"},
		Value:  "64",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[1]", "string"},
		Value:  "xyz",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleStructMap", "[1]", "override"},
		Value:  "overrideString",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleKeysMap", "__length"},
		Value:  "2",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleKeysMap", "foo"},
		Value:  "overrideValue2",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFooMap",
		Key:    []string{"[0]", "exampleKeysMap", "bar"},
		Value:  "overrideValue2",
	})

	// Iterating through maps are not always in the same order
	// `compareFeatures()` equates `features` and `expected` regardless of order
	if !compareFeatures(features, expected) {
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

func (m MetricsReceiverInvalid) Pipelines() []otel.ReceiverPipeline {
	return nil
}

type MetricsReceiverInlineInvalid struct {
	Foo string `yaml:"foo" tracking:"foo"`
}

func TestInvalidInlineStruct(t *testing.T) {
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
	MetricsReceiverInnerPrefix `yaml:"metrics_receiver_prefix" tracking:"metrics_receiver_override"`
}

func (m MetricsReceiverPrefix) Type() string {
	return "metricsReceiverPrefix"
}

func (m MetricsReceiverPrefix) Pipelines() []otel.ReceiverPipeline {
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
		Key:    []string{"[0]", "metrics_receiver_prefix"},
		Value:  "metrics_receiver_override",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverPrefix",
		Key:    []string{"[0]", "metrics_receiver_prefix", "foo"},
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

func (m MetricsReceiverOverride) Pipelines() []otel.ReceiverPipeline {
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
		Key:    []string{"[0]", "foo"},
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

func (m MetricsReceiverPointer) Pipelines() []otel.ReceiverPipeline {
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
		Key:    []string{"[0]", "foo"},
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
			Pipelines: map[string]*confgenerator.Pipeline{
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

	expected := []confgenerator.Feature{
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
			Value:  "true",
		},
	}

	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

func TestPrometheusFeatureMetrics(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["prometheus"] = confgenerator.PrometheusMetrics{
		confgenerator.ConfigComponent{
			Type: "prometheus",
		},
		promconfig.Config{
			GlobalConfig: promconfig.GlobalConfig{
				ScrapeInterval:     model.Duration(10 * time.Second),
				ScrapeTimeout:      model.Duration(10 * time.Second),
				EvaluationInterval: model.Duration(10 * time.Second),
			},
			ScrapeConfigs: []*promconfig.ScrapeConfig{
				{
					JobName: "prometheus",
					ServiceDiscoveryConfigs: discovery.Configs{
						discovery.StaticConfig{
							{
								Targets: []model.LabelSet{
									{model.AddressLabel: "localhost:8888"},
									{model.AddressLabel: "localhost:8889"},
								},
							},
							{
								Targets: []model.LabelSet{
									{model.AddressLabel: "localhost:8890"},
								},
							},
						},
					},
					MetricsPath:           "/metrics",
					Scheme:                "http",
					HonorLabels:           false,
					HonorTimestamps:       true,
					ScrapeInterval:        model.Duration(10 * time.Second),
					ScrapeTimeout:         model.Duration(10 * time.Second),
					SampleLimit:           10,
					TargetLimit:           10,
					LabelLimit:            10,
					LabelNameLengthLimit:  10,
					LabelValueLengthLimit: 10,
					BodySizeLimit:         10,
					RelabelConfigs: []*relabel.Config{
						{
							SourceLabels: model.LabelNames{"__meta_kubernetes_pod_label_app"},
							Action:       "keep",
							Regex:        relabel.MustNewRegexp(".*"),
						},
					},
					MetricRelabelConfigs: []*relabel.Config{
						{SourceLabels: model.LabelNames{"__name__"},
							Action: "keep",
							Regex:  relabel.MustNewRegexp(".*"),
						},
					},
				},
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
		Module: "metrics",
		Kind:   "receivers",
		Type:   "prometheus",
		Key:    []string{"[0]", "enabled"},
		Value:  "true",
	})

	type expectedFeatureTags struct {
		keys []string
		val  string
	}
	testCases := []expectedFeatureTags{
		{
			keys: []string{"scheme"},
			val:  "http",
		},
		{
			keys: []string{"honor_timestamps"},
			val:  "true",
		},
		{
			keys: []string{"scrape_interval"},
			val:  "10s",
		},
		{
			keys: []string{"scrape_timeout"},
			val:  "10s",
		},
		{
			keys: []string{"sample_limit"},
			val:  "10",
		},
		{
			keys: []string{"label_limit"},
			val:  "10",
		},
		{
			keys: []string{"label_name_length_limit"},
			val:  "10",
		},
		{
			keys: []string{"label_value_length_limit"},
			val:  "10",
		},
		{
			keys: []string{"body_size_limit"},
			val:  "10",
		},
		{
			keys: []string{"relabel_configs"},
			val:  "1",
		},
		{
			keys: []string{"metric_relabel_configs"},
			val:  "1",
		},
		{
			keys: []string{"static_config_target_groups"},
			val:  "2",
		},
	}
	for _, test := range testCases {
		expected = append(expected, confgenerator.Feature{
			Module: "metrics",
			Kind:   "receivers",
			Type:   "prometheus",
			Key:    append([]string{"[0]", "config", "[0]", "scrape_configs"}, test.keys...),
			Value:  test.val,
		})
	}

	if !cmp.Equal(features, expected) {
		t.Errorf("Expected %d features but got %d\n\n", len(expected), len(features))
		t.Fatalf("Diff: %s", cmp.Diff(expected, features))
	}
}

func TestGolden(t *testing.T) {
	_ = apps.BuiltInConfStructs
	components := confgenerator.LoggingReceiverTypes.GetComponentsFromRegistry()
	components = append(components, confgenerator.LoggingProcessorTypes.GetComponentsFromRegistry()...)
	components = append(components, confgenerator.MetricsReceiverTypes.GetComponentsFromRegistry()...)
	components = append(components, confgenerator.MetricsProcessorTypes.GetComponentsFromRegistry()...)

	features := getFeatures(components)

	bufferString := bytes.NewBufferString("")
	csvWriter := csv.NewWriter(bufferString)
	err := csvWriter.WriteAll(features)
	if err != nil {
		log.Fatal(err)
	}
	csvWriter.Flush()

	// Remove extra newline before sorting
	bufStr := bufferString.String()
	bufStr = bufStr[:len(bufStr)-1]

	// Sort results for assertion is consistent
	s := strings.Split(bufStr, "\n")
	sort.Strings(s)
	// Add title after re-ordering
	title := []string{"App,Field,Override,"}
	s = append(title, s...)
	// Add newline back
	actual := fmt.Sprintf("%s\n", strings.Join(s, "\n"))
	golden.Assert(t, actual, "feature/golden.csv")
}

func getFeatures(components []confgenerator.Component) [][]string {
	points := make([][]string, 0)
	for _, c := range components {
		p := []string{reflect.TypeOf(c).String()}
		points = append(points, getFeaturesForComponent(c, p)...)
	}
	return points
}

func getFeaturesForComponent(i interface{}, parent []string) [][]string {
	features := make([][]string, 0)

	v := reflect.Indirect(reflect.ValueOf(i))
	t := v.Type()

	// Short circuit if the component defines its own feature extraction.
	if customFeatures, ok := v.Interface().(confgenerator.CustomFeatures); ok {
		features := customFeatures.ListAllFeatures()
		fullFeatures := make([][]string, 0)
		for _, feature := range features {
			fullFeatures = append(fullFeatures, appendFieldName(parent, feature))
		}
		return fullFeatures
	}

	for j := 0; j < t.NumField(); j++ {
		f := t.Field(j)
		override, ok := f.Tag.Lookup("tracking")
		if override == "-" {
			// Skip fields with tracking tag "-".
			continue
		}
		switch f.Type.Kind() {
		case reflect.Struct:
			p := appendFieldName(parent, f.Type.String())
			features = append(features, getFeaturesForComponent(v.Field(j).Interface(), p)...)
		case reflect.Map:
			m := v.Field(j)
			if ok {
				for _, k := range m.MapKeys() {
					features = append(features, getFeaturesForComponent(m.MapIndex(k).Interface(), append(parent, k.String()))...)
				}
			}
		case reflect.Slice, reflect.Array, reflect.String:
			if ok {
				p := append(appendFieldName(parent, f.Name), override)
				features = append(features, p)
			}
		default:
			p := append(appendFieldName(parent, f.Name), override)
			features = append(features, p)
		}
	}
	return features
}

func appendFieldName(parent []string, fieldName string) []string {
	p := make([]string, len(parent))
	p[0] = parent[0]
	if len(p) > 1 {
		p[1] = fmt.Sprintf("%v.%v", parent[1], fieldName)
	} else {
		p = append(p, fieldName)
	}
	return p
}

type Example struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	A                             Nested `yaml:"a" tracking:"nestedOverride"`
	B                             Nested `yaml:"b"`
}

type Nested struct {
	Str string `yaml:"str" tracking:""`
	In  int    `yaml:"int"`
}

func (m Example) Type() string {
	return "example"
}

func (m Example) Pipelines() []otel.ReceiverPipeline {
	return nil
}

func TestNestedStructs(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["example"] = Example{
		ConfigComponent: confgenerator.ConfigComponent{Type: "Example"},
		A: Nested{
			In:  32,
			Str: "foo",
		},
		B: Nested{
			In:  64,
			Str: "goo",
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
		Type:   "example",
		Key:    []string{"[0]", "enabled"},
		Value:  "true",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "example",
		Key:    []string{"[0]", "a"},
		Value:  "nestedOverride",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "example",
		Key:    []string{"[0]", "a", "str"},
		Value:  "foo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "example",
		Key:    []string{"[0]", "a", "int"},
		Value:  "32",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "example",
		Key:    []string{"[0]", "b", "str"},
		Value:  "goo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "example",
		Key:    []string{"[0]", "b", "int"},
		Value:  "64",
	})
	if !cmp.Equal(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

type MetricsReceiverSlice struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	MetricsReceiverSliceInline    `yaml:",inline"`
}

func (m MetricsReceiverSlice) Type() string {
	return "metricsReceiverSlice"
}

func (m MetricsReceiverSlice) Pipelines() []otel.ReceiverPipeline {
	return nil
}

type MetricsReceiverSliceInline struct {
	SliceInt            []int                        `yaml:"sliceInt" tracking:""`
	SliceStringOverride []string                     `yaml:"sliceString" tracking:"stringOverride"`
	SliceStruct         []MetricsReceiverSliceStruct `yaml:"sliceStruct" tracking:"overrideValue"`
}

type MetricsReceiverSliceStruct struct {
	Int    int    `yaml:"int"`
	String string `yaml:"string" tracking:""`
	PII    string `yaml:"string"`
}

func TestValidSliceStruct(t *testing.T) {
	uc := emptyUc
	receivers := make(map[string]confgenerator.MetricsReceiver)
	receivers["metricsReceiverSlice"] = MetricsReceiverSlice{
		confgenerator.ConfigComponent{
			Type: "MetricsReceiverSlice",
		},
		MetricsReceiverSliceInline{
			SliceInt:            []int{1, 2, 3},
			SliceStringOverride: []string{"t1", "t2", "t3"},
			SliceStruct: []MetricsReceiverSliceStruct{
				{
					Int:    32,
					String: "foo",
					PII:    "goo",
				},
				{
					Int:    64,
					String: "bar",
					PII:    "baz",
				},
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
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "enabled"},
		Value:  "true",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceInt", "__length"},
		Value:  "3",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceInt", "[0]"},
		Value:  "1",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceInt", "[1]"},
		Value:  "2",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceInt", "[2]"},
		Value:  "3",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceString", "__length"},
		Value:  "3",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceString", "[0]"},
		Value:  "stringOverride",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceString", "[1]"},
		Value:  "stringOverride",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceString", "[2]"},
		Value:  "stringOverride",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "__length"},
		Value:  "2",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "[0]"},
		Value:  "overrideValue",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "[0]", "int"},
		Value:  "32",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "[0]", "string"},
		Value:  "foo",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "[1]"},
		Value:  "overrideValue",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "[1]", "int"},
		Value:  "64",
	})
	expected = append(expected, confgenerator.Feature{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverSlice",
		Key:    []string{"[0]", "sliceStruct", "[1]", "string"},
		Value:  "bar",
	})

	a, _ := json.Marshal(features)
	b, _ := json.Marshal(expected)
	x := string(a)
	y := string(b)
	fmt.Println(x)
	fmt.Println(y)
	if !reflect.DeepEqual(features, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, features)
	}
}

func compareFeatures(features, expected []confgenerator.Feature) bool {
	if len(features) != len(expected) {
		return false
	}
	for _, i := range expected {
		found := false
		for _, j := range features {
			if cmp.Equal(i, j) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
