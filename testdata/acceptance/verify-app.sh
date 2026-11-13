#!/usr/bin/env bash
# Unpacks, mounts and launches the nfpm-built macOS application bundles and
# disk images, and checks their signatures with codesign. The packages are
# built on Linux by "task acceptance:macos:package".
#
# Nothing here needs root or leaves anything behind outside dist/.
set -euo pipefail

cd "$(dirname "$0")/../.."
# shellcheck source=testdata/acceptance/macos-lib.sh
source testdata/acceptance/macos-lib.sh

name="Nfpm App Test"
id=dev.goreleaser.nfpm.apptest
work=dist/app-verify
arch="${NFPM_PKG_TEST_ARCH:?set NFPM_PKG_TEST_ARCH to the architecture the packages were built for}"
mounted=""

plist() {
  /usr/libexec/PlistBuddy -c "Print :$2" "$1/Contents/Info.plist"
}

cleanup() {
  if [ -n "$mounted" ]; then
    hdiutil detach "$mounted" -force >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

attach() {
  mounted="$work/mount-$1"
  mkdir -p "$mounted"
  hdiutil attach -nobrowse -readonly -mountpoint "$mounted" "$2" >/dev/null
}

detach() {
  hdiutil detach "$mounted" >/dev/null
  mounted=""
}

# composed prints its input in composed Unicode form. A disk image stores
# names decomposed, as macOS volumes do, so "é" comes back as "e" followed by
# a combining accent: the same name, in different bytes.
composed() {
  iconv -f UTF-8-MAC -t UTF-8
}

volume_name() {
  diskutil info "$mounted" | awk -F': *' '/Volume Name/ { print $2 }' | composed
}

# launch starts a bundle the way a user does, through Launch Services, which
# must accept the bundle and run its main executable. The test program prints
# and exits at once, sooner than "open -W" can start waiting for it, so the
# output is waited for instead.
launch() {
  rm -f "$work/launch.out"
  open -n --stdout "$work/launch.out" "$1"
  for _ in $(seq 1 60); do
    [ -s "$work/launch.out" ] && break
    sleep 1
  done
  cat "$work/launch.out"
}

# check_bundle verifies the test bundle wherever it is: unpacked from the zip
# or on the mounted disk image.
check_bundle() {
  local app="$1"
  [ -d "$app" ] || fail "$app is missing"
  plutil -lint "$app/Contents/Info.plist" >/dev/null || fail "Info.plist is not a valid property list"
  expect "bundle identifier" "$(plist "$app" CFBundleIdentifier)" "$id"
  expect "bundle name" "$(plist "$app" CFBundleName)" "$name"
  expect "bundle executable" "$(plist "$app" CFBundleExecutable)" "nfpm-app-test"
  expect "version shown to the user" "$(plist "$app" CFBundleShortVersionString)" "1.0.0-rc1"
  expect "build version" "$(plist "$app" CFBundleVersion)" "1.0.0"
  expect "minimum macOS version" "$(plist "$app" LSMinimumSystemVersion)" "11.0"
  expect "extra Info.plist key" "$(plist "$app" LSUIElement)" "true"
  expect "PkgInfo" "$(cat "$app/Contents/PkgInfo")" "APPL????"

  [ -x "$app/Contents/MacOS/nfpm-app-test" ] || fail "the main executable lost its executable bit"
  expect "main program output" "$("$app/Contents/MacOS/nfpm-app-test")" "nfpm-pkg-v1-test-ok"
  expect "second program output" "$("$app/Contents/MacOS/helper")" "nfpm-pkg-v2-test-ok"
  [ -L "$app/Contents/Resources/launcher" ] || fail "the symbolic link became a regular file"
  expect "symlink target" "$(readlink "$app/Contents/Resources/launcher")" "../MacOS/nfpm-app-test"
  expect "program output through the symlink" "$("$app/Contents/Resources/launcher")" "nfpm-pkg-v1-test-ok"
  expect "file with a non-ASCII name" "$(cat "$app/Contents/Resources/héllo-世界.txt")" "hello from nfpm"
  expect "output when launched through Launch Services" "$(launch "$app")" "nfpm-pkg-v1-test-ok"
}

# signing_certificate prints the fingerprint of the certificate a bundle or
# disk image was signed with.
signing_certificate() {
  rm -f "$work"/leaf-*
  codesign --display --extract-certificates="$work/leaf-" "$1" 2>/dev/null
  certificate_fingerprint "$work/leaf-0"
}

# check_signed verifies a bundle signed by nfpm: codesign accepts the whole
# bundle, every program in it is signed, its resources are sealed, and the
# signature was made with the expected certificate and no other.
check_signed() {
  local app="$1" fingerprint="$2"
  codesign --verify --deep --strict --verbose=2 "$app" || fail "codesign rejects the bundle signed by nfpm"
  codesign --verify --strict "$app/Contents/MacOS/helper" || fail "the second program is not signed"
  local details
  details="$(codesign --display --verbose=4 "$app" 2>&1)"
  grep -q "^Identifier=$id\$" <<<"$details" || fail "the signature does not carry the bundle identifier: $details"
  grep -q "Sealed Resources version=2" <<<"$details" || fail "the bundle's resources are not sealed: $details"
  expect "bundle signing certificate" "$(signing_certificate "$app")" "$fingerprint"
}

# check_developer_id verifies what only a real Developer ID signature has:
# the chain up to Apple, a secure timestamp and, for programs, the hardened
# runtime.
check_developer_id() {
  local target="$1" kind="$2"
  local details
  details="$(codesign --display --verbose=4 "$target" 2>&1)"
  for authority in "Authority=Developer ID Application: " "Authority=Developer ID Certification Authority" "Authority=Apple Root CA"; do
    grep -qF "$authority" <<<"$details" || fail "the certificate chain of $target lacks '$authority': $details"
  done
  grep -q "^Timestamp=" <<<"$details" || fail "$target has no secure timestamp: $details"
  grep -Eq "^TeamIdentifier=[A-Z0-9]{10}\$" <<<"$details" || fail "$target has no team identifier: $details"
  if [ "$kind" = bundle ]; then
    grep -Eq "flags=0x[0-9a-f]+\(.*runtime.*\)" <<<"$details" || fail "$target lacks the hardened runtime: $details"
  fi
}

require_host_arch "$arch"
rm -rf "$work"
mkdir -p "$work"
throwaway="$(certificate_fingerprint dist/test.crt)"

echo "== app: unsigned zip"
ditto -x -k dist/nfpm-app-test.app.zip "$work/unsigned"
expect "archive content" "$(ls "$work/unsigned")" "$name.app"
check_bundle "$work/unsigned/$name.app"

echo "== app: zip signed with the throwaway certificate"
ditto -x -k dist/nfpm-app-test-signed.app.zip "$work/signed"
check_bundle "$work/signed/$name.app"
check_signed "$work/signed/$name.app" "$throwaway"

echo "== app: a changed resource breaks the seal"
cp -R "$work/signed/$name.app" "$work/tampered.app"
echo changed >"$work/tampered.app/Contents/Resources/héllo-世界.txt"
if codesign --verify --deep --strict "$work/tampered.app" 2>/dev/null; then
  fail "codesign accepts a bundle whose resource was changed after signing"
fi
echo "ok: codesign rejects the changed bundle"

echo "== dmg: unsigned image"
hdiutil verify dist/nfpm-app-test.dmg >/dev/null || fail "hdiutil rejects the image"
unsigned="$(codesign --verify dist/nfpm-app-test.dmg 2>&1 || true)"
grep -q "not signed at all" <<<"$unsigned" || fail "the unsigned image reports a signature: $unsigned"
attach unsigned dist/nfpm-app-test.dmg
expect "volume name" "$(volume_name)" "$name"
expect "volume content" "$(ls "$mounted" | sort | tr '\n' ',')" "Applications,$name.app,"
[ -L "$mounted/Applications" ] || fail "Applications is not a symbolic link"
expect "Applications link target" "$(readlink "$mounted/Applications")" "/Applications"
check_bundle "$mounted/$name.app"
detach

echo "== dmg: image signed with the throwaway certificate"
hdiutil verify dist/nfpm-app-test-signed.dmg >/dev/null || fail "hdiutil rejects the signed image"
codesign --verify --strict --verbose=2 dist/nfpm-app-test-signed.dmg || fail "codesign rejects the image signed by nfpm"
expect "image signing certificate" "$(signing_certificate dist/nfpm-app-test-signed.dmg)" "$throwaway"
attach signed dist/nfpm-app-test-signed.dmg
check_bundle "$mounted/$name.app"
check_signed "$mounted/$name.app" "$throwaway"
detach

echo "== non-ASCII bundle names"
# The list is read on its own descriptor: the commands in the loop must not
# be able to consume it.
while IFS=$'\t' read -r -u 3 case_name product; do
  ditto -x -k "dist/unicode-$case_name.app.zip" "$work/unicode-$case_name"
  app="$work/unicode-$case_name/$product.app"
  [ -d "$app" ] || fail "the bundle of dist/unicode-$case_name.app.zip is not named '$product.app'"
  expect "$case_name bundle name" "$(plist "$app" CFBundleName)" "$product"
  expect "$case_name program output" "$("$app/Contents/MacOS/$product")" "nfpm-pkg-v1-test-ok"
  expect "$case_name output when launched through Launch Services" "$(launch "$app")" "nfpm-pkg-v1-test-ok"

  hdiutil verify "dist/unicode-$case_name.dmg" >/dev/null || fail "hdiutil rejects dist/unicode-$case_name.dmg"
  attach "unicode-$case_name" "dist/unicode-$case_name.dmg"
  expect "$case_name volume name" "$(volume_name)" "$(composed <<<"$product")"
  [ -d "$mounted/$product.app" ] || fail "the image does not hold '$product.app'"
  expect "$case_name output when launched from the image" "$(launch "$mounted/$product.app")" "nfpm-pkg-v1-test-ok"
  detach
done 3<dist/unicode-cases.tsv

echo "== signature: Developer ID Application"
if developer_id; then
  application="$(cat dist/developer-id/application.sha256)"

  ditto -x -k dist/developer-id/nfpm-app-test-signed.app.zip "$work/developer-id"
  check_bundle "$work/developer-id/$name.app"
  check_signed "$work/developer-id/$name.app" "$application"
  check_developer_id "$work/developer-id/$name.app" bundle
  check_developer_id "$work/developer-id/$name.app/Contents/MacOS/helper" bundle

  image=dist/developer-id/nfpm-app-test-signed.dmg
  hdiutil verify "$image" >/dev/null || fail "hdiutil rejects the Developer ID signed image"
  codesign --verify --strict --verbose=2 "$image" || fail "codesign rejects the Developer ID signed image"
  expect "Developer ID image signing certificate" "$(signing_certificate "$image")" "$application"
  check_developer_id "$image" image
  attach developer-id "$image"
  check_bundle "$mounted/$name.app"
  check_signed "$mounted/$name.app" "$application"
  detach
fi

echo "all macOS application and disk image checks passed"
