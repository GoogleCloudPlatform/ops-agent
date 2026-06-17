# Nvidia NVML Receiver

This receiver uses Nvidia's NVML [Go API](https://github.com/NVIDIA/go-nvml) to collect Nvidia GPU metrics.

## `gpu` Build Tag

When the `gpu` build tag is set, this receiver will be built with full functionality enabled. This requires `CGO` support in your build environment.

When the `gpu` build tag is not set, no functionality is built and the receiver factory will return an error when attempting to construct.

## `has_gpu` Build Tag

The `has_gpu` tag is for tests that will only work if there is definitely a GPU available. Running the tests in this package with this tag as well as `gpu` on will also build and run those particular tests along with everything else.
