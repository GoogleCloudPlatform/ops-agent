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
	"fmt"
	"unsafe"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func (m *deviceMetric) setFloat64(val float64) {
	*(*float64)(unsafe.Pointer(&m.value[0])) = val
}

func (m *deviceMetric) asFloat64() float64 {
	return *(*float64)(unsafe.Pointer(&m.value[0]))
}

func (m *deviceMetric) setInt64(val int64) {
	*(*int64)(unsafe.Pointer(&m.value[0])) = val
}

func (m *deviceMetric) asInt64() int64 {
	return *(*int64)(unsafe.Pointer(&m.value[0]))
}

func nvmlSampleAsFloat64(value [8]byte, nvmlType nvml.ValueType) (float64, error) {
	switch nvmlType {
	case nvml.VALUE_TYPE_DOUBLE:
		return *(*float64)(unsafe.Pointer(&value[0])), nil
	case nvml.VALUE_TYPE_UNSIGNED_INT:
		return (float64)(*(*uint32)(unsafe.Pointer(&value[0]))), nil
	case nvml.VALUE_TYPE_UNSIGNED_LONG:
		return (float64)(*(*uint64)(unsafe.Pointer(&value[0]))), nil
	case nvml.VALUE_TYPE_UNSIGNED_LONG_LONG:
		return (float64)(*(*uint64)(unsafe.Pointer(&value[0]))), nil
	case nvml.VALUE_TYPE_SIGNED_LONG_LONG:
		return (float64)(*(*int64)(unsafe.Pointer(&value[0]))), nil
	}

	return 0.0, fmt.Errorf("Unable to convert Nvidia NVML sample value to float64")
}
