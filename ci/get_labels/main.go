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
	"log"
	"net/http"
	"strings"
)

var prUrl = flag.String("url", "", "Url of pull request")

type RespLabel struct {
	Name string `json:"name"`
}

type RespLabelCollection []RespLabel

func (lc RespLabelCollection) String() string {
	var out string
	for _, label := range lc {
		out += label.Name + " "
	}
	return out
}

func main() {
	flag.Parse()

	labelCollector := LabelCollector{Client: &http.Client{}}
	labels, err := labelCollector.GetLabels(*prUrl)

	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(labels)
}

type LabelCollector struct {
	Client *http.Client
}

func (l *LabelCollector) GetLabels(prUrl string) (RespLabelCollection, error) {
	// We can only get the PR URL from Github Action, so we need to transform it into the issue URL
	labelsUrl := strings.Replace(prUrl, "/pulls/", "/issues/", 1)
	labelsUrl += "/labels"
	resp, err := l.Client.Get(labelsUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var responseLabels RespLabelCollection
	err = json.NewDecoder(resp.Body).Decode(&responseLabels)
	if err != nil {
		return nil, err
	}

	return responseLabels, nil
}
