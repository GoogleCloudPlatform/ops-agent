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

/*
A simple HTTP server - can be used to host static testing files in a folder
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

var (
	dir  = flag.String("dir", "./", "directory to serve")
	port = flag.String("port", "8000", "port to listen to")
)

func serve(dir, port string) error {
	dirHandler := http.FileServer(http.Dir(dir))
	http.Handle("/", dirHandler)
	log.Printf("Serving %s on port %s", dir, port)
	return http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}

func main() {
	flag.Parse()
	if err := serve(*dir, *port); err != nil {
		log.Fatalf("error running http server with dir=%s and port=%s: %s",
			*dir, *port, err)
	}
}
