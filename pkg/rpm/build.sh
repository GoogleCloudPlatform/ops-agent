#!/bin/bash

set -ex

. VERSION

# Build .rpms
rpmbuild --define "_source_filedigest_algorithm md5" \
  --define "package_version $PKG_VERSION" \
  --define "_sourcedir $(pwd)" \
  --define "_rpmdir $(pwd)" \
  -ba pkg/rpm/google-cloud-ops-agent.spec
cp $(uname -m)/google-cloud-ops-agent*.rpm /
