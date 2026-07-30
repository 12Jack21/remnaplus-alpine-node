#!/usr/bin/env bash
# 安装 rw-core（Xray）及 geo 资源文件
set -euo pipefail

XRAY_CORE_VERSION="${XRAY_CORE_VERSION:-v26.6.27}"
UPSTREAM_REPO="${UPSTREAM_REPO:-XTLS}"
NODE_ENV="${NODE_ENV:-/etc/remnanode/node.env}"

usage() {
  cat <<'EOF'
用法：install-xray.sh [--version v26.6.27] [--upstream XTLS] [--dry-run]

环境变量：
  XRAY_CORE_VERSION   rw-core 版本，默认 v26.6.27（Node 2.8.0 要求 ≥ 26.6.27）
  UPSTREAM_REPO       上游仓库标识，默认 XTLS
  XRAY_RELEASE_BASE_URL  Xray Release 基础 URL（测试可覆盖）
  CUSTOM_CORE_URL     自定义 rw-core 下载 URL（对齐官方 Docker entrypoint，设置后跳过官方安装脚本）
EOF
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
  export "$key"
}

install_custom_core() {
  local url="$1"
  local target="/usr/local/bin/rw-core"
  echo "CUSTOM_CORE_URL 已设置，从自定义地址下载 rw-core..."
  echo "  URL: ${url}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$target"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$target" "$url"
  else
    echo "缺少 curl 或 wget，无法下载 CUSTOM_CORE_URL" >&2
    return 1
  fi
  chmod +x "$target"
  if [ ! -x "$target" ]; then
    echo "下载完成但 $target 不可执行" >&2
    return 1
  fi
  echo "自定义 rw-core 已安装到 ${target}"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "64" ;;
    aarch64|arm64) echo "arm64-v8a" ;;
    *)
      echo "不支持的架构：$(uname -m)" >&2
      exit 1
      ;;
  esac
}

install_release_core() {
  local arch="$1"
  local base_url="${XRAY_RELEASE_BASE_URL:-https://github.com/${UPSTREAM_REPO}/Xray-core/releases/download/${XRAY_CORE_VERSION}}"
  local archive_name="Xray-linux-${arch}.zip"
  local tmp
  tmp="$(mktemp -d)"

  echo "下载 rw-core ${XRAY_CORE_VERSION} (${archive_name})..."
  curl -fsSL "${base_url}/${archive_name}" -o "${tmp}/${archive_name}"
  unzip -q "${tmp}/${archive_name}" -d "$tmp"
  install -m 0755 "${tmp}/xray" /usr/local/bin/rw-core
  ln -sf /usr/local/bin/rw-core /usr/local/bin/xray
  install -d /usr/local/share/xray
  install -m 0644 "${tmp}/geoip.dat" /usr/local/share/xray/geoip.dat
  install -m 0644 "${tmp}/geosite.dat" /usr/local/share/xray/geosite.dat
  rm -rf "$tmp"
}

# ASN 前缀数据库（插件 asList 共享列表解析；对齐官方 2.8.0 的 /usr/local/share/asn）。
# 未提供 ASN_DB_URL 时跳过；运行时缺失该文件则 asList 自动降级为空。
ASN_DB_URL="${ASN_DB_URL:-}"
ASN_DB_PATH="${ASN_DB_PATH:-/usr/local/share/asn/asn-prefixes.bin}"
install_asn_db() {
  if [ -z "${ASN_DB_URL}" ]; then
    echo "提示：未设置 ASN_DB_URL，跳过 ASN 数据库安装（插件 asList 将降级为空）。"
    return 0
  fi
  mkdir -p "$(dirname "${ASN_DB_PATH}")"
  echo "下载 ASN 前缀数据库：${ASN_DB_URL}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${ASN_DB_URL}" -o "${ASN_DB_PATH}" || { echo "警告：ASN 数据库下载失败，asList 将降级为空。" >&2; return 0; }
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "${ASN_DB_PATH}" "${ASN_DB_URL}" || { echo "警告：ASN 数据库下载失败，asList 将降级为空。" >&2; return 0; }
  else
    echo "警告：缺少 curl/wget，跳过 ASN 数据库下载。" >&2
    return 0
  fi
  echo "ASN 数据库已安装到 ${ASN_DB_PATH}"
}

DRY_RUN=0
while [ $# -gt 0 ]; do
  case "$1" in
    --version) XRAY_CORE_VERSION="$2"; shift 2 ;;
    --upstream) UPSTREAM_REPO="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *)
      echo "未知参数：$1" >&2
      usage
      exit 1
      ;;
  esac
done

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "请使用 root 运行（Alpine 通常无 sudo）：su - 后执行 bash install-xray.sh" >&2
    exit 1
  fi
}

require_root

load_env_var CUSTOM_CORE_URL "$NODE_ENV"

if ! command -v bash >/dev/null 2>&1; then
  echo "缺少命令：bash（Alpine: apk add --no-cache bash）" >&2
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
  echo "rw-core 版本：$(/usr/local/bin/rw-core version | head -n 1)"
else
  echo "警告：/usr/local/bin/rw-core 未找到，请检查安装日志。" >&2
  exit 1
fi

for dat in geoip.dat geosite.dat; do
  if [ ! -f "/usr/local/share/xray/${dat}" ]; then
    echo "警告：缺少 /usr/local/share/xray/${dat}" >&2
  fi
done

install_asn_db

echo "rw-core 安装完成。"
