# To edit this file, follow these instructions: go/sdi-integ-test#updating-the-test-runner-container.

FROM golang:1.22-bullseye

RUN curl -sSL https://sdk.cloud.google.com | bash

ENV PATH $PATH:/root/google-cloud-sdk/bin

# Needed for --max-run-duration, see b/227348032.
RUN gcloud components install beta

RUN go install gotest.tools/gotestsum@main

RUN apt-get update

RUN apt-get install --yes python3-yaml

# Install terraform
RUN apt-get update && apt-get install -y gnupg software-properties-common
RUN wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
RUN echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list
RUN apt-get update && apt-get install terraform

# Install go/grte.
COPY grte-runtimes.deb /install/grte-debs/
RUN dpkg -i /install/grte-debs/*.deb
