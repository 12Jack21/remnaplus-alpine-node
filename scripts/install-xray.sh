#!/usr/bin/env bash
# 安装 rw-core（Xray）及 geo 资源文件
set -euo pipefail

RNL_LANG="${RNL_LANG:-zh}"
export RNL_LANG
SCRIPT_DIR=""
if resolved_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)"; then
  SCRIPT_DIR="$resolved_script_dir"
fi
if [ -n "$SCRIPT_DIR" ] && [ -f "${SCRIPT_DIR}/install-node-alpine.messages.sh" ]; then
  # shellcheck disable=SC1091
  source "${SCRIPT_DIR}/install-node-alpine.messages.sh"
else
  echo "Missing install-node-alpine.messages.sh" >&2
  exit 1
fi

XRAY_CORE_VERSION="${XRAY_CORE_VERSION:-v26.6.27}"
UPSTREAM_REPO="${UPSTREAM_REPO:-XTLS}"
NODE_ENV="${NODE_ENV:-/etc/remnanode/node.env}"

usage() {
  rnl_print_xray_usage
}

load_env_var() {
  local key="$1"
  local file="$2"
  [ -f "$file" ] || return 0
  local line val
  line="$(grep -E "^[[:space:]]*${key}=" "$file" 2>/dev/null | head -n 1 || true)"
  [ -n "$line" ] || return 0
  val="${line#*=}"
  val="$(printf '%s' "$val" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")"
  [ -n "$val" ] || return 0
  printf -v "$key" '%s' "$val"
  export "${key?}"
}

install_custom_core() {
  local url="$1"
  local expected="${CUSTOM_CORE_SHA256:-}"
  local target="/usr/local/bin/xray"
  local staged
  if ! [[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    echo "CUSTOM_CORE_URL requires CUSTOM_CORE_SHA256." >&2
    return 1
  fi
  staged="$(mktemp /usr/local/bin/.xray.custom.XXXXXX)"
  trap 'rm -f -- "${staged:-}"' RETURN
  rnl_msg custom_core_download "$url"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$staged"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$staged" "$url"
  else
    rnl_msg download_tool_missing CUSTOM_CORE_URL >&2
    return 1
  fi
  printf '%s  %s\n' "${expected,,}" "$staged" | sha256sum -c -
  chmod 0755 "$staged"
  if ! "$staged" version >/dev/null 2>&1; then
    rnl_msg not_executable "$staged" >&2
    return 1
  fi
  mv -f "$staged" "$target"
  ensure_managed_core_link
  rnl_msg custom_core_installed "$target"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "64" ;;
    aarch64|arm64) echo "arm64-v8a" ;;
    *)
      rnl_msg unsupported_arch "$(uname -m)" >&2
      exit 1
      ;;
  esac
}

xray_archive_sha256() {
  case "$1" in
    64)
      if [ "$XRAY_CORE_VERSION" = "v26.6.27" ]; then
        echo "${XRAY_SHA256_64:-b3e5902d06d6282fe53cfa2fc426058b9aeaa429b2c812e20887cd47f26d08bf}"
      else
        echo "${XRAY_SHA256_64:-}"
      fi
      ;;
    arm64-v8a)
      if [ "$XRAY_CORE_VERSION" = "v26.6.27" ]; then
        echo "${XRAY_SHA256_ARM64:-13a251379bea366c2cf10363ad71e75734193d401f26f518bf0c25e5c8f8c931}"
      else
        echo "${XRAY_SHA256_ARM64:-}"
      fi
      ;;
    *) return 1 ;;
  esac
}

ensure_managed_core_link() {
  local active="/usr/local/bin/rw-core"
  local stock="/usr/local/bin/xray"
  if [ -e "$active" ] && [ ! -L "$active" ]; then
    if [ ! -e "$stock" ]; then
      mv "$active" "$stock"
    else
      rm -f "$active"
    fi
  fi
  local staged="${active}.link.$$"
  ln -s "$stock" "$staged"
  mv -f "$staged" "$active"
}

