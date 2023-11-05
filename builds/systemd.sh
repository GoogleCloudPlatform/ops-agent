# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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