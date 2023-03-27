# To edit this file, follow these instructions: go/sdi-integ-test#updating-the-test-runner-container.

FROM golang:1.20.1-bullseye

RUN curl -sSL https://sdk.cloud.google.com | bash

ENV PATH $PATH:/root/google-cloud-sdk/bin

RUN go install -v github.com/jstemmer/go-junit-report/v2@latest
