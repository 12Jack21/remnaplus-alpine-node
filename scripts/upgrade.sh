#!/usr/bin/env bash
# github.com/12Jack21/remnaplus-alpine-node Alpine/OpenRC transactional upgrade
set -euo pipefail

VERSION="1.0.3"
PREFIX="/usr/local/bin"
ETC_DIR="/etc/remnanode"
OPENRC_SVC="/etc/init.d/remnawave-node"
RUN_WRAPPER="${PREFIX}/remnawave-node-run"
BIN_NAME="remnanode-lite"
NODE_ENV="${ETC_DIR}/node.env"
REPO="${RNL_REPO:-12Jack21/remnaplus-alpine-node}"
TAG="${RNL_TAG:-v${VERSION}}"
RNL_RELEASE_BASE_URL="${RNL_RELEASE_BASE_URL:-https://github.com/${REPO}/releases/download/${TAG}}"
UPGRADE_XRAY="${RNL_UPGRADE_XRAY:-0}"
SCRIPT_DIR=""
if resolved_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)"; then
  SCRIPT_DIR="$resolved_script_dir"
fi
if [ -f "${SCRIPT_DIR}/../release.env" ]; then
  # shellcheck disable=SC1091
  source "${SCRIPT_DIR}/../release.env"
fi

YES=0
DRY_RUN=0
STAGE="初始化"

usage() {
  cat <<EOF
用法：upgrade.sh [--yes] [--dry-run] [--upgrade-xray] [--help] [--version]

RemnaPlus Alpine Node 升级到 ${TAG}

环境变量：
  RNL_REPO              GitHub 仓库，默认 12Jack21/remnaplus-alpine-node
  RNL_TAG               固定 Release 标签，默认 v${VERSION}
  RNL_RELEASE_BASE_URL  Release 资产基础 URL（测试可覆盖）
  RNL_UPGRADE_XRAY      设为 1 时同时运行 install-xray.sh
EOF
}

version() {
  echo "remnaplus-alpine-node upgrade ${VERSION}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) YES=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --upgrade-xray) UPGRADE_XRAY=1 ;;
    --help|-h) usage; exit 0 ;;
    --version) version; exit 0 ;;
    *)
      echo "未知参数：$1" >&2
      usage
      exit 1
      ;;
  esac
  shift
done

on_error() {
  local exit_code=$?
  echo "升级失败：${STAGE}" >&2
  echo "失败命令：${BASH_COMMAND}" >&2
  exit "$exit_code"
}

trap on_error ERR

step() {
  STAGE="$1"
  echo "==> $1"
}

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] $*"
  else
    "$@"
  fi
}

require_alpine() {
  if [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if [ ! -f /etc/alpine-release ]; then
    echo "此脚本仅适用于 Alpine Linux（未找到 /etc/alpine-release）。" >&2
    exit 1
  fi
}

require_root() {
  if [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if [ "$(id -u)" -ne 0 ]; then
    echo "请使用 root 运行（Alpine 通常无 sudo）：su - 后执行 bash upgrade.sh" >&2
    exit 1
  fi
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少命令：$1" >&2
    exit 1
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "不支持的架构：$(uname -m)" >&2
      exit 1
      ;;
  esac
}

current_version() {
  if [ -x "${PREFIX}/${BIN_NAME}" ]; then
    "${PREFIX}/${BIN_NAME}" version 2>/dev/null || echo "unknown"
  else
    echo "not installed"
  fi
}

configured_node_port() {
  if [ -f "$NODE_ENV" ] && grep -q '^NODE_PORT=' "$NODE_ENV" 2>/dev/null; then
    grep '^NODE_PORT=' "$NODE_ENV" | head -n 1 | cut -d= -f2-
  else
    echo "2222"
  fi
}

confirm_upgrade() {
  if [ "$YES" -eq 1 ] || [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  echo "当前：$(current_version)"
  echo "目标：${TAG}"
  read -r -p "继续升级？[y/N] " ans
  case "$ans" in
    y|Y|yes|YES) ;;
    *) echo "已取消。"; exit 0 ;;
  esac
}

download_candidate() {
  local arch="$1"
  local tmp="$2"
  local archive_name="remnanode-lite_linux_${arch}.tar.gz"
  local url="${RNL_RELEASE_BASE_URL}/${archive_name}"
  local candidate_dir="${tmp}/candidate"
  local checksum_var="RNL_BINARY_SHA256_${arch^^}"
  local expected="${RNL_BINARY_SHA256:-${!checksum_var:-}}"

  step "下载候选版本 ${TAG} (linux/${arch})"
  mkdir -p "$candidate_dir"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] curl -fsSL ${url}"
    echo "[dry-run] sha256sum -c -"
    return 0
  fi

  if ! [[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    echo "Missing reviewed binary SHA-256 for linux/${arch}." >&2
    exit 1
  fi
  curl -fsSL "$url" -o "${tmp}/${archive_name}"
  printf '%s  %s\n' "${expected,,}" "${tmp}/${archive_name}" | sha256sum -c -
  while IFS= read -r entry; do
    case "$entry" in
      /*|../*|*/../*|*/..) echo "Unsafe binary archive entry: $entry" >&2; exit 1 ;;
    esac
  done < <(tar -tzf "${tmp}/${archive_name}")
  tar -xzf "${tmp}/${archive_name}" -C "$candidate_dir"
  test -x "${candidate_dir}/${BIN_NAME}"
  "${candidate_dir}/${BIN_NAME}" version
}

refresh_openrc() {
  step "刷新 OpenRC 服务文件"
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 更新 ${OPENRC_SVC} 与 ${RUN_WRAPPER}"
    return 0
  fi

  if [ -f "${script_dir}/../deploy/remnawave-node-run.sh" ]; then
    install -m 0755 "${script_dir}/../deploy/remnawave-node-run.sh" "$RUN_WRAPPER"
  else
    echo "Verified installer bundle is missing deploy/remnawave-node-run.sh." >&2
    exit 1
  fi

  if [ -f "${script_dir}/../deploy/remnawave-node.openrc" ]; then
    install -m 0755 "${script_dir}/../deploy/remnawave-node.openrc" "$OPENRC_SVC"
  else
    echo "Verified installer bundle is missing deploy/remnawave-node.openrc." >&2
    exit 1
  fi
  rc-update add remnawave-node default 2>/dev/null || true
}

apply_capabilities() {
  step "重新授予 CAP_NET_ADMIN（Alpine setcap）"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] setcap cap_net_admin+ep ${PREFIX}/${BIN_NAME}"
    return 0
  fi
  setcap cap_net_admin+ep "${PREFIX}/${BIN_NAME}"
}

