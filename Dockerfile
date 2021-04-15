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

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM debian:stretch AS stretch

# TODO: Factor out the common code without rerunning apt-get on every build.

RUN set -x; \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl1.0-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:focal AS focal

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:bionic AS bionic

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:xenial AS xenial

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y -t xenial-backports install debhelper && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work

RUN ./pkg/deb/build.sh

FROM centos:7 AS centos7

RUN set -x; yum -y update && \
    yum -y install git systemd \
    autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
    gcc gcc-c++ make bison flex file systemd-devel zlib-devel gtest-devel rpm-build \
    expect rpm-sign && \
    yum install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm && \
    yum install -y cmake3 && \
    ln -fs cmake3 /usr/bin/cmake

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

FROM centos:8 AS centos8

RUN set -x; yum -y update && \
    dnf -y install 'dnf-command(config-manager)' && \
    yum config-manager --set-enabled powertools && \
    yum -y install git systemd \
    autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
    gcc gcc-c++ make cmake bison flex file systemd-devel zlib-devel gtest-devel rpm-build systemd-rpm-macros \
    expect rpm-sign

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

# Use OpenSUSE Leap 42.3 to emulate SLES 12: https://en.opensuse.org/openSUSE:Build_Service_cross_distribution_howto#Detect_a_distribution_flavor_for_special_code
FROM opensuse/leap:42.3 as sles12

RUN set -x; zypper -n install git systemd autoconf automake flex libtool libcurl-devel libopenssl-devel libyajl-devel gcc gcc-c++ zlib-devel rpm-build expect cmake systemd-devel systemd-rpm-macros \
# Add home:Ledest:devel repo to install >3.4 bison
&& zypper addrepo https://download.opensuse.org/repositories/home:Ledest:devel/openSUSE_Leap_42.3/home:Ledest:devel.repo \
&& zypper -n --gpg-auto-import-keys refresh \
&& zypper -n update \
# zypper/libcurl has a use-after-free bug that causes segfaults for particular download sequences.
# If this bug happens to trigger in the future, adding a "zypper -n download" of a subset of the packages can avoid the segfault.
&& zypper -n install bison>3.4 \
# Allow fluent-bit to find systemd
&& ln -fs /usr/lib/systemd /lib/systemd

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

FROM opensuse/leap:15.1 as sles15

RUN set -x; zypper -n install git systemd autoconf automake flex libtool libcurl-devel libopenssl-devel libyajl-devel gcc gcc-c++ zlib-devel rpm-build expect cmake systemd-devel systemd-rpm-macros \
# Add home:ptrommler:formal repo to install >3.4 bison
&& zypper addrepo https://download.opensuse.org/repositories/home:ptrommler:formal/openSUSE_Leap_15.1/home:ptrommler:formal.repo \
&& zypper -n --gpg-auto-import-keys refresh \
&& zypper -n update \
&& zypper -n install bison>3.4 \
# Allow fluent-bit to find systemd
&& ln -fs /usr/lib/systemd /lib/systemd

ADD https://golang.org/dl/go1.16.3.linux-amd64.tar.gz /tmp/go1.16.3.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.16.3.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

FROM scratch
COPY --from=buster /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-buster.tgz
COPY --from=buster /google-cloud-ops-agent*.deb /

COPY --from=stretch /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-stretch.tgz
COPY --from=stretch /google-cloud-ops-agent*.deb /

COPY --from=focal /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-focal.tgz
COPY --from=focal /google-cloud-ops-agent*.deb /

COPY --from=bionic /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-bionic.tgz
COPY --from=bionic /google-cloud-ops-agent*.deb /

COPY --from=xenial /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-xenial.tgz
COPY --from=xenial /google-cloud-ops-agent*.deb /

COPY --from=centos7 /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-centos-7.tgz
COPY --from=centos7 /google-cloud-ops-agent*.rpm /

COPY --from=centos8 /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-centos-8.tgz
COPY --from=centos8 /google-cloud-ops-agent*.rpm /

COPY --from=sles12 /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-sles-12.tgz
COPY --from=sles12 /google-cloud-ops-agent*.rpm /

COPY --from=sles15 /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-sles-15.tgz
COPY --from=sles15 /google-cloud-ops-agent*.rpm /
