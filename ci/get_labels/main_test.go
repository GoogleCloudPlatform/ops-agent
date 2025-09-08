// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gl "github.com/GoogleCloudPlatform/ops-agent/ci/get_labels"
)

type Test struct {
	Name   string
	Labels gl.RespLabelCollection
}

func TestLabels(t *testing.T) {
	testTable := []Test{
		{
			Name: "empty",
		},
		{
			Name: "release & foo",
			Labels: gl.RespLabelCollection{
				{
					Name: "release",
				},
				{
					Name: "foo",
				},
			},
		},
	}

	for _, test := range testTable {
		t.Run(test.Name, func(t *testing.T) {
			testGetLabels(t, test.Labels)
		})
	}
}

func testGetLabels(t *testing.T, expected gl.RespLabelCollection) {
	t.Parallel()
	server := getHttpClient(t, expected)
	defer server.Close()
	labelCollector := gl.LabelCollector{
		Client: server.Client(),
	}

	actual, err := labelCollector.GetLabels(server.URL + "/GoogleCloudPlatform/ops-agent/pulls/818")
	if err != nil {
		t.Fatal(err)
	}

	assertRespLabelCollectionEqual(t, expected, actual)
}

func assertRespLabelCollectionEqual(t *testing.T, expected gl.RespLabelCollection, actual gl.RespLabelCollection) {
	if len(actual) != len(expected) {
		t.Fatalf("Actual did not meet expected.\nactual: \"%s\"\nexpected: \"%s\"", actual, expected)
	}

	for i := range actual {
		if actual[i] != expected[i] {
			t.Fatalf("Actual did not meet expected.\nactual: \"%s\"\nexpected: \"%s\"", actual[i], expected[i])
		}
	}

	if actual.String() != expected.String() {
		t.Fatalf("Actual did not meet expected.\nactual: \"%s\"\nexpected: \"%s\"", actual, expected)
	}
}

func getHttpClient(t *testing.T, respLabels gl.RespLabelCollection) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bytes, err := json.Marshal(respLabels)
		if err != nil {
			t.Fatal(err)
		}

		_, err = writer.Write(bytes)
		if err != nil {
			t.Fatal(err)
		}
	}))
	return server
}
