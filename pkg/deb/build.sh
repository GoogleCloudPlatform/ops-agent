#!/bin/bash

set -ex

. VERSION

# Add changelog entry
dch --create -b --package google-cloud-ops-agent -M \
  --distribution $(lsb_release -cs) \
  -v $PKG_VERSION~$(lsb_release -is | tr '[:upper:]' '[:lower:]')$(lsb_release -rs) \
  "Automated build"

# Build .debs
debuild --preserve-envvar JAVA_HOME -us -uc -sa