upgrade_xray() {
  if [ "$UPGRADE_XRAY" -ne 1 ]; then
    echo "跳过 rw-core 升级（设 RNL_UPGRADE_XRAY=1 或 --upgrade-xray 可启用）。"
    return 0
  fi

  step "升级 rw-core"
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if [ -f "${script_dir}/install-xray.sh" ]; then
    bash "${script_dir}/install-xray.sh"
  else
    echo "Verified installer bundle is missing scripts/install-xray.sh." >&2
    exit 1
  fi
}

restart_service() {
  step "重启 remnawave-node"
  if [ "${RNL_OPENRC_DIRECT_START:-0}" = "1" ]; then
    if command -v pidof >/dev/null 2>&1; then
      for pid in $(pidof "$BIN_NAME" 2>/dev/null || true); do
        kill "$pid" 2>/dev/null || true
      done
      sleep 1
    fi
    if [ "$DRY_RUN" -eq 0 ]; then
      nohup "$RUN_WRAPPER" >>/var/log/remnanode/openrc.log 2>>/var/log/remnanode/openrc.err.log &
    else
      echo "[dry-run] nohup ${RUN_WRAPPER}"
    fi
    return 0
  fi
  run rc-service remnawave-node restart
  if [ "$DRY_RUN" -eq 0 ]; then
    sleep 1
    rc-service remnawave-node status || true
  fi
}

wait_for_service_stable() {
  local port="$1"
  local max_wait="${2:-30}"
  local i=0

  if [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi

  while [ "$i" -lt "$max_wait" ]; do
    if ss -tln 2>/dev/null | grep -q ":${port} "; then
      if [ "${RNL_OPENRC_DIRECT_START:-0}" = "1" ] || \
        rc-service remnawave-node status 2>/dev/null | grep -qi 'started'; then
        return 0
      fi
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}

rollback() {
  local backup="$1"
  local port="$2"
  echo "候选版本启动失败，开始 rollback。" >&2
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] install backup ${backup} -> ${PREFIX}/${BIN_NAME}"
    return 0
  fi
  install -m 0755 "$backup" "${PREFIX}/${BIN_NAME}"
  setcap cap_net_admin+ep "${PREFIX}/${BIN_NAME}" 2>/dev/null || true
  restart_service || true
  wait_for_service_stable "$port" 30 || true
}

install_candidate() {
  local tmp="$1"
  local backup="${tmp}/rollback/${BIN_NAME}"
  local port
  port="$(configured_node_port)"

  step "安装候选二进制"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] candidate install ${PREFIX}/${BIN_NAME}"
    echo "[dry-run] rollback backup ${backup}"
    return 0
  fi

  mkdir -p "${tmp}/rollback"
  install -m 0755 "${PREFIX}/${BIN_NAME}" "$backup"
  install -m 0755 "${tmp}/candidate/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"

  apply_capabilities
  refresh_openrc
  restart_service

  if ! wait_for_service_stable "$port" 30; then
    rollback "$backup" "$port"
    exit 1
  fi

  install -m 0755 "$backup" "${PREFIX}/${BIN_NAME}.bak.$(date +%Y%m%d%H%M%S)"
}

main() {
  require_root
  require_alpine
  require_command curl
  require_command tar
  require_command sha256sum
  require_command setcap
  require_command rc-service
  require_command ss

  if [ ! -x "${PREFIX}/${BIN_NAME}" ] && [ "$DRY_RUN" -eq 0 ]; then
    echo "未检测到已安装的 ${BIN_NAME}，请先运行 install-node-alpine.sh。" >&2
    exit 1
  fi

  confirm_upgrade

  local arch tmp
  arch="$(detect_arch)"
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp:-}"' EXIT

  echo "升级前：$(current_version)"
  download_candidate "$arch" "$tmp"
  install_candidate "$tmp"
  upgrade_xray

  echo
  echo "升级完成。"
  echo "  当前版本：$(current_version)"
  echo "  配置保留：${NODE_ENV}"
  echo "  日志：    tail -f /var/log/remnanode/openrc.log"
  echo "  rollback 备份：ls ${PREFIX}/${BIN_NAME}.bak.*"
}

main "$@"
