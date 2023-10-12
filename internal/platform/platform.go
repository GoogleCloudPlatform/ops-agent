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

package platform

import (
	"context"
	"fmt"
	"log"

	"github.com/shirou/gopsutil/host"
)

type Platform struct {
	Type               Type
	WindowsBuildNumber string
	WinlogV1Channels   []string
	HostInfo           *host.InfoStat
	HasNvidiaGpu       bool
}

type Type int

const (
	Linux Type = 1 << iota
	Windows
	All = Linux | Windows
)

func (p Platform) Is2016() bool {
	// https://en.wikipedia.org/wiki/List_of_Microsoft_Windows_versions#Server_versions
	return p.WindowsBuildNumber == "14393"
}

type platformKeyType struct{}

// platformKey is a singleton that is used as a Context key for retrieving the current platform from the context.Context.
var platformKey = platformKeyType{}

func (p Platform) TestContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, platformKey, p)
}

var detectedPlatform Platform = detect()

func FromContext(ctx context.Context) Platform {
	if opt := ctx.Value(platformKey); opt != nil {
		return opt.(Platform)
	}
	return detectedPlatform
}

func detect() Platform {
	info, err := host.Info()
	if err != nil {
		log.Fatalf("Failed to detect platform: %v", err)
	}
	p := Platform{
		HostInfo: info,
	}
	p.detectPlatform()
	return p
}

func (p Platform) Hostname() string {
	return p.HostInfo.Hostname
}

func (p Platform) Name() string {
	if p.Type == Windows {
		return "windows"
	} else if p.Type == Linux {
		return "linux"
	}
	panic(fmt.Sprintf("unknown type %v", p.Type))
}
