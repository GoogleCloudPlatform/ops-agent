// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package command

import (
	"errors"
	"fmt"
	"log"
	"os"
)

var errCommandNotFound = errors.New("command not found")

// Command is a type that parses args from the command
// line and can run. Intended to be registered by a
// Runner.
type Command interface {
	Run() error
	ParseArgs(args []string) error
}

// Runner has a set of registered named commands that
// can be requested by a caller.
type Runner struct {
	// Dir is set if the working directory of the commands
	// should be changed before running.
	Dir string

	commands map[string]Command
}

func NewRunner() *Runner {
	return &Runner{
		commands: make(map[string]Command),
	}
}

func (cs *Runner) Register(name string, cmd Command) {
	if _, exists := cs.commands[name]; exists {
		log.Panicf(`developer error: command with name "%s" already exists`, name)
	}
	cs.commands[name] = cmd
}

func (cs *Runner) Run(name string) (err error) {
	if cs.Dir != "" {
		originalDir, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := os.Chdir(cs.Dir); err != nil {
			return err
		}
		defer func() {
			err = os.Chdir(originalDir)
		}()
	}

	cmd, ok := cs.commands[name]
	if !ok {
		return fmt.Errorf("%w: %s", errCommandNotFound, name)
	}
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}
	if err := cmd.ParseArgs(args); err != nil {
		return err
	}
	return cmd.Run()
}
