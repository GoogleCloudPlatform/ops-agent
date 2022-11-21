package experimental_test

import (
	"os"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/experimental"
	"gotest.tools/v3/assert"
)

func TestExperimental(t *testing.T) {
	tests := []struct {
		input               string
		expectedErrorString string
		assertFunc          func()
	}{
		{
			input:               "",
			expectedErrorString: "",
			assertFunc: func() {
				assert.Equal(t, experimental.PrometheusReceiver, false)
			},
		},
		{
			input:               "prometheus_receiver",
			expectedErrorString: "",
			assertFunc: func() {
				assert.Equal(t, experimental.PrometheusReceiver, true)
			},
		},
		{
			input:               "unrecognized",
			expectedErrorString: "unrecognized",
			assertFunc:          func() {},
		},
		{
			input:               "prometheus_receiver,prometheus_receiver",
			expectedErrorString: "duplicate",
			assertFunc:          func() {},
		},
		{
			input:               "prometheus_receiver ",
			expectedErrorString: "",
			assertFunc: func() {
				assert.Equal(t, experimental.PrometheusReceiver, true)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			os.Setenv("EXPERIMENTAL_FEATURES", test.input)
			err := experimental.Load()
			if test.expectedErrorString != "" {
				assert.ErrorContains(t, err, test.expectedErrorString)
			} else {
				assert.NilError(t, err)
			}
			test.assertFunc()
		})
	}
}

func TestMultipleLoads(t *testing.T) {
	os.Setenv("EXPERIMENTAL_FEATURES", "prometheus_receiver")
	err := experimental.Load()
	assert.NilError(t, err)
	assert.Equal(t, experimental.PrometheusReceiver, true)
	os.Setenv("EXPERIMENTAL_FEATURES", "")
	err = experimental.Load()
	assert.NilError(t, err)
	assert.Equal(t, experimental.PrometheusReceiver, false)
}
