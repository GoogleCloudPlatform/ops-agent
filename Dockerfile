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
# or DOCKER_BUILDKIT=1 docker build -o /tmp/out . --target=buster
# Generated tarball(s) will end up in /tmp/out

FROM debian:bullseye AS bullseye-build

# TODO: Factor out the common code without rerunning apt-get on every debian and
# ubuntu build.

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config openjdk-11-jdk zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM debian:buster AS buster-build

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config openjdk-11-jdk zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM debian:stretch AS stretch-build

RUN set -x; \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl1.0-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config zip

ADD https://github.com/adoptium/temurin11-binaries/releases/download/jdk-11.0.13%2B8/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz
RUN set -xe; \
    mkdir -p /usr/local/java-11-openjdk && \
    tar -xf /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz -C /usr/local/java-11-openjdk --strip-components=1

ENV JAVA_HOME /usr/local/java-11-openjdk/

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:jammy AS jammy-build

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config openjdk-11-jdk zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:impish AS impish-build

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config openjdk-11-jdk zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:focal AS focal-build

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config openjdk-11-jdk zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM ubuntu:bionic AS bionic-build

RUN set -x; apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
    autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
    build-essential cmake bison flex file libsystemd-dev \
    devscripts cdbs pkg-config openjdk-11-jdk zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/deb/build.sh

FROM centos:7 AS centos7-build

RUN set -x; yum -y update && \
    yum -y install git systemd \
    autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
    gcc gcc-c++ make bison flex file systemd-devel zlib-devel gtest-devel rpm-build java-11-openjdk-devel \
    expect rpm-sign zip && \
    yum install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm && \
    yum install -y cmake3 && \
    ln -fs cmake3 /usr/bin/cmake

ENV JAVA_HOME /usr/lib/jvm/java-11-openjdk/

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

FROM rockylinux:8 AS centos8-build

RUN set -x; yum -y update && \
    dnf -y install 'dnf-command(config-manager)' && \
    yum config-manager --set-enabled powertools && \
    yum -y install git systemd \
    autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
    gcc gcc-c++ make cmake bison flex file systemd-devel zlib-devel gtest-devel rpm-build systemd-rpm-macros java-11-openjdk-devel \
    expect rpm-sign zip

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

# Use OpenSUSE Leap 42.3 to emulate SLES 12: https://en.opensuse.org/openSUSE:Build_Service_cross_distribution_howto#Detect_a_distribution_flavor_for_special_code
FROM opensuse/archive:42.3 AS sles12-build

RUN set -x; \
    # The 'OSS Update' repo signature is no longer valid, so verify the checksum instead.
    zypper --no-gpg-check refresh 'OSS Update' && \
    (echo 'b889b4bba03074cd66ef9c0184768f4816d4ccb1fa9ec2721c5583304c5f23d0  /var/cache/zypp/raw/OSS Update/repodata/repomd.xml' | sha256sum --check) && \
    zypper -n install git systemd autoconf automake flex libtool libcurl-devel libopenssl-devel libyajl-devel gcc gcc-c++ zlib-devel rpm-build expect cmake systemd-devel systemd-rpm-macros unzip zip && \
    # Remove expired root certificate.
    mv /var/lib/ca-certificates/pem/DST_Root_CA_X3.pem /etc/pki/trust/blacklist/ && \
    update-ca-certificates && \
    # Add home:Ledest:devel repo to install >3.4 bison
    zypper addrepo https://download.opensuse.org/repositories/home:Ledest:devel/openSUSE_Leap_42.3/home:Ledest:devel.repo && \
    zypper -n --gpg-auto-import-keys refresh && \
    zypper -n update && \
    # zypper/libcurl has a use-after-free bug that causes segfaults for particular download sequences.
    # If this bug happens to trigger in the future, adding a "zypper -n download" of a subset of the packages can avoid the segfault.
    zypper -n install bison>3.4 && \
    # Allow fluent-bit to find systemd
    ln -fs /usr/lib/systemd /lib/systemd

ADD https://github.com/adoptium/temurin11-binaries/releases/download/jdk-11.0.13%2B8/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz
RUN set -xe; \
    mkdir -p /usr/local/java-11-openjdk && \
    tar -xf /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz -C /usr/local/java-11-openjdk --strip-components=1

ENV JAVA_HOME /usr/local/java-11-openjdk/

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

FROM opensuse/leap:15.1 AS sles15-build

RUN set -x; zypper -n install git systemd autoconf automake flex libtool libcurl-devel libopenssl-devel libyajl-devel gcc gcc-c++ zlib-devel rpm-build expect cmake systemd-devel systemd-rpm-macros java-11-openjdk-devel unzip zip && \
    # Add home:ptrommler:formal repo to install >3.4 bison
    zypper addrepo https://download.opensuse.org/repositories/home:ptrommler:formal/openSUSE_Leap_15.1/home:ptrommler:formal.repo && \
    zypper -n --gpg-auto-import-keys refresh && \
    zypper -n update && \
    zypper -n install bison>3.4 && \
    # Allow fluent-bit to find systemd
    ln -fs /usr/lib/systemd /lib/systemd

ADD https://golang.org/dl/go1.17.linux-amd64.tar.gz /tmp/go1.17.linux-amd64.tar.gz
RUN set -xe; \
    tar -xf /tmp/go1.17.linux-amd64.tar.gz -C /usr/local

COPY . /work
WORKDIR /work
RUN ./pkg/rpm/build.sh

FROM scratch AS bullseye
COPY --from=bullseye-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-bullseye.tgz
COPY --from=bullseye-build /google-cloud-ops-agent*.deb /

FROM scratch AS buster
COPY --from=buster-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-buster.tgz
COPY --from=buster-build /google-cloud-ops-agent*.deb /

FROM scratch AS stretch
COPY --from=stretch-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-debian-stretch.tgz
COPY --from=stretch-build /google-cloud-ops-agent*.deb /

FROM scratch AS jammy
COPY --from=jammy-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-jammy.tgz
COPY --from=jammy-build /google-cloud-ops-agent*.deb /

FROM scratch AS impish
COPY --from=impish-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-impish.tgz
COPY --from=impish-build /google-cloud-ops-agent*.deb /

FROM scratch AS focal
COPY --from=focal-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-focal.tgz
COPY --from=focal-build /google-cloud-ops-agent*.deb /

FROM scratch AS bionic
COPY --from=bionic-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-ubuntu-bionic.tgz
COPY --from=bionic-build /google-cloud-ops-agent*.deb /

FROM scratch AS centos7
COPY --from=centos7-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-centos-7.tgz
COPY --from=centos7-build /google-cloud-ops-agent*.rpm /

FROM scratch AS centos8
COPY --from=centos8-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-centos-8.tgz
COPY --from=centos8-build /google-cloud-ops-agent*.rpm /

FROM scratch AS sles12
COPY --from=sles12-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-sles-12.tgz
COPY --from=sles12-build /google-cloud-ops-agent*.rpm /

FROM scratch AS sles15
COPY --from=sles15-build /tmp/google-cloud-ops-agent.tgz /google-cloud-ops-agent-sles-15.tgz
COPY --from=sles15-build /google-cloud-ops-agent*.rpm /

FROM scratch
COPY --from=bullseye /* /
COPY --from=buster /* /
COPY --from=stretch /* /
COPY --from=impish /* /
COPY --from=focal /* /
COPY --from=bionic /* /
COPY --from=centos7 /* /
COPY --from=centos8 /* /
COPY --from=sles12 /* /
COPY --from=sles15 /* /
