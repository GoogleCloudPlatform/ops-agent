#!/bin/bash

set -ex

# Add changelog entry
DEBEMAIL="google-cloud-ops-agent@google.com" DEBFULLNAME="Google Cloud Ops Agent" dch --create -b --package google-cloud-ops-agent \
  --distribution `lsb_release -cs` -v $PKG_VERSION~$(lsb_release -is | tr A-Z a-z)$(lsb_release -rs) "Automated build"

# Build .debs
debuild -us -uc -sa
