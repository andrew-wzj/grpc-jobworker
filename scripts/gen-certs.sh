#!/bin/sh
# Generate a throwaway CA plus server and client certificates for local mTLS.
# Works with both OpenSSL and LibreSSL (macOS).
# Usage: scripts/gen-certs.sh [out-dir]   (default: certs/)
set -eu
out=${1:-certs}
mkdir -p "$out"
cd "$out"

newkey() { openssl ecparam -name prime256v1 -genkey -noout -out "$1"; }

newkey ca.key
openssl req -x509 -new -key ca.key -sha256 -days 365 -subj "/CN=jobworker dev CA" -out ca.crt

for name in server client; do
  newkey "$name.key"
  openssl req -new -key "$name.key" -sha256 -subj "/CN=$name" -out "$name.csr"
  printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth,clientAuth\n' > "$name.ext"
  openssl x509 -req -in "$name.csr" -CA ca.crt -CAkey ca.key -CAcreateserial \
    -sha256 -days 365 -extfile "$name.ext" -out "$name.crt" 2>/dev/null
  rm "$name.csr" "$name.ext"
done
rm -f ca.srl
chmod 600 ./*.key
echo "Wrote $(pwd)/{ca,server,client}.{crt,key}"
