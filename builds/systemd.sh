#!/bin/bash
set -x -e
DESTDIR=$1

sysconfdir=/etc
systemdsystemunitdir=$(pkg-config systemd --variable=systemdsystemunitdir)
systemdsystempresetdir=$(pkg-config systemd --variable=systemdsystempresetdir)

function install_unit() {
# $1 = source path; $2 = destination path relative to the unit directory
sed "s|@PREFIX@|/opt/google-cloud-ops-agent|g; s|@SYSCONFDIR@|$sysconfdir|g" "$1" > "$DESTDIR$systemdsystemunitdir/$2"
}
mkdir -p "$DESTDIR$systemdsystemunitdir"
for f in systemd/*.service; do
install_unit "$f" "$(basename "$f")"
done
if [ "$(systemctl --version | grep -Po '^systemd \K\d+')" -lt 240 ]; then
for d in systemd/*.service.d; do
    mkdir "$DESTDIR$systemdsystemunitdir/$(basename "$d")"
    for f in "$d"/*.conf; do
    install_unit "$f" "$(basename "$d")/$(basename "$f")"
    done
done
fi
mkdir -p "$DESTDIR$systemdsystempresetdir"
for f in systemd/*.preset; do
cp "$f" "$DESTDIR$systemdsystempresetdir/$(basename "$f")"
done