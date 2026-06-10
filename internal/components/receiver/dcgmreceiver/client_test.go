// Copyright 2023 Google LLC
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

//go:build gpu
// +build gpu

package dcgmreceiver

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestNewDcgmClientOnInitializationError(t *testing.T) {
	realDcgmInit := dcgmInit
	defer func() { dcgmInit = realDcgmInit }()
	dcgmInit = func(...string) (func(), error) {
		return nil, fmt.Errorf("No DCGM client library *OR* No DCGM connection")
	}

	seenDcgmConnectionWarning := false
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.Hooks(func(e zapcore.Entry) error {
		if e.Level == zap.WarnLevel && strings.Contains(e.Message, "Unable to connect to DCGM daemon") {
			seenDcgmConnectionWarning = true
		}
		return nil
	})))

	client, err := newClient(&dcgmClientSettings{endpoint: defaultEndpoint}, logger)
	assert.Equal(t, seenDcgmConnectionWarning, true)
	assert.True(t, errors.Is(err, ErrDcgmInitialization))
	assert.Regexp(t, ".*Unable to connect.*", err)
	assert.Nil(t, client)
}
