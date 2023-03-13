// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type templateArguments struct {
	from_image        string
	target_name       string
	install_packages  string
	package_build     string
	tar_distro_name   string
	package_extension string
}

func applyTemplate(template string, arguments templateArguments) string {
	param_to_args := map[string]string{
		"{from_image}":        arguments.from_image,
		"{target_name}":       arguments.target_name,
		"{install_packages}":  arguments.install_packages,
		"{package_build}":     arguments.package_build,
		"{tar_distro_name}":   arguments.tar_distro_name,
		"{package_extension}": arguments.package_extension,
	}
	for param, arg := range param_to_args {
		template = strings.ReplaceAll(template, param, arg)
	}
	return template
}

var dockerfileArguments = []templateArguments{
	{
		from_image:  "centos:7",
		target_name: "centos7",
		install_packages: `RUN set -x; yum -y update && \
		yum -y install git systemd \
		autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
		gcc gcc-c++ make bison flex file systemd-devel zlib-devel gtest-devel rpm-build java-11-openjdk-devel \
		expect rpm-sign zip wget && \
		yum install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm && \
		yum install -y cmake3 && \
		ln -fs cmake3 /usr/bin/cmake && \
		# Download and install CMake manually (zypper only gives us CMake 3.5, but we need >= 3.12)
		wget https://github.com/Kitware/CMake/releases/download/v3.25.2/cmake-3.25.2-linux-x86_64.sh -O cmake.sh && \
		(echo '4d98de8d605da676e71a889dd94f80c76abb377fade2f21e3510e62ece1e1ada  cmake.sh' | sha256sum --check) && \
		bash cmake.sh --skip-license --prefix=/usr/local
		ENV JAVA_HOME /usr/lib/jvm/java-11-openjdk/`,
		package_build:     "RUN ./pkg/rpm/build.sh",
		tar_distro_name:   "centos-7",
		package_extension: "rpm",
	},
	{
		from_image:  "rockylinux:8",
		target_name: "centos8",
		install_packages: `RUN set -x; yum -y update && \
		dnf -y install 'dnf-command(config-manager)' && \
		yum config-manager --set-enabled powertools && \
		yum -y install git systemd \
		autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
		gcc gcc-c++ make cmake bison flex file systemd-devel zlib-devel gtest-devel rpm-build systemd-rpm-macros java-11-openjdk-devel \
		expect rpm-sign zip`,
		package_build:     "RUN ./pkg/rpm/build.sh",
		tar_distro_name:   "centos-8",
		package_extension: "rpm",
	},
	{
		from_image:  "rockylinux:9",
		target_name: "rockylinux9",
		install_packages: `RUN set -x; dnf -y update && \
		dnf -y install 'dnf-command(config-manager)' && \
		dnf config-manager --set-enabled crb && \
		dnf install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm && \
		dnf -y install git systemd \
		autoconf libtool libcurl-devel libtool-ltdl-devel openssl-devel yajl-devel \
		gcc gcc-c++ make cmake bison flex file systemd-devel zlib-devel gtest-devel rpm-build systemd-rpm-macros java-11-openjdk-devel \
		expect rpm-sign zip
	
		ENV JAVA_HOME /usr/lib/jvm/java-11-openjdk/`,
		package_build:     "RUN ./pkg/rpm/build.sh",
		tar_distro_name:   "rockylinux-9",
		package_extension: "rpm",
	},
	{
		from_image:  "debian:bullseye",
		target_name: "bullseye",
		install_packages: `RUN set -x; apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
		autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
		build-essential cmake bison flex file libsystemd-dev \
		devscripts cdbs pkg-config openjdk-11-jdk zip`,
		package_build:     "RUN ./pkg/deb/build.sh",
		tar_distro_name:   "debian-bullseye",
		package_extension: "deb",
	},
	{
		from_image:  "debian:buster",
		target_name: "buster",
		install_packages: `RUN set -x; apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
		autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
		build-essential cmake bison flex file libsystemd-dev \
		devscripts cdbs pkg-config openjdk-11-jdk zip`,
		package_build:     "RUN ./pkg/deb/build.sh",
		tar_distro_name:   "debian-buster",
		package_extension: "deb",
	},
	{
		from_image:  "debian:stretch",
		target_name: "stretch",
		install_packages: `RUN set -x; \
		(echo "deb http://ftp.debian.org/debian stretch-backports main" | tee /etc/apt/sources.list.d/backports.list) && \
		(echo "deb http://ftp.debian.org/debian stretch-backports-sloppy main" >> /etc/apt/sources.list.d/backports.list) && \
		apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
		autoconf libtool libcurl4-openssl-dev libltdl-dev libssl1.0-dev libyajl-dev \
		build-essential cmake/stretch-backports libuv1/stretch-backports \
		libarchive13/stretch-backports-sloppy bison flex file libsystemd-dev \
		devscripts cdbs pkg-config zip
		ADD https://github.com/adoptium/temurin11-binaries/releases/download/jdk-11.0.13%2B8/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz
		RUN set -xe; \
		mkdir -p /usr/local/java-11-openjdk && \
		tar -xf /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz -C /usr/local/java-11-openjdk --strip-components=1
		ENV JAVA_HOME /usr/local/java-11-openjdk/`,
		package_build:     "RUN ./pkg/deb/build.sh",
		tar_distro_name:   "debian-stretch",
		package_extension: "deb",
	},
	{
		// Use OpenSUSE Leap 42.3 to emulate SLES 12:
		//https://en.opensuse.org/openSUSE:Build_Service_cross_distribution_howto#Detect_a_distribution_flavor_for_special_code
		from_image:  "opensuse/archive:42.3",
		target_name: "sles12",
		install_packages: `RUN set -x; \
		# The 'OSS Update' repo signature is no longer valid, so verify the checksum instead.
		zypper --no-gpg-check refresh 'OSS Update' && \
		(echo 'b889b4bba03074cd66ef9c0184768f4816d4ccb1fa9ec2721c5583304c5f23d0  /var/cache/zypp/raw/OSS Update/repodata/repomd.xml' | sha256sum --check) && \
		zypper -n install git systemd autoconf automake flex libtool libcurl-devel libopenssl-devel libyajl-devel gcc gcc-c++ zlib-devel rpm-build expect cmake systemd-devel systemd-rpm-macros unzip zip wget && \
		# Remove expired root certificate.
		mv /var/lib/ca-certificates/pem/DST_Root_CA_X3.pem /etc/pki/trust/blacklist/ && \
		update-ca-certificates && \
		# Add home:odassau repo to install >3.4 bison
		zypper addrepo https://download.opensuse.org/repositories/home:/odassau/SLE_12_SP4/home:odassau.repo && \
		zypper -n --gpg-auto-import-keys refresh && \
		zypper -n update && \
		# zypper/libcurl has a use-after-free bug that causes segfaults for particular download sequences.
		# If this bug happens to trigger in the future, adding a "zypper -n download" of a subset of the packages can avoid the segfault.
		zypper -n install bison>3.4 && \
		# Allow fluent-bit to find systemd
		ln -fs /usr/lib/systemd /lib/systemd && \
		# Download and install CMake manually (zypper only gives us CMake 3.5, but we need >= 3.12)
		wget https://github.com/Kitware/CMake/releases/download/v3.25.2/cmake-3.25.2-linux-x86_64.sh -O cmake.sh && \
		(echo '4d98de8d605da676e71a889dd94f80c76abb377fade2f21e3510e62ece1e1ada  cmake.sh' | sha256sum --check) && \
		bash cmake.sh --skip-license --prefix=/usr/local
	
		ADD https://github.com/adoptium/temurin11-binaries/releases/download/jdk-11.0.13%2B8/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz
		RUN set -xe; \
			mkdir -p /usr/local/java-11-openjdk && \
			tar -xf /tmp/OpenJDK11U-jdk_x64_linux_hotspot_11.0.13_8.tar.gz -C /usr/local/java-11-openjdk --strip-components=1
		
		ENV JAVA_HOME /usr/local/java-11-openjdk/`,
		package_build:     "RUN ./pkg/rpm/build.sh",
		tar_distro_name:   "sles-12",
		package_extension: "rpm",
	},
	{
		from_image:  "opensuse/leap:15.1",
		target_name: "sles15",
		install_packages: `RUN set -x; zypper -n install git systemd autoconf automake flex libtool libcurl-devel libopenssl-devel libyajl-devel gcc gcc-c++ zlib-devel rpm-build expect cmake systemd-devel systemd-rpm-macros java-11-openjdk-devel unzip zip wget
		# Add agent-vendor.repo to install >3.4 bison
		RUN echo $'[google-cloud-monitoring-sles15-x86_64-test] \n\
		name=google-cloud-monitoring-sles15-x86_64-test \n\
		baseurl=https://packages.cloud.google.com/yum/repos/google-cloud-monitoring-sles15-x86_64-test-20221109-1 \n\
		enabled         = 1 \n\
		autorefresh     = 0 \n\
		repo_gpgcheck   = 0 \n\
		gpgcheck        = 0' > agent-vendor.repo
		RUN set -x; zypper addrepo agent-vendor.repo && \
			zypper -n --gpg-auto-import-keys refresh && \
			zypper -n update && \
			zypper -n install bison>3.4 && \
			# Allow fluent-bit to find systemd
			ln -fs /usr/lib/systemd /lib/systemd && \
			# Download and install CMake manually (zypper only gives us CMake 3.5, but we need >= 3.12)
			wget https://github.com/Kitware/CMake/releases/download/v3.25.2/cmake-3.25.2-linux-x86_64.sh -O cmake.sh && \
			(echo '4d98de8d605da676e71a889dd94f80c76abb377fade2f21e3510e62ece1e1ada  cmake.sh' | sha256sum --check) && \
			bash cmake.sh --skip-license --prefix=/usr/local`,
		package_build:     "RUN ./pkg/rpm/build.sh",
		tar_distro_name:   "sles-15",
		package_extension: "rpm",
	},
	{
		from_image:  "ubuntu:bionic-20220801",
		target_name: "bionic",
		install_packages: `RUN set -x; apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
		autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
		build-essential bison flex file libsystemd-dev \
		devscripts cdbs pkg-config openjdk-11-jdk zip wget && \
		# Download and install CMake manually (zypper only gives us CMake 3.5, but we need >= 3.12)
		wget https://github.com/Kitware/CMake/releases/download/v3.25.2/cmake-3.25.2-linux-x86_64.sh -O cmake.sh && \
		(echo '4d98de8d605da676e71a889dd94f80c76abb377fade2f21e3510e62ece1e1ada  cmake.sh' | sha256sum --check) && \
		bash cmake.sh --skip-license --prefix=/usr/local`,
		package_build:     "RUN ./pkg/deb/build.sh",
		tar_distro_name:   "ubuntu-bionic",
		package_extension: "deb",
	},
	{
		from_image:  "ubuntu:focal",
		target_name: "focal",
		install_packages: `RUN set -x; apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
		autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
		build-essential cmake bison flex file libsystemd-dev \
		devscripts cdbs pkg-config openjdk-11-jdk zip`,
		package_build:     "RUN ./pkg/deb/build.sh",
		tar_distro_name:   "ubuntu-focal",
		package_extension: "deb",
	},
	{
		from_image:  "ubuntu:jammy",
		target_name: "jammy",
		install_packages: `RUN set -x; apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get -y install git systemd \
		autoconf libtool libcurl4-openssl-dev libltdl-dev libssl-dev libyajl-dev \
		build-essential cmake bison flex file libsystemd-dev \
		devscripts cdbs pkg-config openjdk-11-jdk zip`,
		package_build:     "RUN ./pkg/deb/build.sh",
		tar_distro_name:   "ubuntu-jammy",
		package_extension: "deb",
	},
}

