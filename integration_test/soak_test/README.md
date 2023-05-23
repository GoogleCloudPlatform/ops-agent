# Soak Test

This program launches a VM with a load generator running on it to simulate
load. To run it, replace `my_project` with your project (and adjust any other
parameters as desired) and run:

```
PROJECT=my_project \
  DISTRO=debian-11 \
  ZONES=us-central1-b \
  TTL=100m \
  LOG_SIZE_IN_BYTES=1000 \
  LOG_RATE=1000 \
  go run -tags=integration_test ./cmd/launcher
```

The VM will be cleaned up after 100 minutes (because `TTL=100m` above) if you
are using one of the projects set up for Googlers to use. For now, we do not
have automatic VM cleanup set up for other projects, but see b/227348032.
