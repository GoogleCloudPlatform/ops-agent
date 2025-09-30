// Created this go.mod to encapsulate all Go code from the submodules and avoid interaction with top-level submodule.
// For example, this enables "go test ./..." and "go mod tidy" commands from top-level module to avoid any interaction
// with the code in "/submodules" folder.
