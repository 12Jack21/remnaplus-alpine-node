#!/usr/bin/env bash
set -euo pipefail

out_dir="${1:?usage: generate-auth-fixture.sh <output-dir>}"
rm -rf "$out_dir"
mkdir -p "$out_dir"

openssl genrsa -out "${out_dir}/ca.key" 2048 >/dev/null 2>&1
openssl req -x509 -new -nodes -key "${out_dir}/ca.key" -sha256 -days 2 \
  -subj "/CN=remnaplus-alpine-test-ca" \
  -out "${out_dir}/ca.crt" >/dev/null 2>&1

openssl genrsa -out "${out_dir}/node.key" 2048 >/dev/null 2>&1
openssl req -new -key "${out_dir}/node.key" -subj "/CN=127.0.0.1" -out "${out_dir}/node.csr" >/dev/null 2>&1
cat > "${out_dir}/node.ext" <<'EOF'
subjectAltName=IP:127.0.0.1,DNS:localhost
extendedKeyUsage=serverAuth
EOF
openssl x509 -req -in "${out_dir}/node.csr" -CA "${out_dir}/ca.crt" -CAkey "${out_dir}/ca.key" \
  -CAcreateserial -out "${out_dir}/node.crt" -days 2 -sha256 -extfile "${out_dir}/node.ext" >/dev/null 2>&1

openssl genrsa -out "${out_dir}/client.key" 2048 >/dev/null 2>&1
openssl req -new -key "${out_dir}/client.key" -subj "/CN=remnaplus-panel-test" -out "${out_dir}/client.csr" >/dev/null 2>&1
cat > "${out_dir}/client.ext" <<'EOF'
extendedKeyUsage=clientAuth
EOF
openssl x509 -req -in "${out_dir}/client.csr" -CA "${out_dir}/ca.crt" -CAkey "${out_dir}/ca.key" \
  -CAcreateserial -out "${out_dir}/client.crt" -days 2 -sha256 -extfile "${out_dir}/client.ext" >/dev/null 2>&1

openssl genrsa -out "${out_dir}/jwt.key" 2048 >/dev/null 2>&1
openssl rsa -in "${out_dir}/jwt.key" -pubout -out "${out_dir}/jwt.pub" >/dev/null 2>&1

pem_json() {
  awk '{printf "%s\\n", $0}' "$1"
}

secret_json="${out_dir}/secret.json"
{
  printf '{'
  printf '"caCertPem":"%s",' "$(pem_json "${out_dir}/ca.crt")"
  printf '"jwtPublicKey":"%s",' "$(pem_json "${out_dir}/jwt.pub")"
  printf '"nodeCertPem":"%s",' "$(pem_json "${out_dir}/node.crt")"
  printf '"nodeKeyPem":"%s"' "$(pem_json "${out_dir}/node.key")"
  printf '}'
} > "$secret_json"

secret_key="$(base64 < "$secret_json" | tr -d '\n')"

b64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

header="$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)"
exp="$(( $(date +%s) + 86400 ))"
payload="$(printf '{"iss":"remnawave","aud":"remnawave-node","sub":"remnawave-backend","exp":%s}' "$exp" | b64url)"
signed="${header}.${payload}"
signature="$(printf '%s' "$signed" | openssl dgst -sha256 -sign "${out_dir}/jwt.key" -binary | b64url)"
token="${signed}.${signature}"

cat > "${out_dir}/fixture.env" <<EOF
SECRET_KEY=${secret_key}
TOKEN=${token}
CA_CERT=${out_dir}/ca.crt
CLIENT_CERT=${out_dir}/client.crt
CLIENT_KEY=${out_dir}/client.key
EOF
