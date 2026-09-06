#!/usr/bin/env bash
# github.com/12Jack21/remnaplus-alpine-node Alpine Linux 一键安装（OpenRC）
set -euo pipefail

VERSION="1.0.3"
RNL_LANG="${RNL_LANG:-zh}"
export RNL_LANG
PREFIX="/usr/local/bin"
ETC_DIR="/etc/remnanode"
DATA_DIR="/var/lib/remnanode"
LOG_DIR="/var/log/remnanode"
OPENRC_SVC="/etc/init.d/remnawave-node"
RUN_WRAPPER="${PREFIX}/remnawave-node-run"
BIN_NAME="remnanode-lite"
NODE_ENV="${ETC_DIR}/node.env"
SECRET_FILE="${ETC_DIR}/secret.key"
REPO="${RNL_REPO:-12Jack21/remnaplus-alpine-node}"
TAG="${RNL_TAG:-v${VERSION}}"
RNL_RELEASE_BASE_URL="${RNL_RELEASE_BASE_URL:-https://github.com/${REPO}/releases/download/${TAG}}"
RESTART_CMD="rc-service remnawave-node restart"
export RESTART_CMD

case "$RNL_LANG" in
  zh|en) ;;
  *) echo "Unsupported installer language: ${RNL_LANG}" >&2; exit 64 ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  if [ "$RNL_LANG" = "en" ]; then
    echo "Missing command: curl (Alpine: apk add --no-cache curl bash)" >&2
  else
    echo "缺少命令：curl（Alpine: apk add --no-cache curl bash）" >&2
  fi
  exit 1
fi
_INSTALLER_DIR=""
if [ -n "${BASH_SOURCE[0]:-}" ]; then
  if _RESOLVED_INSTALLER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)"; then
    _INSTALLER_DIR="$_RESOLVED_INSTALLER_DIR"
  fi
fi
if [ -n "$_INSTALLER_DIR" ] && [ -f "${_INSTALLER_DIR}/install-node-alpine.messages.sh" ] && \
   [ -f "${_INSTALLER_DIR}/install-env-helpers.sh" ]; then
  # shellcheck disable=SC1091
  source "${_INSTALLER_DIR}/install-node-alpine.messages.sh"
  # shellcheck disable=SC1091
  source "${_INSTALLER_DIR}/install-env-helpers.sh"
