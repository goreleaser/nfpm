# Shared by the macOS verification scripts. Source it, do not run it.

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

expect() {
  if [ "$2" != "$3" ]; then
    fail "$1: got '$2', want '$3'"
  fi
  echo "ok: $1 is '$2'"
}

# require_host_arch fails unless the packages were built for this machine.
require_host_arch() {
  [ "$(uname -s)" = Darwin ] || fail "this script needs macOS"
  local host
  case "$(uname -m)" in
    arm64) host=arm64 ;;
    x86_64) host=amd64 ;;
    *) fail "unexpected machine $(uname -m)" ;;
  esac
  expect "package architecture matches this machine" "$1" "$host"
}

# certificate_fingerprint prints the SHA-256 fingerprint of a PEM or DER
# certificate file as upper-case hex without separators.
certificate_fingerprint() {
  local form=PEM
  grep -q "BEGIN CERTIFICATE" "$1" || form=DER
  openssl x509 -inform "$form" -in "$1" -noout -fingerprint -sha256 |
    cut -d= -f2 | tr -d ':\n' | tr '[:lower:]' '[:upper:]'
}

# developer_id reports whether packages signed with real Apple Developer ID
# identities were built. CI sets NFPM_MACOS_EXPECT_DEVELOPER_ID where it has
# the identities, so their absence there is a failure, not a skipped check.
developer_id() {
  if [ -d dist/developer-id ]; then
    return 0
  fi
  if [ "${NFPM_MACOS_EXPECT_DEVELOPER_ID:-false}" = true ]; then
    fail "the Developer ID signed packages are missing from dist/developer-id"
  fi
  echo "skipped: no Developer ID signed packages (the identities are not available to this run)"
  return 1
}
