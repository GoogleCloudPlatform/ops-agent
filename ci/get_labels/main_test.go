package main_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/ci/get_labels"
)

const testLabels = `[
  {
    "name": "release"
  },
	{
		"name": "foo"
	}
]`

func TestGetLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, err := writer.Write([]byte(testLabels))
		if err != nil {
			t.Fatal(err)
		}
	}))

	defer server.Close()
	labelCollector := main.LabelCollector{
		Client: server.Client(),
	}

	actual := labelCollector.GetLabels(server.URL + "/GoogleCloudPlatform/ops-agent/pulls/818")
	expected := "release foo "
	if actual != expected {
		t.Fatalf("Actual did not meet expected.\nactual: \"%s\"\nexpected: \"%s\"", actual, expected)
	}
}
