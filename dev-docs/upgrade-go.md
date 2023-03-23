# Upgrade Go version

To upgrade the version of Go that the Ops Agent uses, update the version in the following places:

* `go.mod` version restriction
* `dockerfiles/template` which downloads and installs Go and is compiled into the main Dockerfile
* `Dockerfile.windows` which downloads and runs the Go MSI

Once you have updated the Go version in the following places, verify the new version works:

* Ensure that your local Go version is the same as the new one in `go.mod` 
* Run `make test` and verify whether any code updates are required
* Run `make compile_dockerfile`
* Run `make build` to ensure the new Dockerfile will build
* Submit a PR with a title that clearly states the Go upgrade and ensure all Build and Integration Test CI workflows pass