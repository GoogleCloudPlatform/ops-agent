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

// Note: The DCGM library should be loaded to find the symbols

//go:build gpu && has_gpu
// +build gpu,has_gpu

package testprofilepause

/*
#include <stdint.h>
typedef uintptr_t dcgmHandle_t;
typedef enum dcgmReturn_enum { DCGM_ST_OK = 0, DCGM_ST_NOT_SUPPORTED = -6 } dcgmReturn_t;
dcgmReturn_t dcgmProfPause(dcgmHandle_t pDcgmHandle);
dcgmReturn_t dcgmProfResume(dcgmHandle_t pDcgmHandle);
const char *errorString(dcgmReturn_t result);
*/
import "C"
import (
	"fmt"
	_ "unsafe"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

type dcgmHandle struct{ handle C.dcgmHandle_t }

//go:linkname handle github.com/NVIDIA/go-dcgm/pkg/dcgm.handle
var handle dcgmHandle

var errorMap = map[C.dcgmReturn_t]error{
	C.DCGM_ST_OK: nil,
}

func errorString(result C.dcgmReturn_t) error {
	if err, ok := errorMap[result]; ok {
		return err
	}
	msg := C.GoString(C.errorString(result))
	err := fmt.Errorf("%v", msg)
	errorMap[result] = err
	return err
}

var FeatureNotSupportedError error
var initErrors = func() {
	if FeatureNotSupportedError == nil {
		FeatureNotSupportedError = errorString(C.DCGM_ST_NOT_SUPPORTED)
	}
}

func PauseProfilingMetrics(endpoint string) error {
	initErrors()
	cleanup, err := dcgm.Init(dcgm.Standalone, endpoint, "0")
	if err != nil {
		return err
	}
	defer cleanup()
	result := C.dcgmProfPause(handle.handle)
	err = errorString(result)
	if err != nil {
		fmt.Printf("CUDA version %d\n", dcgm.DCGM_FI_CUDA_DRIVER_VERSION)
		fmt.Printf("Failed to pause profiling (%v)\n", err)
	}
	return err
}

func ResumeProfilingMetrics(endpoint string) error {
	initErrors()
	cleanup, err := dcgm.Init(dcgm.Standalone, endpoint, "0")
	if err != nil {
		return err
	}
	defer cleanup()
	result := C.dcgmProfResume(handle.handle)
	err = errorString(result)
	if err != nil {
		fmt.Printf("Failed to resume profiling (%v)\n", err)
	}
	return err
}