else
  _EXPECTED_BUNDLE="${RNL_INSTALLER_BUNDLE_SHA256:-}"
  if ! [[ "$_EXPECTED_BUNDLE" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    echo "Standalone installation requires the reviewed RNL_INSTALLER_BUNDLE_SHA256." >&2
    exit 1
  fi
  _BOOTSTRAP_DIR="$(mktemp -d)"
  trap 'rm -rf -- "$_BOOTSTRAP_DIR"' EXIT
  _BUNDLE="remnanode-native-installer_${TAG}.tar.gz"
  curl -fsSL "${RNL_RELEASE_BASE_URL}/${_BUNDLE}" -o "${_BOOTSTRAP_DIR}/${_BUNDLE}"
  printf '%s  %s\n' "${_EXPECTED_BUNDLE,,}" "${_BOOTSTRAP_DIR}/${_BUNDLE}" | sha256sum -c -
  while IFS= read -r entry; do
    case "$entry" in
      /*|../*|*/../*|*/..) echo "Unsafe installer bundle entry: $entry" >&2; exit 1 ;;
    esac
  done < <(tar -tzf "${_BOOTSTRAP_DIR}/${_BUNDLE}")
  tar --no-same-owner --no-same-permissions -xzf "${_BOOTSTRAP_DIR}/${_BUNDLE}" -C "$_BOOTSTRAP_DIR"
  _BUNDLED="${_BOOTSTRAP_DIR}/remnanode-native-installer/scripts/install-node-alpine.sh"
  if [ ! -f "$_BUNDLED" ]; then
    echo "Verified installer bundle does not contain scripts/install-node-alpine.sh." >&2
    exit 1
  fi
  RNL_BOOTSTRAPPED=1 bash "$_BUNDLED" "$@"
  exit $?
fi
if [ -f "${_INSTALLER_DIR}/../release.env" ]; then
  # shellcheck disable=SC1091
  source "${_INSTALLER_DIR}/../release.env"
fi
TAG="$(resolve_install_tag "$REPO" "v${VERSION}")"
RNL_RELEASE_BASE_URL="${RNL_RELEASE_BASE_URL:-https://github.com/${REPO}/releases/download/${TAG}}"
INSTALL_XRAY="${RNL_INSTALL_XRAY:-1}"
SKIP_XRAY="${RNL_SKIP_XRAY:-0}"
SECRET_FILE_ARG=""

YES=0
DRY_RUN=0
LOW_MEMORY=0
PORT_EXPLICIT=0
ACTION=""
UNINSTALL_MODE=""
STAGE="initialization"

usage() {
  rnl_print_usage "$VERSION" "$REPO"
}

version() {
  echo "remnawave-node-lite alpine install ${VERSION}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --install) ACTION=install ;;
    --upgrade) ACTION=upgrade ;;
    --uninstall) ACTION=uninstall ;;
    --menu) ACTION=menu ;;
    --yes|-y) YES=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --skip-xray) SKIP_XRAY=1 ;;
    --low-memory) LOW_MEMORY=1 ;;
    --port)
      NODE_PORT="${2:-}"
      if [ -z "$NODE_PORT" ]; then
        rnl_msg argument_requires_value "--port" port >&2
        exit 1
      fi
      PORT_EXPLICIT=1
      shift 2
      continue
      ;;
    --secret-file)
      SECRET_FILE_ARG="${2:-}"
      if [ -z "$SECRET_FILE_ARG" ]; then
        rnl_msg argument_requires_value "--secret-file" file >&2
        exit 1
      fi
      shift 2
      continue
      ;;
    --help|-h) usage; exit 0 ;;
    --version) version; exit 0 ;;
    *)
      rnl_msg unknown_argument "$1" >&2
      usage
      exit 1
      ;;
  esac
  shift
done

on_error() {
  rnl_msg install_failed "$STAGE" >&2
  rnl_msg failed_command "$BASH_COMMAND" >&2
  exit $?
}

trap on_error ERR

step() {
  STAGE="$(rnl_msg "$@")"
  echo "==> $STAGE"
}

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] $*"
  else
    "$@"
  fi
}

read_tty() {
  local _var="$1"
  local _prompt="${2:-}"
  local _line=""
  if [ -n "$_prompt" ]; then
    if [ -t 0 ]; then
      read -r -p "$_prompt" _line || _line=""
    elif [ -r /dev/tty ]; then
      read -r -p "$_prompt" _line </dev/tty || _line=""
    else
      return 1
    fi
  else
    if [ -t 0 ]; then
      read -r _line || _line=""
    elif [ -r /dev/tty ]; then
      read -r _line </dev/tty || _line=""
    else
      return 1
    fi
  fi
  printf -v "$_var" '%s' "$_line"
}

script_dir() {
  if [ -n "${BASH_SOURCE[0]:-}" ]; then
    cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
  else
    echo ""
  fi
}

run_sibling_script() {
  local name="$1"
  shift
  local dir
  dir="$(script_dir)"
  if [ -n "$dir" ] && [ -f "${dir}/${name}" ]; then
    bash "${dir}/${name}" "$@"
  else
    echo "Verified installer bundle is missing scripts/${name}." >&2
    exit 1
  fi
}

show_menu() {
  rnl_print_main_menu "$VERSION"
  local choice=""
  local prompt="Select [1-4]: "
  [ "$RNL_LANG" = "zh" ] && prompt="请选择 [1-4]: "
  read_tty choice "$prompt" || {
    if [ "$RNL_LANG" = "en" ]; then echo "Cannot read input; use --install, --upgrade, or --uninstall." >&2; else echo "无法读取输入。非交互请用: --install | --upgrade | --uninstall" >&2; fi
    exit 1
  }
  case "$choice" in
    1) ACTION=install ;;
    2) ACTION=upgrade ;;
    3) ACTION=uninstall ;;
    4) exit 0 ;;
    *)
      if [ "$RNL_LANG" = "en" ]; then echo "Invalid selection: ${choice}" >&2; else echo "无效选择：${choice}" >&2; fi
      exit 1
      ;;
  esac
}