install_release_core() {
  local arch="$1"
  local base_url="${XRAY_RELEASE_BASE_URL:-https://github.com/${UPSTREAM_REPO}/Xray-core/releases/download/${XRAY_CORE_VERSION}}"
  local archive_name="Xray-linux-${arch}.zip"
  local expected
  expected="$(xray_archive_sha256 "$arch")"
  if ! [[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    echo "Xray ${XRAY_CORE_VERSION} requires a reviewed SHA-256 for ${arch}." >&2
    exit 1
  fi
  local tmp
  tmp="$(mktemp -d)"

  rnl_msg xray_download "$XRAY_CORE_VERSION" "$archive_name"
  curl -fsSL "${base_url}/${archive_name}" -o "${tmp}/${archive_name}"
  printf '%s  %s\n' "${expected,,}" "${tmp}/${archive_name}" | sha256sum -c -
  while IFS= read -r entry; do
    case "$entry" in
      /*|../*|*/../*|*/..) echo "Unsafe Xray archive entry: $entry" >&2; exit 1 ;;
    esac
  done < <(unzip -Z1 "${tmp}/${archive_name}")
  unzip -q "${tmp}/${archive_name}" -d "$tmp"
  "${tmp}/xray" version >/dev/null
  local staged_core="/usr/local/bin/.xray.stock.$$"
  install -m 0755 "${tmp}/xray" "$staged_core"
  mv -f "$staged_core" /usr/local/bin/xray
  ensure_managed_core_link
  install -d /usr/local/share/xray
  for dat in geoip.dat geosite.dat; do
    install -m 0644 "${tmp}/${dat}" "/usr/local/share/xray/.${dat}.new.$$"
    mv -f "/usr/local/share/xray/.${dat}.new.$$" "/usr/local/share/xray/${dat}"
  done
  rm -rf "$tmp"
}

# ASN 前缀数据库（插件 asList 共享列表解析；对齐官方 2.8.0 的 /usr/local/share/asn）。
# 未提供 ASN_DB_URL 时跳过；运行时缺失该文件则 asList 自动降级为空。
ASN_DB_URL="${ASN_DB_URL:-}"
ASN_DB_PATH="${ASN_DB_PATH:-/usr/local/share/asn/asn-prefixes.bin}"
install_asn_db() {
  if [ -z "${ASN_DB_URL}" ]; then
    rnl_msg asn_skip
    return 0
  fi
  mkdir -p "$(dirname "${ASN_DB_PATH}")"
  local expected="${ASN_DB_SHA256:-}"
  if ! [[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    echo "ASN_DB_URL requires ASN_DB_SHA256; skipping unverified ASN database." >&2
    return 0
  fi
  local staged="${ASN_DB_PATH}.new.$$"
  rnl_msg asn_download "$ASN_DB_URL"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${ASN_DB_URL}" -o "$staged" || { rm -f "$staged"; rnl_msg asn_download_failed >&2; return 0; }
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$staged" "${ASN_DB_URL}" || { rm -f "$staged"; rnl_msg asn_download_failed >&2; return 0; }
  else
    rnl_msg asn_tool_missing >&2
    return 0
  fi
  if ! printf '%s  %s\n' "${expected,,}" "$staged" | sha256sum -c -; then
    rm -f "$staged"
    rnl_msg asn_download_failed >&2
    return 0
  fi
  chmod 0644 "$staged"
  mv -f "$staged" "$ASN_DB_PATH"
  rnl_msg asn_installed "$ASN_DB_PATH"
}

DRY_RUN=0
while [ $# -gt 0 ]; do
  case "$1" in
    --version) XRAY_CORE_VERSION="$2"; shift 2 ;;
    --upstream) UPSTREAM_REPO="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *)
      rnl_msg unknown_argument "$1" >&2
      usage
      exit 1
      ;;
  esac
done

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    rnl_msg root_required >&2
    exit 1
  fi
}

require_root

load_env_var CUSTOM_CORE_URL "$NODE_ENV"

if ! command -v bash >/dev/null 2>&1; then
  rnl_msg missing_command "bash (Alpine: apk add --no-cache bash)" >&2
  exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
  rnl_msg missing_command "sha256sum" >&2
  exit 1
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "[dry-run] curl -fsSL \${XRAY_RELEASE_BASE_URL:-https://github.com/${UPSTREAM_REPO}/Xray-core/releases/download/${XRAY_CORE_VERSION}}/Xray-linux-\$(detect_arch).zip"
  echo "[dry-run] install /usr/local/bin/rw-core"
  exit 0
fi

if [ -n "${CUSTOM_CORE_URL:-}" ]; then
  install_custom_core "$CUSTOM_CORE_URL"
else
  install_release_core "$(detect_arch)"
fi

if [ -x /usr/local/bin/rw-core ]; then
  rnl_msg xray_version "$(/usr/local/bin/rw-core version | head -n 1)"
else
  rnl_msg xray_missing >&2
  exit 1
fi

for dat in geoip.dat geosite.dat; do
  if [ ! -f "/usr/local/share/xray/${dat}" ]; then
    rnl_msg geo_missing "/usr/local/share/xray/${dat}" >&2
  fi
done

install_asn_db

rnl_msg xray_complete
