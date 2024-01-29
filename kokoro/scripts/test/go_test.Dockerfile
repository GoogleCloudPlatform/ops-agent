# To edit this file, follow these instructions: go/sdi-integ-test#updating-the-test-runner-container.

FROM golang:1.19-bullseye

RUN curl -sSL https://sdk.cloud.google.com | bash

ENV PATH $PATH:/root/google-cloud-sdk/bin

# Needed for --max-run-duration, see b/227348032.
RUN gcloud components install beta

RUN go install -v github.com/jstemmer/go-junit-report/v2@latest

RUN apt-get update

RUN apt-get install --yes python3-yaml
