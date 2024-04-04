# To edit this file, follow these instructions: go/sdi-integ-test#updating-the-test-runner-container.

FROM golang:1.22-bullseye

RUN curl -sSL https://sdk.cloud.google.com | bash

ENV PATH $PATH:/root/google-cloud-sdk/bin

# Needed for --max-run-duration, see b/227348032.
RUN gcloud components install beta

RUN go install gotest.tools/gotestsum@main

RUN apt-get update

RUN apt-get install --yes python3-yaml

# Install go/grte.
COPY grte-runtimes.deb /install/grte-debs/
RUN dpkg -i /install/grte-debs/*.deb
