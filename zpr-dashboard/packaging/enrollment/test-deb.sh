#!/bin/sh
set -eu

if [ "$#" -ne 2 ] || [ "$(id -u)" -ne 0 ] || [ ! -f /.dockerenv ]; then
    echo "Run only in a DISPOSABLE Debian/Ubuntu container: sh test-deb.sh package.deb linux-test-binary" >&2
    exit 2
fi
package=$1
tests=$2
if id setup-test >/dev/null 2>&1 || dpkg-query -W -f='${Status}' zpr-enrollment-setup 2>/dev/null | grep -q 'install ok installed'; then
    echo "Requires a fresh disposable container without a setup-test user or installed wizard" >&2
    exit 2
fi
[ "$(dpkg-deb -f "$package" Package)" = zpr-enrollment-setup ]
[ "$(dpkg-deb -f "$package" Architecture)" = amd64 ]
dpkg -i "$package"
test -x /usr/bin/zpr-enrollment-setup
test -f /usr/share/applications/zpr-enrollment-setup.desktop
test ! -e /etc/zpr/enrollment-setup.json
test ! -e /lib/systemd/system/zpr-enrollment-setup.service
mkdir -p /etc/zpr
install -m 0644 /usr/share/zpr-enrollment-setup/setup.example.json /etc/zpr/enrollment-setup.json
useradd --create-home setup-test
su -s /bin/sh setup-test -c "'$tests' -test.run 'TestUserSetup|TestSetup' -test.v"
# A foreground wizard intentionally stays alive; timeout proves startup without
# leaving a listener or browser process behind after the test.
set +e
timeout 3 su -s /bin/sh setup-test -c '/usr/bin/zpr-enrollment-setup -config /etc/zpr/enrollment-setup.json -user-state' > /tmp/setup-output 2>&1
result=$?
set -e
test "$result" -eq 124
grep -q 'http://127.0.0.1:.*#session=' /tmp/setup-output
# HOME must resolve as the target desktop user.
# shellcheck disable=SC2016
su -s /bin/sh setup-test -c 'mkdir -m 700 "$HOME/.local/state/zpr-enrollment-development"; printf "preservation-test" > "$HOME/.local/state/zpr-enrollment-development/retention-test"'
dpkg -i "$package"
test -f /home/setup-test/.local/state/zpr-enrollment-development/retention-test
dpkg --purge zpr-enrollment-setup
test -f /home/setup-test/.local/state/zpr-enrollment-development/retention-test
test -f /etc/zpr/enrollment-setup.json
test ! -e /usr/bin/zpr-enrollment-setup
echo "Development package install, launch, reinstall, and purge-preservation checks passed."
