#!/usr/bin/env bash
# Installs the nfpm-built macOS packages with the real macOS installer and
# checks what ends up on disk. The packages are built on Linux by
# "task acceptance:macos:package".
#
# This installs into / with sudo: run it on a disposable macOS CI runner only.
set -euo pipefail

cd "$(dirname "$0")/../.."
# shellcheck source=testdata/acceptance/macos-lib.sh
source testdata/acceptance/macos-lib.sh

id=dev.goreleaser.nfpm.pkgtest
markers=/private/tmp/nfpm-pkg-test
support="/Library/Application Support/NfpmPkgTest"
unicode_support="/Library/Application Support/NfpmUnicode"
bin=/usr/local/bin/nfpm-pkg-test
arch="${NFPM_PKG_TEST_ARCH:?set NFPM_PKG_TEST_ARCH to the architecture the packages were built for}"

# mode, owner ID and group ID, the three things a package records per entry.
perm() {
  sudo stat -f '%Sp %u %g' "$1"
}

receipts() {
  pkgutil --pkgs | grep -c "^$1\$" || true
}

receipt_version() {
  pkgutil --pkg-info "$1" | awk '/^version:/ { print $2 }'
}

# leaf_fingerprint reads "pkgutil --check-signature" output and prints the
# SHA-256 fingerprint of the signing certificate, the first in the chain.
leaf_fingerprint() {
  awk '
    /SHA256 Fingerprint:/ { grab = 1; next }
    grab && /^[ 0-9A-F]+$/ { printf "%s", $0; next }
    grab { exit }
  ' | tr -d ' '
}

cleanup() {
  sudo rm -rf "$bin" "$bin-link" "$bin-rollback" "$bin-postfail" "$bin-signed" \
    /usr/local/bin/nfpm-unicode-* /private/etc/nfpm-pkg-test "$support" "$unicode_support" "$markers"
  for receipt in "$id" "$id.rollback" "$id.postfail" "$id.signed" \
    dev.goreleaser.nfpm.unicode.latin dev.goreleaser.nfpm.unicode.japanese dev.goreleaser.nfpm.unicode.cyrillic; do
    sudo pkgutil --forget "$receipt" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

require_host_arch "$arch"
cleanup

# The package must leave directories it does not own exactly as they were.
sudo mkdir -p /usr/local/bin
usr_local_before="$(perm /usr/local)"
usr_local_bin_before="$(perm /usr/local/bin)"

echo "== structure"
rm -rf dist/expanded
pkgutil --expand dist/nfpm-pkg-test-v1.pkg dist/expanded
[ -f dist/expanded/Distribution ] || fail "the package is not a product archive: no Distribution file"
case "$arch" in
  arm64) want_arch='hostArchitectures="arm64"' ;;
  amd64) want_arch='hostArchitectures="x86_64"' ;;
esac
grep -q "$want_arch" dist/expanded/Distribution || fail "Distribution does not declare $want_arch"
grep -q "$id" dist/expanded/Distribution || fail "Distribution does not name $id"
grep -q '<os-version min="11.0"/>' dist/expanded/Distribution || fail "Distribution does not carry the minimum macOS version"
payload="$(pkgutil --payload-files dist/nfpm-pkg-test-v1.pkg)"
for entry in ./usr/local/bin/nfpm-pkg-test ./private/etc/nfpm-pkg-test/app.conf; do
  grep -qx "$entry" <<<"$payload" || fail "payload does not list $entry"
done
if grep -qx './etc' <<<"$payload"; then
  fail "payload lists ./etc, which is a symbolic link on macOS"
fi
expect "package title" "$(installer -pkginfo -pkg dist/nfpm-pkg-test-v1.pkg)" "nfpm-pkg-test"

