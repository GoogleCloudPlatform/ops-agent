// Copyright 2020 Google LLC
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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
)

var prUrl = flag.String("url", "", "Url of pull request")

func main() {
	flag.Parse()
	labelsUrl := strings.Replace(*prUrl, "/pulls/", "/issues/", 1)
	labelsUrl += "/labels"
	resp, err := http.Get(labelsUrl)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	type Labels struct {
		Name string
	}

	var responseLabels []Labels
	err = json.NewDecoder(resp.Body).Decode(&responseLabels)
	if err != nil {
		panic(err)
	}

	var output string
	for _, label := range responseLabels {
		output += label.Name + " "
	}

	fmt.Println(output)
}
