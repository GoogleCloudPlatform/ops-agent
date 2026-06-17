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

// adapted from StackOverflow (license CC BY-SA 3.0)
// https://stackoverflow.com/questions/27900849/unable-to-use-cublasxt
// https://creativecommons.org/licenses/by-sa/3.0/

// go:build gpu && has_gpu
//  +build gpu,has_gpu

#include "cublasXt.h"
#include "curand.h"
#include <stdio.h>

void FillMatrix(double *&x, long m, long n, double val) {
  x = new double[m * n];
  for (long i = 0; i < m; ++i)
    for (long j = 0; j < n; ++j)
      x[i * n + j] = val;
}

void PrintMatrix(double *x, long m, long n) {
  for (int i = 0; i < m; ++i) {
    for (int j = 0; j < n; ++j)
      printf("%lf ", x[i * n + j]);
    printf("\n");
  }
}

extern "C" bool SubmitCudaTestKernel() {
  cublasXtHandle_t xt_;
  cublasXtCreate(&xt_);
  int devices[1] = {0};
  if (cublasXtDeviceSelect(xt_, 1, devices) != CUBLAS_STATUS_SUCCESS)
    return false;

  double *A, *B, *C;
  long m = 10, n = 10, k = 20;

  FillMatrix(A, m, k, 0.2);
  FillMatrix(B, k, n, 0.3);
  FillMatrix(C, m, n, 0.0);

  double alpha = 1.0;
  double beta = 0.0;

  cublasXtDgemm(xt_, CUBLAS_OP_N, CUBLAS_OP_N, m, n, k, &alpha, A, m, B, k,
                &beta, C, m);

  cudaDeviceSynchronize();
  cublasXtDestroy(xt_);
  return true;
}
