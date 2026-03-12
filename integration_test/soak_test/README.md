# Soak Test

This program launches a VM with a load generator running on it to simulate
load. To run it, replace `my_project` with your project (and adjust any other
parameters as desired) and run:

```
PROJECT=my_project \
  DISTRO=debian-cloud:debian-11 \
  ZONES=us-central1-b \
  TTL=100m \
  LOG_SIZE_IN_BYTES=1000 \
  LOG_RATE=1000 \
  go run -tags=integration_test ./cmd/launcher
```

The VM will be cleaned up after 100 minutes (because `TTL=100m` above) if you
are using one of the projects set up for Googlers to use. For now, we do not
have automatic VM cleanup set up for other projects, but see b/227348032.

# Additional options

#### FEATURE : 

You can set "FEATURE" to run the Ops Agent with an specific "EXPERIMENTAL_FEATURES" flag enabled.

```
PROJECT=my_project \
  DISTRO=debian-cloud:debian-11 \
  ZONES=us-central1-b \
  TTL=100m \
  LOG_SIZE_IN_BYTES=1000 \
  LOG_RATE=1000 \
  FEATURE=otel_logging \
  go run -tags=integration_test ./cmd/launcher
```

#### TRANSFERS_BUCKET : 
You may need to set `TRANSFERS_BUCKET` with the correct permissions to be able to run in another project.

#### AGENT_PACKAGES_IN_GCS : 
You need to set `AGENT_PACKAGES_IN_GCS` to run the soak test with an specific Ops Agent build.