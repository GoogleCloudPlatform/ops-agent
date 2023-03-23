# Upgrade Go version

To upgrade the version of Go that the Ops Agent uses:

* Change the version of Go in `go.mod`
* Ensuring that you are using the same version as the new one stated in `go.mod`, run `make test` and verify whether any code updates are required
* Edit `dockerfiles/template` to download and extract the new version of Go
* Run `make compile_dockerfile`
* Run `make build` to locally ensure the new Dockerfile will build
* Submit a PR with a title that clearly states the Go upgrade and ensure all CI workflows pass