show_uninstall_menu() {
  rnl_print_uninstall_menu
  local choice=""
  local prompt="Select [1-3]: "
  [ "$RNL_LANG" = "zh" ] && prompt="请选择 [1-3]: "
  read_tty choice "$prompt" || exit 1
  case "$choice" in
    1) UNINSTALL_MODE=keep ;;
    2) UNINSTALL_MODE=full ;;
    3) exit 0 ;;
    *)
      if [ "$RNL_LANG" = "en" ]; then echo "Invalid selection" >&2; else echo "无效选择" >&2; fi
      exit 1
      ;;
  esac
}

dispatch_action() {
  case "$ACTION" in
    install) do_install ;;
    upgrade)
      run_sibling_script upgrade.sh --yes
      ;;
    uninstall)
      show_uninstall_menu
      if [ "${UNINSTALL_MODE:-}" = "full" ]; then
        run_sibling_script uninstall.sh --full
      else
        run_sibling_script uninstall.sh --keep-config --yes
      fi
      ;;
    menu) show_menu; dispatch_action ;;
    *)
      if [ "$RNL_LANG" = "en" ]; then echo "Unknown action: ${ACTION}" >&2; else echo "未知动作：${ACTION}" >&2; fi
      usage
      exit 1
      ;;
  esac
}