echo "== install 1.0.0"
sudo installer -pkg dist/nfpm-pkg-test-v1.pkg -target /
expect "installed program output" "$("$bin")" "nfpm-pkg-v1-test-ok"
expect "program mode and owner" "$(perm "$bin")" "-rwxr-xr-x 0 0"
expect "symlink target" "$(readlink "$bin-link")" "$bin"
expect "program output through the symlink" "$("$bin-link")" "nfpm-pkg-v1-test-ok"
expect "config mode and owner" "$(perm /private/etc/nfpm-pkg-test/app.conf)" "-rw-r----- 0 80"
[ -L /etc ] || fail "/etc is no longer a symbolic link"
expect "/etc link target" "$(readlink /etc)" "private/etc"
expect "explicit directory mode and owner" "$(perm "$support")" "drwxr-x--- 0 20"
sudo test -f "$support/héllo-世界.txt" || fail "the file with a non-ASCII name was not installed"
sudo test -f "$support/only-in-v1.conf" || fail "only-in-v1.conf was not installed"
expect "preinstall ran as root on the target volume" "$(cat "$markers/preinstall")" "uid=0 target=/"
expect "postinstall ran as root on the target volume" "$(cat "$markers/postinstall")" "uid=0 target=/"
expect "receipt count" "$(receipts "$id")" "1"
expect "receipt version" "$(receipt_version "$id")" "1.0.0"
expect "/usr/local is untouched" "$(perm /usr/local)" "$usr_local_before"
expect "/usr/local/bin is untouched" "$(perm /usr/local/bin)" "$usr_local_bin_before"

echo "== upgrade to 2.0.0-rc1"
sudo rm -f "$markers/preinstall" "$markers/postinstall"
sudo installer -pkg dist/nfpm-pkg-test-v2.pkg -target /
expect "upgraded program output" "$("$bin")" "nfpm-pkg-v2-test-ok"
expect "receipt count after upgrade" "$(receipts "$id")" "1"
expect "receipt version after upgrade" "$(receipt_version "$id")" "2.0.0-rc1"
expect "preinstall ran again" "$(cat "$markers/preinstall")" "uid=0 target=/"
expect "postinstall ran again" "$(cat "$markers/postinstall")" "uid=0 target=/"
# Documented behavior: the installer removes a file that a newer release of
# the same package no longer ships.
if sudo test -e "$support/only-in-v1.conf"; then
  fail "only-in-v1.conf survived the upgrade; update the documentation"
fi
sudo test -f "$support/héllo-世界.txt" || fail "a file both releases ship was removed by the upgrade"
expect "/usr/local is untouched by the upgrade" "$(perm /usr/local)" "$usr_local_before"

echo "== an older package installs over a newer one"
# Documented behavior: macOS has no version check. The same identifier is
# always an upgrade, whichever way the versions go.
sudo rm -f "$markers/preinstall" "$markers/postinstall"
sudo installer -pkg dist/nfpm-pkg-test-v1.pkg -target /
expect "program output after the downgrade" "$("$bin")" "nfpm-pkg-v1-test-ok"
expect "receipt count after the downgrade" "$(receipts "$id")" "1"
expect "receipt version after the downgrade" "$(receipt_version "$id")" "1.0.0"
expect "preinstall ran for the downgrade" "$(cat "$markers/preinstall")" "uid=0 target=/"
sudo test -f "$support/only-in-v1.conf" || fail "the downgrade did not bring only-in-v1.conf back"

echo "== a failing preinstall stops the installation"
sudo rm -f "$markers/preinstall-fail"
if sudo installer -pkg dist/nfpm-pkg-test-rollback.pkg -target /; then
  fail "the installer succeeded although preinstall exits 1"
fi
expect "the failing preinstall ran" "$(cat "$markers/preinstall-fail")" "PREINSTALL_FAILURE_EXECUTED"
[ ! -e "$bin-rollback" ] || fail "payload was installed although preinstall failed"
expect "receipt count after the failed install" "$(receipts "$id.rollback")" "0"

echo "== a failing postinstall fails the installation"
sudo rm -f "$markers/postinstall-fail"
if sudo installer -pkg dist/nfpm-pkg-test-postfail.pkg -target /; then
  fail "the installer succeeded although postinstall exits 1"
