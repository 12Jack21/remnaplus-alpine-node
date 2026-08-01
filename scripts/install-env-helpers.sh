# shellcheck shell=bash
# Shared env/secret helpers for Alpine install scripts
# Expects: NODE_ENV, SECRET_FILE, DRY_RUN

resolve_install_tag() {
  local _repo="${1:?}"
  local fallback="${2:?}"
  if [ -n "${RNL_TAG:-}" ]; then
    printf '%s' "$RNL_TAG"
  else
    printf '%s' "$fallback"
  fi
}

secret_from_env_file() {
  if [ ! -f "$NODE_ENV" ]; then
    return 1
  fi
  local line val
  line="$(grep -E '^[[:space:]]*SECRET_KEY=' "$NODE_ENV" 2>/dev/null | head -n 1 || true)"
  [ -n "$line" ] || return 1
  val="${line#SECRET_KEY=}"
  val="$(printf '%s' "$val" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")"
  [ -n "$val" ]
}

secret_configured() {
  if secret_from_env_file; then
    return 0
  fi
  [ -f "$SECRET_FILE" ] && [ -s "$SECRET_FILE" ]
}

write_secret_to_env() {
  local value="$1"
  if [ -z "$value" ]; then
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 写入 ${NODE_ENV} SECRET_KEY=..."
    return 0
  fi
  if [ ! -f "$NODE_ENV" ]; then
    echo "找不到 ${NODE_ENV}，请先创建环境配置。" >&2
    exit 1
  fi
  local tmp
  tmp="$(mktemp)"
  grep -v '^SECRET_KEY=' "$NODE_ENV" | grep -v '^SECRET_KEY_FILE=' >"$tmp" || true
  {
    cat "$tmp"
    printf 'SECRET_KEY="%s"\n' "$value"
  } >"$NODE_ENV"
  rm -f "$tmp"
  chmod 600 "$NODE_ENV"
  rnl_msg secret_saved "$NODE_ENV"
}

enable_secret_key_file() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 启用 ${NODE_ENV} SECRET_KEY_FILE=${SECRET_FILE}"
    return 0
  fi
  if [ ! -f "$NODE_ENV" ]; then
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  grep -v '^SECRET_KEY=' "$NODE_ENV" | grep -v '^SECRET_KEY_FILE=' | grep -v '^# SECRET_KEY_FILE=' >"$tmp" || true
  {
    cat "$tmp"
    echo "SECRET_KEY="
    echo "SECRET_KEY_FILE=${SECRET_FILE}"
  } >"$NODE_ENV"
  rm -f "$tmp"
  chmod 600 "$NODE_ENV"
}

write_secret_from_source() {
  local src="$1"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 写入 ${SECRET_FILE} <- ${src}"
    return 0
  fi
  install -m 0600 -D /dev/null "$SECRET_FILE"
  if [ "$src" = "-" ]; then
    cat >"$SECRET_FILE"
  else
    install -m 0600 "$src" "$SECRET_FILE"
  fi
  enable_secret_key_file
}

write_secret_from_env() {
  local value="${SECRET_KEY:-}"
  if [ -z "$value" ]; then
    return 0
  fi
  write_secret_to_env "$value"
}

ensure_internal_socket_in_env() {
  if [ ! -f "$NODE_ENV" ] || [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if grep -q '^INTERNAL_SOCKET_PATH=.' "$NODE_ENV" 2>/dev/null; then
    return 0
  fi
  if grep -q '^INTERNAL_SOCKET_PATH=' "$NODE_ENV" 2>/dev/null; then
    sed -i 's|^INTERNAL_SOCKET_PATH=.*|INTERNAL_SOCKET_PATH=/run/remnanode/internal.sock|' "$NODE_ENV"
  else
    echo 'INTERNAL_SOCKET_PATH=/run/remnanode/internal.sock' >>"$NODE_ENV"
  fi
}

prompt_secret_key() {
  if secret_configured; then
    return 0
  fi

  write_secret_from_env
  if secret_configured; then
    return 0
  fi

  if [ -n "$SECRET_FILE_ARG" ]; then
    return 0
  fi

  if [ "$YES" -eq 1 ] || [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi

  echo
  rnl_msg secret_prompt
  if [ "${RNL_LANG:-zh}" = "en" ]; then
    echo "If the Node is enabled, the Panel should bring it online about 10 seconds after installation."
  else
    echo "（节点已启用时，装完后约 10s 内 Panel 将自动上线）"
  fi
  local secret=""
  if [ -t 0 ]; then
    read -r secret
  elif [ -r /dev/tty ]; then
    read -r secret </dev/tty
  fi

  if [ -n "$secret" ]; then
    write_secret_to_env "$secret"
    return 0
  fi

  print_env_config_hint "${RESTART_CMD:-rc-service remnawave-node restart}"
}

cleanup_runtime() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] cleanup runtime sockets and rw-core"
    return 0
  fi
  pkill -x rw-core 2>/dev/null || pkill -f '/usr/local/bin/rw-core' 2>/dev/null || true
  rm -rf /run/remnanode 2>/dev/null || true
  rm -f /run/remnawave-internal-*.sock 2>/dev/null || true
}

print_pre_install_panel_hint() {
  rnl_print_pre_install_hint
}

