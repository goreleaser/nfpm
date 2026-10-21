#!/usr/bin/env bash
# Throwaway self-signed code-signing certificate for the signed-package
# acceptance jobs. Mirrors create-test-cert.ps1 (same subject, same password)
# but runs on Linux so nfpm's pure-Go MSI/MSIX signers can be exercised
# without any Windows tooling; the resulting packages are then validated and
# installed on a real Windows runner.
#
# Subject RDNs are given in C, O, CN order on purpose: Windows renders them
# reversed as "CN=TestCompany, O=TestCompany, C=US". The signed MSIX config
# omits publisher to exercise nfpm's Windows-compatible subject formatting.
set -euo pipefail

cd "$(dirname "$0")/../.."
mkdir -p dist

openssl req -x509 -newkey rsa:2048 -nodes -days 2 -sha256 \
  -subj "/C=US/O=TestCompany/CN=TestCompany" \
  -addext "keyUsage=critical,digitalSignature" \
  -addext "extendedKeyUsage=codeSigning" \
  -addext "basicConstraints=CA:FALSE" \
  -keyout dist/test.key -out dist/test.crt

openssl pkcs12 -export -inkey dist/test.key -in dist/test.crt \
  -passout pass:test123 -out dist/test.pfx

openssl x509 -in dist/test.crt -outform DER -out dist/test.cer

rm -f dist/test.key
echo "wrote dist/test.pfx, dist/test.crt, dist/test.cer"
