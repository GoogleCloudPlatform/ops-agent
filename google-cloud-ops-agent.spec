%if 0%{?sle_version} > 0
# we expect the distro suffix
%global dist .sles%(expr substr %{sle_version} 1 2)
%endif

Name: google-cloud-ops-agent
Version: %{package_version}
Release: 1%{?dist}
Summary: Google Cloud Ops Agent
Packager: Google Cloud Ops Agent <google-cloud-ops-agent@google.com>
License: ASL 2.0
Conflicts: stackdriver-agent, google-fluentd
BuildRoot: %{_tmppath}/%{name}-%{version}-%{release}-root

%description
The Google Cloud Ops Agent collects metrics and logs from the system.

%define _prefix /opt/%{name}
%define _confdir /etc/%{name}
%define _subagentdir %{_prefix}/subagents

%prep

%install
cd %{_sourcedir}
DESTDIR="%{buildroot}" ./build.sh

%files
%config %{_confdir}/config.yaml
%{_subagentdir}/fluent-bit/*
%{_subagentdir}/collectd/*
# We aren't using %{_libexecdir} here because that would be lib on some
# platforms, but the build.sh script hard-codes libexec.
%{_prefix}/libexec/generate_config
%{_unitdir}/%{name}*

%changelog