print_panel_address_hint() {
  local port="$1"
  local pub_ip=""
  pub_ip="$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -n1 || true)"

  rnl_print_panel_hint "$port" "$pub_ip"
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
      if [ "${RNL_OPENRC_DIRECT_START:-0}" = "1" ]; then
        return 0
      fi
      if command -v rc-service >/dev/null 2>&1; then
        if rc-service remnawave-node status 2>/dev/null | grep -qi 'started'; then
          return 0
        fi
      else
        return 0
      fi
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}

verify_service_listening() {
  local port="$1"
  if [ "$DRY_RUN" -eq 1 ]; then
    return 0
  fi
  if ! wait_for_service_stable "$port" 30; then
    rnl_msg service_failed "$port" >&2
    return 1
  fi
  if ss -tln 2>/dev/null | grep -q ":${port} "; then
    rnl_msg service_ready "$port"
    ss -tlnp 2>/dev/null | grep ":${port} " | head -n1 || true
    return 0
  fi
  echo "错误: :${port} 未监听，请检查服务状态（rc-service remnawave-node）" >&2
  return 1
}

print_env_config_hint() {
  local restart_cmd="$1"
  rnl_print_env_hint "$NODE_ENV" "$restart_cmd"
}

read_env_value() {
  local key="$1" file="$2"
  local line val
  [ -f "$file" ] || return 0
  line="$(grep -E "^[[:space:]]*${key}=" "$file" 2>/dev/null | head -n 1 || true)"
  [ -n "$line" ] || return 0
  val="${line#*=}"
  val="$(printf '%s' "$val" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")"
  [ -n "$val" ] || return 0
  printf '%s' "$val"
}

install_geo_extra_files() {
  local geo_dir="${GEO_DIR:-/usr/local/share/xray}"
  local env_file="${NODE_ENV:-/etc/remnanode/node.env}"
  local geo_zapret ip_zapret
  if [ -z "${GEO_ZAPRET_FILE:-}" ]; then
    geo_zapret="$(read_env_value GEO_ZAPRET_FILE "$env_file")"
  else
    geo_zapret="$GEO_ZAPRET_FILE"
  fi
  if [ -z "${IP_ZAPRET_FILE:-}" ]; then
    ip_zapret="$(read_env_value IP_ZAPRET_FILE "$env_file")"
  else
    ip_zapret="$IP_ZAPRET_FILE"
  fi

  local copied=0
  install_one_geo_extra() {
    local src="$1" dest_name="$2"
    [ -n "$src" ] || return 0
    [ -f "$src" ] || { echo "警告：找不到 ${src}（跳过 ${dest_name}）" >&2; return 0; }
    if [ "$DRY_RUN" -eq 1 ]; then
      echo "[dry-run] 复制 ${src} -> ${geo_dir}/${dest_name}"
      return 0
    fi
    mkdir -p "$geo_dir"
    cp -f "$src" "${geo_dir}/${dest_name}"
    chmod 0644 "${geo_dir}/${dest_name}"
    echo "已安装 ${dest_name} -> ${geo_dir}/${dest_name}"
    copied=1
  }

  install_one_geo_extra "$geo_zapret" "geo-zapret.dat"
  install_one_geo_extra "$ip_zapret" "ip-zapret.dat"

  if [ "$copied" -eq 0 ]; then
    return 0
  fi
  echo "提示：Xray 路由使用 ext:geo-zapret.dat:zapret / ext:ip-zapret.dat:zapret 引用上述文件。"
}

render_env_template() {
  local port="$1"
  local low_mem="$2"
  local installer="$3"
  if [ "${RNL_LANG:-zh}" = "en" ]; then
    cat <<EOF
# Generated by ${installer} for Remnawave Node Lite
NODE_PORT=${port}
SECRET_KEY=
# SECRET_KEY_FILE=${SECRET_FILE}
XRAY_BIN=/usr/local/bin/rw-core
GEO_DIR=/usr/local/share/xray
LOG_DIR=${LOG_DIR}
INTERNAL_SOCKET_PATH=/run/remnanode/internal.sock
INTERNAL_REST_TOKEN=
LOW_MEMORY=${low_mem}
BODY_LIMIT_MB=
# CUSTOM_CORE_URL=https://example.com/xray-custom
# GEO_ZAPRET_FILE=/path/to/geo-zapret.dat
# IP_ZAPRET_FILE=/path/to/ip-zapret.dat
EOF
    return 0
  fi
  cat <<EOF
# Remnawave Node Lite — 由 ${installer} 生成
# 借鉴官方 environment 变量名，仅需修改下面两项：

NODE_PORT=${port}
SECRET_KEY=

# 可选：密钥极长时可改用独立文件（取消下行注释并清空 SECRET_KEY）
# SECRET_KEY_FILE=${SECRET_FILE}

XRAY_BIN=/usr/local/bin/rw-core
GEO_DIR=/usr/local/share/xray
LOG_DIR=${LOG_DIR}
INTERNAL_SOCKET_PATH=/run/remnanode/internal.sock
INTERNAL_REST_TOKEN=
LOW_MEMORY=${low_mem}
BODY_LIMIT_MB=

# 可选：自定义 rw-core 下载 URL（对齐官方 CUSTOM_CORE_URL）
# CUSTOM_CORE_URL=https://example.com/xray-custom

# 可选：zapret 规则文件（复制到 GEO_DIR，供 ext:geo-zapret.dat 引用）
# GEO_ZAPRET_FILE=/path/to/geo-zapret.dat
# IP_ZAPRET_FILE=/path/to/ip-zapret.dat
EOF
}
