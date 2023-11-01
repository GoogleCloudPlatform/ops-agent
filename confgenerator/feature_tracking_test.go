// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package confgenerator_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/GoogleCloudPlatform/ops-agent/apps"
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

var expectedTestFeatureBase = []confgenerator.Feature{
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
	{
		Module: confgenerator.MetricsReceiverTypes.Subagent,
		Kind:   "receivers",
		Type:   "metricsReceiverFoo",
		Key:    []string{"[0]", "enabled"},
		Value:  "true",
	},
}

func TestEmptyConfig(t *testing.T) {
	features, err := confgenerator.ExtractFeatures(&emptyUc)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(features, expectedFeatureBase) {
		t.Fatalf("expected: %v, actual: %v", expectedFeatureBase, features)
	}
}

type Test struct {
	Name          string
	Config        *confgenerator.UnifiedConfig
	Expected      []confgenerator.Feature
	ExpectedError error
}

func TestBed(t *testing.T) {
	b := true

	tests := []Test{
		{
			Name: "StringWithTracking",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFoo: MetricsReceiverInlineFoo{
								StringWithTracking: "foo",
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "stringWithTracking"},
					Value:  "foo",
				},
			),
		},
		{
			Name: "StringWithoutTracking",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFoo: MetricsReceiverInlineFoo{
								StringWithoutTracking: "foo",
							},
						},
					},
				},
			},
			Expected: expectedTestFeatureBase,
		},
		{
			Name: "BoolWithAutoTracking",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFoo: MetricsReceiverInlineFoo{
								Bool: true,
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "bool"},
					Value:  "true",
				},
			),
		},
		{
			Name: "PointerBool",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFoo: MetricsReceiverInlineFoo{
								Ptr: &b,
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "ptr"},
					Value:  "true",
				},
			),
		},
		{
			Name: "EmptyStruct",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFoo: MetricsReceiverInlineFoo{
								Struct: MetricsReceiverInnerPointer{},
							},
						},
					},
				},
			},
			Expected: expectedTestFeatureBase,
		},
		{
			Name: "Struct",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFoo: MetricsReceiverInlineFoo{
								Struct: MetricsReceiverInnerPointer{
									Foo: &b,
								},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "struct"},
					Value:  "override",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "struct", "foo"},
					Value:  "true",
				},
			),
		},
		{
			Name: "MapLength",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFooMap: MetricsReceiverInlineFooMap{
								StringWithTracking: map[string]string{
									"a": "",
									"b": "",
									"c": "",
									"d": "",
								},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "stringMap", "__length"},
					Value:  "4",
				},
			),
		},
		{
			Name: "MapStringWithTracking",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFooMap: MetricsReceiverInlineFooMap{
								StringWithTracking: map[string]string{
									"foo": "goo",
									"bar": "baz",
								},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "stringMap", "__length"},
					Value:  "2",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "stringMap", "[0]"},
					Value:  "baz",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "stringMap", "[1]"},
					Value:  "goo",
				},
			),
		},
		{
			Name: "MapStringKeyGeneration",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineFooMap: MetricsReceiverInlineFooMap{
								MapKeys: map[string]string{
									"a": "foo",
								},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "mapKeys", "__length"},
					Value:  "1",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "mapKeys", "[0]", "__key"},
					Value:  "a",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "mapKeys", "[0]"},
					Value:  "overrideValue2",
				},
			),
		},
		{
			Name: "InvalidStruct",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverInlineInvalid: MetricsReceiverInlineInvalid{
								Foo: "foo",
							},
						},
					},
				},
			},
			ExpectedError: confgenerator.ErrTrackingInlineStruct,
		},
		{
			Name: "SliceInt",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverSliceInline: MetricsReceiverSliceInline{
								Int: []int{1, 2, 3},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "int", "__length"},
					Value:  "3",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "int", "[0]"},
					Value:  "1",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "int", "[1]"},
					Value:  "2",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "int", "[2]"},
					Value:  "3",
				},
			),
		},
		{
			Name: "SliceString",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverSliceInline: MetricsReceiverSliceInline{
								String: []string{"foo", "goo", "bar"},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "string", "__length"},
					Value:  "3",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "string", "[0]"},
					Value:  "foo",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "string", "[1]"},
					Value:  "goo",
				},
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "string", "[2]"},
					Value:  "bar",
				},
			),
		},
		{
			Name: "SliceEmpty",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverSliceInline: MetricsReceiverSliceInline{
								SliceStruct: []MetricsReceiverSliceStruct{},
							},
						},
					},
				},
			},
			Expected: append(
				expectedTestFeatureBase,
				confgenerator.Feature{
					Module: confgenerator.MetricsReceiverTypes.Subagent,
					Kind:   "receivers",
					Type:   "metricsReceiverFoo",
					Key:    []string{"[0]", "sliceStruct", "__length"},
					Value:  "0",
				},
			),
		},
		{
			Name: "SliceNil",
			Config: &confgenerator.UnifiedConfig{
				Metrics: &confgenerator.Metrics{
					Receivers: map[string]confgenerator.MetricsReceiver{
						"metricsReceiverFoo": &MetricsReceiverFoo{
							ConfigComponent: confgenerator.ConfigComponent{
								Type: "MetricsReceiverFoo",
							},
							MetricsReceiverSliceInline: MetricsReceiverSliceInline{
								SliceStruct: nil,
							},
						},
					},
				},
			},
			Expected: expectedTestFeatureBase,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			actual, err := confgenerator.ExtractFeatures(test.Config)

			if test.ExpectedError != nil {
				if test.Expected != nil {
					t.Fatalf("invalid test: %v", test.Name)
				}
				if !errors.Is(err, test.ExpectedError) {
					t.Fatal(err)
				}
			} else {
				expected := test.Expected
				if !cmp.Equal(actual, expected) {
					t.Fatalf("expected: %v, actual: %v, \ndiff: %v", expected, actual, cmp.Diff(expected, actual))
				}
			}
		})
	}
}