var dockerfileHeader = `# Copyright 2020 Google LLC
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

# This file was generated by dockerfiles/compile.go
# To re-generate, run from the repository root: go run ./dockerfiles

# Build as DOCKER_BUILDKIT=1 docker build -o /tmp/out .
# or DOCKER_BUILDKIT=1 docker build -o /tmp/out . --target=buster
# Generated tarball(s) will end up in /tmp/out`

func getDockerfileFooter() string {
	components := []string{"FROM scratch"}
	for _, arg := range dockerfileArguments {
		components = append(components, fmt.Sprintf("COPY --from=%s /* /", arg.target_name))
	}
	return strings.Join(components, "\n")
}

func getDockerfilePath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filepath.Dir(filename)), "Dockerfile")
}

func getTemplatePath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "template")
}

func getDockerfile() (string, error) {
	dat, err := os.ReadFile(getTemplatePath())
	if err != nil {
		return "", err
	}
	template := string(dat)
	components := []string{dockerfileHeader}
	for _, arg := range dockerfileArguments {
		components = append(components, applyTemplate(template, arg))
	}
	components = append(components, getDockerfileFooter())
	return strings.Join(components, "\n\n"), nil

}

func main() {
	dockerfile, err := getDockerfile()
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(getDockerfilePath(), []byte(dockerfile), 0644)
	if err != nil {
		panic(err)
	}

}
