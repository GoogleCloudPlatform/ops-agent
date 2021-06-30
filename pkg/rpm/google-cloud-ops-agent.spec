%if 0%{?sle_version} > 0
# we expect the distro suffix
%global dist .sles%(expr substr %{sle_version} 1 2)
%if %{sle_version} <= 12
# systemd macros have different names
%global systemd_post %{service_add_post %1}
%global systemd_preun %{service_del_preun %1}
%global systemd_postun %{service_del_postun %1}
%endif
%endif

Name: google-cloud-ops-agent
Version: %{package_version}
Release: 1%{?dist}
Summary: Google Cloud Ops Agent
Packager: Google Cloud Ops Agent <google-cloud-ops-agent@google.com>
License: ASL 2.0
%if 0%{?rhel} <= 7
BuildRequires: systemd
%else
BuildRequires: systemd-rpm-macros
%endif
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
build_distro=%{dist}
CODE_VERSION=%{version} BUILD_DISTRO=${build_distro#.} DESTDIR="%{buildroot}" ./build.sh

%files
%config %{_confdir}/config.yaml
%{_subagentdir}/fluent-bit/*
%{_subagentdir}/opentelemetry-collector/*
# We aren't using %{_libexecdir} here because that would be lib on some
# platforms, but the build.sh script hard-codes libexec.
%{_prefix}/libexec/google_cloud_ops_agent_engine
%{_unitdir}/%{name}*
%{_unitdir}-preset/*-%{name}*

%post
%systemd_post google-cloud-ops-agent.service
# rhel7 systemctl does not support --value
if [ "$(systemctl show -p LoadState google-cloud-ops-agent.target 2>/dev/null || :)" = "LoadState=loaded" ]; then
  systemctl stop google-cloud-ops-agent.target > /dev/null 2>&1 || :
  # If there was a .target installed, copy its enabledness
  if systemctl is-enabled google-cloud-ops-agent.target > /dev/null 2>&1; then
    systemctl enable google-cloud-ops-agent.service > /dev/null 2>&1 || :
    systemctl start google-cloud-ops-agent.service > /dev/null 2>&1 || :
  else
    systemctl disable google-cloud-ops-agent.service > /dev/null 2>&1 || :
  fi

  # Clean up old .target
  # RPM will remove the .target file after this scriplet runs
  systemctl --no-reload disable google-cloud-ops-agent.target > /dev/null 2>&1 || :
fi

if [ $1 -eq 1 ]; then  # Initial installation
  systemctl start google-cloud-ops-agent.service >/dev/null 2>&1 || :
fi

%preun
%systemd_preun google-cloud-ops-agent.service

%postun
%systemd_postun_with_restart google-cloud-ops-agent.service

%changelog