type MetricsReceiverFoo struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	MetricsReceiverInlineFoo      `yaml:",inline"`
	MetricsReceiverInlineFooMap   `yaml:",inline"`
	// tracking tag is not allowed on inline struct
	MetricsReceiverInlineInvalid `yaml:",inline" tracking:"metrics_receiver_inline_error"`
	MetricsReceiverInnerPrefix   `yaml:"metrics_receiver_prefix" tracking:"metrics_receiver_override"`
	MetricsReceiverInnerOverride `yaml:",inline"`
	MetricsReceiverInnerPointer  `yaml:",inline"`
	MetricsReceiverSliceInline   `yaml:",inline"`
}

func (m MetricsReceiverFoo) Type() string {
	return "metricsReceiverFoo"
}

func (m MetricsReceiverFoo) Pipelines(_ context.Context) []otel.ReceiverPipeline {
	return nil
}

type MetricsReceiverInlineFoo struct {
	StringWithTracking    string                      `yaml:"stringWithTracking" tracking:""`
	StringWithoutTracking string                      `yaml:"stringWithoutTracking"`
	Bool                  bool                        `yaml:"bool"`
	Ptr                   *bool                       `yaml:"ptr"`
	Struct                MetricsReceiverInnerPointer `yaml:"struct" tracking:"override"`
}

type MetricsReceiverInlineFooMap struct {
	StringWithTracking map[string]string `yaml:"stringMap" tracking:""`
	MapKeys            map[string]string `yaml:"mapKeys" tracking:"overrideValue2,keys"`
}

type MetricsReceiverInlineInvalid struct {
	Foo string `yaml:"foo" tracking:"foo"`
}

type MetricsReceiverInnerPrefix struct {
	Foo string `yaml:"foo" tracking:""`
}

type MetricsReceiverInnerOverride struct {
	Foo string `yaml:"foo" tracking:"goo"`
}

type MetricsReceiverInnerPointer struct {
	Foo *bool `yaml:"foo" tracking:""`
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
		ConfigComponent: confgenerator.ConfigComponent{
			Type: "prometheus",
		},
		PromConfig: promconfig.Config{
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
	components := confgenerator.LoggingReceiverTypes.GetComponentsFromRegistry()
	components = append(components, confgenerator.LoggingProcessorTypes.GetComponentsFromRegistry()...)
	components = append(components, confgenerator.MetricsReceiverTypes.GetComponentsFromRegistry()...)
	components = append(components, confgenerator.MetricsProcessorTypes.GetComponentsFromRegistry()...)
	components = append(components, confgenerator.CombinedReceiverTypes.GetComponentsFromRegistry()...)

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

func (m Example) Pipelines(_ context.Context) []otel.ReceiverPipeline {
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

type MetricsReceiverSliceInline struct {
	Int         []int                        `yaml:"int" tracking:""`
	String      []string                     `yaml:"string" tracking:""`
	SliceStruct []MetricsReceiverSliceStruct `yaml:"sliceStruct" tracking:"overrideValue"`
}

type MetricsReceiverSliceStruct struct {
	Int    int    `yaml:"int"`
	String string `yaml:"string" tracking:""`
	PII    string `yaml:"string"`
}
