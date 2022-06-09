#!/bin/bash

set -e
set -o pipefail

KEY_NAME="Google Cloud Packages RPM Signing Key <gc-team@google.com>"
KEY_PATH="/signing/signing-key"

gpg --import "$KEY_PATH"
GPG_NAME="$KEY_NAME" expect /signing/sign-helper.exp -- "$@"

