package main_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/ci/get_labels"
)

type Test struct {
	Name   string
	Labels main.RespLabelCollection
}

func TestLabels(t *testing.T) {
	testTable := []Test{
		{
			Name: "empty",
		},
		{
			Name: "release & foo",
			Labels: main.RespLabelCollection{
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

func testGetLabels(t *testing.T, expected main.RespLabelCollection) {
	t.Parallel()
	server := getHttpClient(t, expected)
	defer server.Close()
	labelCollector := main.LabelCollector{
		Client: server.Client(),
	}

	actual, err := labelCollector.GetLabels(server.URL + "/GoogleCloudPlatform/ops-agent/pulls/818")
	if err != nil {
		t.Fatal(err)
	}

	assertRespLabelCollectionEqual(t, expected, actual)
}

func assertRespLabelCollectionEqual(t *testing.T, expected main.RespLabelCollection, actual main.RespLabelCollection) {
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

func getHttpClient(t *testing.T, respLabels main.RespLabelCollection) *httptest.Server {
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