fi
expect "the failing postinstall ran" "$(cat "$markers/postinstall-fail")" "POSTINSTALL_FAILURE_EXECUTED"
# Documented behavior: postinstall runs after the files are in place, and
# macOS does not take them back when it fails.
[ -x "$bin-postfail" ] || fail "the payload is gone after a failed postinstall; update the documentation"

echo "== non-ASCII package names"
# The list is read on its own descriptor: the commands in the loop must not
# be able to consume it.
while IFS=$'\t' read -r -u 3 case_name product; do
  package="dist/unicode-$case_name.pkg"
  rm -rf dist/expanded-unicode
  pkgutil --expand "$package" dist/expanded-unicode
  grep -qF "<title>$product</title>" dist/expanded-unicode/Distribution ||
    fail "the installer title of $package is not '$product'"
  expect "$case_name package title" "$(installer -pkginfo -pkg "$package")" "$product"
  sudo rm -f "$markers/postinstall-unicode"
  sudo installer -pkg "$package" -target /
  expect "$case_name program output" "$("/usr/local/bin/nfpm-unicode-$case_name")" "nfpm-pkg-v1-test-ok"
  sudo test -f "$unicode_support/$product.conf" || fail "the file named after '$product' was not installed"
  expect "$case_name non-ASCII script literal" "$(cat "$markers/postinstall-unicode")" "héllo-世界-Привет"
  expect "$case_name receipt count" "$(receipts "dev.goreleaser.nfpm.unicode.$case_name")" "1"
done 3<dist/unicode-cases.tsv

echo "== signature: throwaway certificate"
unsigned="$(pkgutil --check-signature dist/nfpm-pkg-test-v1.pkg || true)"
grep -q "Status: no signature" <<<"$unsigned" || fail "the unsigned package reports a signature: $unsigned"
untrusted="$(pkgutil --check-signature dist/nfpm-pkg-test-signed.pkg || true)"
grep -q "Status: signed by untrusted certificate" <<<"$untrusted" ||
  fail "unexpected status for the package signed with the throwaway certificate: $untrusted"
# Prove the signature was made with the throwaway identity and no other.
expect "signing certificate" "$(leaf_fingerprint <<<"$untrusted")" "$(certificate_fingerprint dist/test.crt)"
# Documented behavior: macOS reads the signature and refuses to install a
# package signed with a certificate it does not trust.
if sudo installer -pkg dist/nfpm-pkg-test-signed.pkg -target /; then
  fail "the installer accepted a package signed with an untrusted certificate"
fi
[ ! -e "$bin-signed" ] || fail "payload of the untrusted package was installed"
expect "receipt count of the untrusted package" "$(receipts "$id.signed")" "0"

echo "== signature: Developer ID Installer"
if developer_id; then
  package=dist/developer-id/nfpm-pkg-test-signed.pkg
  # The system trust stack verifies the signature and the chain up to Apple.
  trusted="$(pkgutil --check-signature "$package")" || fail "macOS rejects the Developer ID signature made by nfpm: $trusted"
  grep -q "Status: signed by a developer certificate issued by Apple for distribution" <<<"$trusted" ||
    fail "unexpected signature status: $trusted"
  grep -q "Signed with a trusted timestamp" <<<"$trusted" || fail "the signature has no trusted timestamp: $trusted"
  for authority in "1. Developer ID Installer: " "Developer ID Certification Authority" "Apple Root CA"; do
    grep -qF "$authority" <<<"$trusted" || fail "the certificate chain lacks '$authority': $trusted"
  done
  expect "Developer ID signing certificate" "$(leaf_fingerprint <<<"$trusted")" "$(cat dist/developer-id/installer.sha256)"
  sudo installer -pkg "$package" -target /
  expect "Developer ID signed package program output" "$("$bin-signed")" "nfpm-pkg-v1-test-ok"
  expect "Developer ID signed package receipt count" "$(receipts "$id.signed")" "1"
fi

echo "all macOS package checks passed"