require_root() {
  if [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if [ "$(id -u)" -ne 0 ]; then
    rnl_msg root_required >&2
    echo "  su -" >&2
    echo "  然后在 Dashboard 重新复制已校验的安装命令。" >&2
    exit 1
  fi
}

require_alpine() {
  if [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if [ ! -f /etc/alpine-release ]; then
    rnl_msg alpine_required >&2
    rnl_msg standard_node_hint >&2
    exit 1
  fi
}

validate_port() {
  local port="$1"
  if ! [[ "$port" =~ ^[0-9]+$ ]] || [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then
    rnl_msg invalid_port "$port" >&2
    exit 1
  fi
}

effective_node_port() {
  echo "${NODE_PORT:-2222}"
}

configured_node_port() {
  if [ -f "$NODE_ENV" ] && grep -q '^NODE_PORT=' "$NODE_ENV" 2>/dev/null; then
    grep '^NODE_PORT=' "$NODE_ENV" | head -n 1 | cut -d= -f2-
  else
    effective_node_port
  fi
}

prompt_node_port() {
  if [ -n "${NODE_PORT:-}" ] || [ "$YES" -eq 1 ] || [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  echo
  local input=""
  local prompt="Node listener port (private Panel connection port, default 2222): "
  [ "$RNL_LANG" = "zh" ] && prompt="NODE 监听端口（Panel 连接用，默认 2222）: "
  read_tty input "$prompt" || input=""
  NODE_PORT="${input:-2222}"
  validate_port "$NODE_PORT"
}

confirm_install() {
  if [ "$YES" -eq 1 ] || [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if [ ! -x "${PREFIX}/${BIN_NAME}" ] && [ ! -f "$NODE_ENV" ]; then
    return 0
  fi
  rnl_print_existing_install_menu "$NODE_ENV"
  local choice=""
  local prompt="Select [1-3]: "
  [ "$RNL_LANG" = "zh" ] && prompt="请选择 [1-3]: "
  read_tty choice "$prompt" || {
    if [ "$RNL_LANG" = "en" ]; then echo "In non-interactive mode, use --yes or --install." >&2; else echo "非交互环境请用: --yes 或 --install" >&2; fi
    exit 1
  }
  case "$choice" in
    1) ;;
    2)
      if [ "$DRY_RUN" -eq 1 ]; then
        echo "[dry-run] 删除 ${ETC_DIR} ${LOG_DIR} ${DATA_DIR}"
      else
        rc-service remnawave-node stop 2>/dev/null || true
        rm -rf "$ETC_DIR" "$LOG_DIR" "$DATA_DIR"
        cleanup_runtime
        rm -f "${ETC_DIR}.bak."* 2>/dev/null || true
        if [ "$RNL_LANG" = "en" ]; then echo "Removed the old configuration; starting a clean install."; else echo "已清除旧配置，开始全新安装。"; fi
      fi
      ;;
    *)
      if [ "$RNL_LANG" = "en" ]; then echo "Cancelled."; else echo "已取消。"; fi
      exit 0
      ;;
  esac
}

update_node_port_in_env() {
  local port="$1"
  validate_port "$port"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 更新 ${NODE_ENV} NODE_PORT=${port}"
    return 0
  fi
  if grep -q '^NODE_PORT=' "$NODE_ENV"; then
    sed -i "s/^NODE_PORT=.*/NODE_PORT=${port}/" "$NODE_ENV"
  else
    echo "NODE_PORT=${port}" >>"$NODE_ENV"
  fi
  echo "已设置 NODE_PORT=${port}"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      rnl_msg unsupported_arch "$(uname -m)" >&2
      exit 1
      ;;
  esac
}

install_packages() {
  step packages
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] apk add --no-cache bash curl tar ca-certificates libcap openrc iproute2 nftables vnstat unzip"
    return 0
  fi
  apk add --no-cache bash curl tar ca-certificates libcap openrc iproute2 nftables vnstat unzip
}

download_binary() {
  local arch="$1"
  local archive_name="remnanode-lite_linux_${arch}.tar.gz"
  local url="${RNL_RELEASE_BASE_URL}/${archive_name}"
  local checksum_var="RNL_BINARY_SHA256_${arch^^}"
  local expected="${RNL_BINARY_SHA256:-${!checksum_var:-}}"
  local tmp
  tmp="$(mktemp -d)"

  step binary_download "$BIN_NAME" "$TAG" "$arch"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] curl -fsSL ${url}"
    echo "[dry-run] install ${PREFIX}/${BIN_NAME}"
    rm -rf "$tmp"
    return 0
  fi

  if ! [[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    echo "Missing reviewed binary SHA-256 for linux/${arch}." >&2
    exit 1
  fi
  curl -fsSL "${url}" -o "${tmp}/${archive_name}"
  printf '%s  %s\n' "${expected,,}" "${tmp}/${archive_name}" | sha256sum -c -
  while IFS= read -r entry; do
    case "$entry" in
      /*|../*|*/../*|*/..) echo "Unsafe binary archive entry: $entry" >&2; exit 1 ;;
    esac
  done < <(tar -tzf "${tmp}/${archive_name}")
  tar -xzf "${tmp}/${archive_name}" -C "${tmp}"
  install -m 0755 "${tmp}/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
  rm -rf "$tmp"

  "${PREFIX}/${BIN_NAME}" version
}

apply_capabilities() {
  step capabilities
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] setcap cap_net_admin+ep ${PREFIX}/${BIN_NAME}"
    return 0
  fi
  if ! command -v setcap >/dev/null 2>&1; then
    echo "警告：未找到 setcap，nftables 插件可能不可用。" >&2
    return 0
  fi
  setcap cap_net_admin+ep "${PREFIX}/${BIN_NAME}"
}

install_xray() {
  if [ "$SKIP_XRAY" -eq 1 ] || [ "$INSTALL_XRAY" -eq 0 ]; then
    rnl_msg skip_xray
    return 0
  fi

  step xray_install
  local dir
  dir="$(script_dir)"
  if [ -n "$dir" ] && [ -f "${dir}/install-xray.sh" ]; then
    bash "${dir}/install-xray.sh"
  else
    echo "Verified installer bundle is missing scripts/install-xray.sh." >&2
    exit 1
  fi
}

