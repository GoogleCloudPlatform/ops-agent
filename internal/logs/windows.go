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

//go:build windows
// +build windows

package logs

import (
	"fmt"

	"golang.org/x/sys/windows/svc/debug"
)

type WindowsServiceLogger struct {
	EventID uint32
	Logger  debug.Log
}

func (wsl WindowsServiceLogger) Infof(format string, v ...any) {
	wsl.Logger.Info(wsl.EventID, fmt.Sprintf(format, v...))
}

func (wsl WindowsServiceLogger) Warnf(format string, v ...any) {
	wsl.Logger.Warning(wsl.EventID, fmt.Sprintf(format, v...))
}

func (wsl WindowsServiceLogger) Errorf(format string, v ...any) {
	wsl.Logger.Error(wsl.EventID, fmt.Sprintf(format, v...))
}

func (wsl WindowsServiceLogger) Infow(msg string, keysAndValues ...any) {
	wsl.Logger.Info(wsl.EventID, msg)
}

func (wsl WindowsServiceLogger) Warnw(msg string, keysAndValues ...any) {
	wsl.Logger.Warning(wsl.EventID, msg)
}

func (wsl WindowsServiceLogger) Errorw(msg string, keysAndValues ...any) {
	wsl.Logger.Warning(wsl.EventID, msg)
}

func (wsl WindowsServiceLogger) Println(v ...any) {}
