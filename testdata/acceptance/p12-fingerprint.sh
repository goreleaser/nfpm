#!/usr/bin/env bash
# Prints the SHA-256 fingerprint of the certificate in a PKCS#12 file as
# upper-case hex without separators. Only the public certificate is read out.
# The passphrase is taken from NFPM_PASSPHRASE and may be empty.
set -euo pipefail

p12="${1:?usage: p12-fingerprint.sh identity.p12}"
pass="pass:${NFPM_PASSPHRASE:-}"

# Identities exported from Keychain use ciphers OpenSSL 3 only reads with
# -legacy; older OpenSSL and LibreSSL do not know the flag.
certificate="$(openssl pkcs12 -in "$p12" -nokeys -clcerts -passin "$pass" 2>/dev/null ||
  openssl pkcs12 -legacy -in "$p12" -nokeys -clcerts -passin "$pass")"

openssl x509 -noout -fingerprint -sha256 <<<"$certificate" |
  cut -d= -f2 | tr -d ':\n' | tr '[:lower:]' '[:upper:]'
echo
