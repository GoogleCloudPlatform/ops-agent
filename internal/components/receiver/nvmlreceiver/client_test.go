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

//go:build gpu
// +build gpu

package nvmlreceiver

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewNvmlClientDisabledOnLibraryNotFoundPanic(t *testing.T) {
	realNvmlInit := nvmlInit
	defer func() { nvmlInit = realNvmlInit }()
	nvmlInit = func() nvml.Return { panic("library not found") }

	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)
	require.Equal(t, client.disable, true)
}

func TestNewNvmlClientDisabledOnLibraryNotFoundError(t *testing.T) {
	realNvmlInit := nvmlInit
	defer func() { nvmlInit = realNvmlInit }()
	nvmlInit = func() nvml.Return { return nvml.ERROR_LIBRARY_NOT_FOUND }

	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)
	require.Equal(t, client.disable, true)
}