setup_directories() {
  step directories
  run mkdir -p "$ETC_DIR" "$DATA_DIR" "$LOG_DIR" /run/remnanode
  run chmod 0755 "$ETC_DIR" "$DATA_DIR" "$LOG_DIR" /run/remnanode
}

setup_env_file() {
  step environment "$NODE_ENV"
  local port
  port="$(effective_node_port)"
  validate_port "$port"

  if [ -f "$NODE_ENV" ]; then
    if [ "$PORT_EXPLICIT" -eq 1 ] || [ -n "${NODE_PORT:-}" ]; then
      update_node_port_in_env "$port"
    else
      if [ "$RNL_LANG" = "en" ]; then
        echo "Keep existing configuration: ${NODE_ENV} (NODE_PORT=$(configured_node_port))"
      else
        echo "保留现有配置：${NODE_ENV}（NODE_PORT=$(configured_node_port)）"
      fi
    fi
    return 0
  fi

  local low_mem="${LOW_MEMORY:-0}"
  if [ "$LOW_MEMORY" -eq 1 ]; then
    low_mem=1
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 创建 ${NODE_ENV}"
    return 0
  fi

  render_env_template "$port" "$low_mem" "install-node-alpine.sh" >"$NODE_ENV"
  chmod 600 "$NODE_ENV"
  if [ "$RNL_LANG" = "en" ]; then echo "Created ${NODE_ENV}"; else echo "已创建 ${NODE_ENV}"; fi
}

setup_secret_file() {
  step secret

  if secret_configured; then
    if secret_from_env_file; then
      echo "保留现有 SECRET_KEY（${NODE_ENV}）"
    else
      echo "保留现有 Secret Key：${SECRET_FILE}"
    fi
    return 0
  fi

  if [ -n "$SECRET_FILE_ARG" ]; then
    if [ ! -f "$SECRET_FILE_ARG" ]; then
      echo "找不到 --secret-file 指定路径：${SECRET_FILE_ARG}" >&2
      exit 1
    fi
    write_secret_from_source "$SECRET_FILE_ARG"
    echo "已从文件导入 Secret Key（SECRET_KEY_FILE 模式）。"
    return 0
  fi

  prompt_secret_key
}

setup_vnstat() {
  step vnstat
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] rc-update add vnstat default"
    echo "[dry-run] rc-service vnstat restart"
    echo "[dry-run] ip route show default"
    echo "[dry-run] vnstat --add -i eth0"
    return 0
  fi

  local service_name="vnstat"
  if [ ! -e /etc/init.d/vnstat ] && [ -e /etc/init.d/vnstatd ]; then
    service_name="vnstatd"
  fi

  rc-update add "$service_name" default

  local iface=""
  iface="$(ip route show default 2>/dev/null | awk '($1 == "default") {for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}')"
  if [ -z "$iface" ]; then
    echo "无法检测默认路由网卡，vnStat 无法配置。" >&2
    exit 1
  fi

  rc-service "$service_name" start 2>/dev/null || rc-service "$service_name" restart 2>/dev/null || true
  sleep 1
  if [ ! -f /var/lib/vnstat/vnstat.db ] && command -v vnstatd >/dev/null 2>&1; then
    vnstatd --initdb --alwaysadd 2>/dev/null || true
  fi
  vnstat --add -i "$iface" 2>/dev/null || true
  rc-service "$service_name" restart 2>/dev/null || rc-service "$service_name" start 2>/dev/null || true

  local year month day i json
  year="$(date +%Y)"
  month="$(date +%m | sed 's/^0//')"
  day="$(date +%d | sed 's/^0//')"
  i=0
  while [ "$i" -lt 30 ]; do
    json="$(vnstat --json d 1 2>/dev/null || true)"
    if printf '%s' "$json" | grep -q "\"year\":${year}" && \
      printf '%s' "$json" | grep -q "\"month\":${month}" && \
      printf '%s' "$json" | grep -q "\"day\":${day}"; then
      rnl_msg vnstat_ready "$iface"
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done

  rnl_msg vnstat_failed "$iface" >&2
  exit 1
}

