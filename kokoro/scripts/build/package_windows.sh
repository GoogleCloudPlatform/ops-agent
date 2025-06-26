#!/bin/bash

helpFunction()
{
   echo ""
   echo "Usage: $0 -a Arch"
   echo -e "\t-a Arch of the binary, must be x86_64 or x86"
   exit 1 # Exit script after printing help
}

Arch="x86_64"

if [[ -n $_LOUHI_TAG_NAME ]]
then
  # Example value: louhi/2.46.0/abcdef/windows/x86_64/start
  IFS="/"
  read -ra louhi_tag_components <<< "$_LOUHI_TAG_NAME"
  Arch="${louhi_tag_components[4]}"
else
  while getopts "a" opt
  do
     case "$opt" in
        a ) Arch="$OPTARG" ;;
        * ) helpFunction ;; # Print helpFunction in case parameter is non-existent
     esac
  done
fi

# Set GOARCH based on ARCH.
case $Arch in
    "x86_64")
      GoArch="amd64"
      ;;
    "x86")
      GoArch="386"
      ;;
    *)
      echo -e "\tArch must be set to one of: x86, x86_64"
      ;;
esac

git config --global --add safe.directory "$(pwd)"

go install -trimpath -ldflags="-s -w" github.com/google/googet/v2/goopack@latest

cd git/unified_agents

mkdir out

ls -la

mv "$KOKORO_GFILE_DIR"/result/out/* ./out

# replace pkg/goo/maint.ps1 with the signed version
mv "${KOKORO_GFILE_DIR}/result/pkg/goo/maint.ps1" ./out

mkdir "${KOKORO_ARTIFACTS_DIR}/result"

releaseName=$(awk -F "=" '/PKG_VERSION/ {print $2}' ./VERSION | tr -d '"')

"$GOPATH"/bin/goopack -output_dir "${KOKORO_ARTIFACTS_DIR}/result" \
  -var:PKG_VERSION="$releaseName" \
  -var:ARCH="$Arch" \
  -var:GOOS=windows \
  -var:GOARCH="$GoArch" \
  pkg/goo/google-cloud-ops-agent.goospec

if [[ -n $_LOUHI_TAG_NAME ]]
then
  # Example value: louhi/2.46.0/abcdef/windows/x86_64/start
  IFS="/"
  read -ra louhi_tag_components <<< "$_LOUHI_TAG_NAME"
  ver="${louhi_tag_components[1]}"
  ref="${louhi_tag_components[2]}"
  target="${louhi_tag_components[3]}"
  Arch="${louhi_tag_components[4]}"
  gcs_bucket="gs://${_STAGING_ARTIFACTS_PROJECT_ID}-ops-agent-releases/${ver}/${ref}/${target}/${Arch}/"
  gsutil cp "${KOKORO_ARTIFACTS_DIR}"/result/*.goo  "${gcs_bucket}"
fi
