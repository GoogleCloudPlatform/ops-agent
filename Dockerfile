# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build as DOCKER_BUILDKIT=1 docker build -o /tmp/out .
# Generated tarball(s) will end up in /tmp/out

FROM debian:buster AS buster

# TODO: Factor out the common code without rerunning apt-get on every build.

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install golang git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-deb.sh

FROM debian:stretch AS stretch

# TODO: Factor out the common code without rerunning apt-get on every build.

RUN echo "deb http://deb.debian.org/debian stretch-backports main" > /etc/apt/sources.list.d/backports.list && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y -t stretch-backports install golang && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl1.0-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-deb.sh

FROM ubuntu:focal AS focal

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install golang git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-deb.sh

FROM ubuntu:eoan AS eoan

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install golang git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-deb.sh

FROM ubuntu:bionic AS bionic

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install software-properties-common && \
    add-apt-repository ppa:longsleep/golang-backports && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install golang-go git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-deb.sh

FROM ubuntu:xenial AS xenial

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install software-properties-common && \
    add-apt-repository ppa:gophers/archive && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y -t xenial-backports install debhelper && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install golang-1.11-go git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work

# golang-1.11-go installs Go in a separate directory, so it needs to
# be added to the PATH for build.sh to find it. But debuild cleans PATH,
# so we need to tell it to re-add it after cleaning PATH.
RUN echo DEBUILD_PREPEND_PATH=/usr/lib/go-1.11/bin >> /etc/devscripts.conf && \
    ./build-deb.sh

FROM centos:7 AS centos7

RUN yum -y update && \
    yum -y install git systemd \
    autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
    gcc gcc-c++ make bison flex file systemd-devel zlib-devel gtest-devel rpm-build && \
    yum install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm && \
    yum install -y cmake3 golang && \
    ln -fs cmake3 /usr/bin/cmake

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-rpm.sh

FROM centos:8 AS centos8


RUN yum -y update && \
    dnf -y install 'dnf-command(config-manager)' && \
    yum config-manager --set-enabled PowerTools && \
    yum -y install golang git systemd \
    autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
    gcc gcc-c++ make cmake bison flex file systemd-devel zlib-devel gtest-devel rpm-build

ARG PKG_VERSION=0.1.0

COPY . /work
WORKDIR /work
RUN ./build-rpm.sh

FROM scratch
COPY --from=buster /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-buster.tgz
COPY --from=buster /google-cloud-ops-agent*.deb /

COPY --from=stretch /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-stretch.tgz
COPY --from=stretch /google-cloud-ops-agent*.deb /

COPY --from=focal /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-focal.tgz
COPY --from=focal /google-cloud-ops-agent*.deb /

COPY --from=eoan /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-eoan.tgz
COPY --from=eoan /google-cloud-ops-agent*.deb /

COPY --from=bionic /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-bionic.tgz
COPY --from=bionic /google-cloud-ops-agent*.deb /

COPY --from=xenial /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-xenial.tgz
COPY --from=xenial /google-cloud-ops-agent*.deb /

COPY --from=centos7 /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-centos-7.tgz
COPY --from=centos7 /google-cloud-ops-agent*.rpm /

COPY --from=centos8 /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-centos-8.tgz
COPY --from=centos8 /google-cloud-ops-agent*.rpm /
