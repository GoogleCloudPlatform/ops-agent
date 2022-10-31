package confgenerator_test

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/google/go-cmp/cmp"
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

func (m MetricsReceiverFooMap) Pipelines() []otel.Pipeline {
	return nil
}

type MetricsReceiverInlineFooMap struct {
	Foo map[string]string `yaml:"fooMap" tracking:"fooMapOverride"`
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
	_, err := confgenerator.ExtractFeatures(&uc)
	if !errors.Is(err, confgenerator.ErrMapAsField) {
		t.Fatal(err)
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
		Key:    []string{"[0]", "foo"},
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

func TestGolden(t *testing.T) {
	_ = apps.BuiltInConfStructs
	components := confgenerator.GetComponentsFromRegistry(confgenerator.LoggingReceiverTypes)
	components = append(components, confgenerator.GetComponentsFromRegistry(confgenerator.LoggingProcessorTypes)...)
	components = append(components, confgenerator.GetComponentsFromRegistry(confgenerator.MetricsReceiverTypes)...)
	components = append(components, confgenerator.GetComponentsFromRegistry(confgenerator.MetricsProcessorTypes)...)

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

	for j := 0; j < t.NumField(); j++ {
		f := t.Field(j)
		override, ok := f.Tag.Lookup("tracking")
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
