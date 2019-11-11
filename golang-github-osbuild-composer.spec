%global provider        github
%global provider_tld    com
%global project         osbuild
%global repo            osbuild-composer
%global provider_prefix %{provider}.%{provider_tld}/%{project}/%{repo}
%global goipath         %{provider_prefix}
%global commit
%global shortcommit     %(c=%{commit}; echo ${c:0:7})

Version:        5

%gometa

%global common_description %{expand:
An image building service based on osbuild
It is inspired by lorax-composer and exposes the same API.
As such, it is a drop-in replacement.
}

Name:           %{goname}
Release:        1%{?dist}
Summary:        An image building service based on osbuild

# Upstream license specification: Apache-2.0
License:        ASL 2.0
URL:            %{gourl}
Source0:        %{gosource}


BuildRequires:  %{?go_compiler:compiler(go-compiler)}%{!?go_compiler:golang}
BuildRequires:  systemd

Requires: systemd
Requires: osbuild

Provides: osbuild-composer

%description
%{common_description}

%prep
%forgeautosetup -p1

%build
GO_BUILD_PATH=$PWD/_build
install -m 0755 -vd $(dirname $GO_BUILD_PATH/src/%{goipath})
ln -fs $PWD $GO_BUILD_PATH/src/%{goipath}
cd $GO_BUILD_PATH/src/%{goipath}
install -m 0755 -vd _bin
export PATH=$PWD/_bin${PATH:+:$PATH}
export GOPATH=$GO_BUILD_PATH:%{gopath}
export GOFLAGS=-mod=vendor
%gobuild -o _bin/osbuild-composer %{goipath}/osbuild-composer
%gobuild -o _bin/osbuild-worker %{goipath}/osbuild-worker

%install
install -m 0755 -vd                                         %{buildroot}%{_libexecdir}/osbuild-composer
install -m 0755 -vp _bin/*                                  %{buildroot}%{_libexecdir}/osbuild-composer/
install -m 0755 -vp dnf-json                                %{buildroot}%{_libexecdir}/osbuild-composer/

install -m 0755 -vd                                         %{buildroot}%{_unitdir}
install -m 0644 -vp distribution/*.{service,socket}         %{buildroot}%{_unitdir}/

install -m 0755 -vd                                         %{buildroot}%{_sysusersdir}
install -m 0644 -vp distribution/osbuild-composer.conf      %{buildroot}%{_sysusersdir}/

install -m 0755 -vd                                         %{buildroot}%{_localstatedir}/cache/osbuild-composer/dnf-cache

%check
export GOFLAGS=-mod=vendor
export GOPATH=$PWD/_build:%{gopath}
%gotest ./...

%post
%systemd_post osbuild-composer.service osbuild-composer.socket osbuild-worker@.service

%preun
%systemd_preun osbuild-composer.service osbuild-composer.socket osbuild-worker@.service

%postun
%systemd_postun_with_restart osbuild-composer.service osbuild-composer.socket osbuild-worker@.service

%files
%license LICENSE
%doc README.md
%{_libexecdir}/osbuild-composer/
%{_unitdir}/*.{service,socket}
%{_sysusersdir}/osbuild-composer.conf

%changelog
* Sun Dec 1 11:00:00 CEST 2019 Ondrej Budai <obudai@redhat.com> - 4-1
- New upstream release.
* Fri Nov 15 11:00:00 CEST 2019 Ondrej Budai <obudai@redhat.com> - 3-1
- New upstream release.
* Wed Nov 13 15:14:00 CEST 2019 Ondrej Budai <obudai@redhat.com> - 2-2
- Fix specfile according to packaging guidelines.
* Mon Nov 11 13:23:00 CEST 2019 Tom Gundersen <teg@jklm.no> - 2-1
- First release.