install_openrc() {
  step openrc
  local dir
  dir="$(script_dir)"

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 安装 ${RUN_WRAPPER} 与 ${OPENRC_SVC}"
    return 0
  fi

  if [ -n "$dir" ] && [ -f "${dir}/../deploy/remnawave-node-run.sh" ]; then
    install -m 0755 "${dir}/../deploy/remnawave-node-run.sh" "$RUN_WRAPPER"
  else
    echo "Verified installer bundle is missing deploy/remnawave-node-run.sh." >&2
    exit 1
  fi

  if [ -n "$dir" ] && [ -f "${dir}/../deploy/remnawave-node.openrc" ]; then
    install -m 0755 "${dir}/../deploy/remnawave-node.openrc" "$OPENRC_SVC"
  else
    echo "Verified installer bundle is missing deploy/remnawave-node.openrc." >&2
    exit 1
  fi

  rc-update add remnawave-node default 2>/dev/null || true
}

install_helpers() {
  step helpers
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] xlogs / xerrors"
    return 0
  fi

  cat >"${PREFIX}/xlogs" <<'EOF'
#!/bin/sh
exec tail -n +1 -f /var/log/remnanode/xray.out.log
EOF
  cat >"${PREFIX}/xerrors" <<'EOF'
#!/bin/sh
exec tail -n +1 -f /var/log/remnanode/xray.err.log
EOF
  chmod +x "${PREFIX}/xlogs" "${PREFIX}/xerrors"
}

start_service() {
  if ! secret_configured; then
    rnl_msg secret_missing
    echo "  请编辑 ${NODE_ENV} 填入 NODE_PORT 与 SECRET_KEY 后：${RESTART_CMD}"
    return 0
  fi

  step start_service
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] ${RESTART_CMD}"
    return 0
  fi

  if [ "${RNL_OPENRC_DIRECT_START:-0}" = "1" ]; then
    if command -v pidof >/dev/null 2>&1; then
      for pid in $(pidof "$BIN_NAME" 2>/dev/null || true); do
        kill "$pid" 2>/dev/null || true
      done
      sleep 1
    fi
    nohup "$RUN_WRAPPER" >>"${LOG_DIR}/openrc.log" 2>>"${LOG_DIR}/openrc.err.log" &
    return 0
  fi

  rc-service remnawave-node restart || rc-service remnawave-node start || true
  sleep 1
  rc-service remnawave-node status || true
}

main() {
  require_root
  require_alpine
  if [ -z "$ACTION" ]; then
    show_menu
  fi
  dispatch_action
}

detect_low_memory_auto() {
  if [ "$LOW_MEMORY" -eq 1 ]; then
    return 0
  fi
  local total_kb=""
  total_kb="$(awk '/MemTotal:/ {print $2}' /proc/meminfo 2>/dev/null || true)"
  if [ -n "$total_kb" ] && [ "$total_kb" -le 524288 ]; then
    LOW_MEMORY=1
    echo "检测到内存 ${total_kb}KB（≤512MB），自动启用低内存模式 LOW_MEMORY=1"
  fi
}

do_install() {
  require_root
  require_alpine

  detect_low_memory_auto

  local arch
  arch="$(detect_arch)"

	install_packages
	confirm_install
	setup_directories
	print_pre_install_panel_hint
  download_binary "$arch"
  apply_capabilities
  install_xray
  install_geo_extra_files
  setup_vnstat
  prompt_node_port
  setup_env_file
  ensure_internal_socket_in_env
  setup_secret_file
  install_openrc
  install_helpers
  start_service
  verify_service_listening "$(configured_node_port)"
  print_panel_address_hint "$(configured_node_port)"

  echo
  rnl_msg install_complete
  rnl_print_install_summary "${PREFIX}/${BIN_NAME}" "$NODE_ENV" "$(configured_node_port)"
  if ! secret_configured; then
    print_env_config_hint "$RESTART_CMD"
  fi
}

main "$@"
