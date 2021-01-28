#!/bin/bash

set -ex

. VERSION

cd pkg/deb

# Add changelog entry
dch --create -b --package google-cloud-ops-agent -M \
  --distribution $(lsb_release -cs) \
  -v $PKG_VERSION~$(lsb_release -is | tr A-Z a-z)$(lsb_release -rs) \
  "Automated build"

# Build .debs
debuild -us -uc -sa
