#!/usr/bin/env bash
# Shared zh/en messages for the Alpine installer family.

case "${RNL_LANG:-zh}" in
  zh|en) ;;
  *)
    echo "Unsupported installer language: ${RNL_LANG:-}" >&2
    return 64 2>/dev/null || exit 64
    ;;
esac

rnl_msg() {
  local key="${1:?}"
  shift
  case "${RNL_LANG}:${key}" in
    zh:missing_curl) printf '缺少命令：curl（Alpine: apk add --no-cache curl bash）\n' ;;
    en:missing_curl) printf 'Missing command: curl (Alpine: apk add --no-cache curl bash)\n' ;;
    zh:missing_command) printf '缺少命令：%s\n' "$@" ;;
    en:missing_command) printf 'Missing command: %s\n' "$@" ;;
    zh:argument_requires_value)
      if [ "${2:-}" = "port" ]; then printf '%s 需要端口号\n' "$1"; else printf '%s 需要文件路径\n' "$1"; fi
      ;;
    en:argument_requires_value)
      if [ "${2:-}" = "port" ]; then printf '%s requires a port\n' "$1"; else printf '%s requires a file path\n' "$1"; fi
      ;;
    zh:unknown_argument) printf '未知参数：%s\n' "$@" ;;
    en:unknown_argument) printf 'Unknown argument: %s\n' "$@" ;;
    zh:install_failed) printf '安装失败：%s\n' "$@" ;;
    en:install_failed) printf 'Installation failed: %s\n' "$@" ;;
    zh:failed_command) printf '失败命令：%s\n' "$@" ;;
    en:failed_command) printf 'Failed command: %s\n' "$@" ;;
    zh:invalid_port) printf '无效端口：%s（端口必须为 1-65535）\n' "$@" ;;
    en:invalid_port) printf 'Invalid port: %s (Port must be between 1 and 65535)\n' "$@" ;;
    zh:root_required) printf '请使用 root 运行（Alpine 通常无 sudo）。\n' ;;
    en:root_required) printf 'Run this installer as root (Alpine usually has no sudo).\n' ;;
    zh:alpine_required) printf '此脚本仅适用于 Alpine Linux（未找到 /etc/alpine-release）。\n' ;;
    en:alpine_required) printf 'This installer supports Alpine Linux only (/etc/alpine-release was not found).\n' ;;
    zh:standard_node_hint) printf '其它系统请在 RemnaPlus 中选择标准 Docker 节点。\n' ;;
    en:standard_node_hint) printf 'For other systems, select a Standard Docker Node in RemnaPlus.\n' ;;
    zh:packages) printf '安装 Alpine 依赖包' ;;
    en:packages) printf 'Install Alpine dependencies' ;;
    zh:binary_download) printf '下载 %s %s (linux/%s)' "$@" ;;
    en:binary_download) printf 'Download %s %s (linux/%s)' "$@" ;;
    zh:capabilities) printf '授予 CAP_NET_ADMIN（nftables / ss -K）' ;;
    en:capabilities) printf 'Grant CAP_NET_ADMIN (nftables / ss -K)' ;;
    zh:xray_install) printf '安装 rw-core (Xray core)' ;;
    en:xray_install) printf 'Install rw-core (Xray core)' ;;
    zh:directories) printf '创建目录' ;;
    en:directories) printf 'Create directories' ;;
    zh:environment) printf '配置 %s' "$@" ;;
    en:environment) printf 'Configure %s' "$@" ;;
    zh:secret) printf '配置 Secret Key' ;;
    en:secret) printf 'Configure Secret Key' ;;
    zh:vnstat) printf '配置 vnStat 默认网卡' ;;
    en:vnstat) printf 'Configure the default vnStat interface' ;;
    zh:openrc) printf '安装 OpenRC 服务' ;;
    en:openrc) printf 'Install the OpenRC service' ;;
    zh:helpers) printf '安装日志辅助命令' ;;
    en:helpers) printf 'Install log helper commands' ;;
    zh:start_service) printf '启动 remnawave-node 服务' ;;
    en:start_service) printf 'Start the remnawave-node service' ;;
    zh:skip_xray) printf '跳过 rw-core 安装。\n' ;;
    en:skip_xray) printf 'Skip rw-core installation.\n' ;;
    zh:install_complete) printf 'Alpine 安装完成。\n' ;;
    en:install_complete) printf 'Alpine installation complete.\n' ;;
    zh:secret_missing) printf '警告：Secret Key 未配置，跳过启动服务。\n' ;;
    en:secret_missing) printf 'Warning: Secret Key is not configured; service startup is skipped.\n' ;;
    zh:secret_prompt) printf '请粘贴 Panel 节点页下发的 Secret Key（整段 base64，粘贴后按 Enter）：\n' ;;
    en:secret_prompt) printf 'Paste the full base64 Secret Key from the Panel Node page, then press Enter:\n' ;;
    zh:secret_saved) printf '已写入 SECRET_KEY 到 %s\n' "$@" ;;
    en:secret_saved) printf 'Saved SECRET_KEY to %s\n' "$@" ;;
    zh:vnstat_ready) printf 'OK: vnStat 正在跟踪 %s\n' "$@" ;;
    en:vnstat_ready) printf 'OK: vnStat is tracking %s\n' "$@" ;;
    zh:vnstat_failed) printf 'vnStat 数据未在 30s 内更新，请检查 vnstat 服务和接口 %s。\n' "$@" ;;
    en:vnstat_failed) printf 'vnStat data did not update within 30 seconds; check the service and interface %s.\n' "$@" ;;
    zh:service_ready) printf 'OK: TCP :%s 已监听\n' "$@" ;;
    en:service_ready) printf 'OK: TCP :%s is listening\n' "$@" ;;
    zh:service_failed) printf '错误: :%s 在 30s 内未就绪，请检查 OpenRC 服务 remnawave-node\n' "$@" ;;
    en:service_failed) printf 'Error: :%s was not ready within 30 seconds; check the remnawave-node OpenRC service\n' "$@" ;;
    zh:unsupported_arch) printf '不支持的架构：%s\n' "$@" ;;
    en:unsupported_arch) printf 'Unsupported architecture: %s\n' "$@" ;;
    zh:custom_core_download) printf 'CUSTOM_CORE_URL 已设置，从自定义地址下载 rw-core：%s\n' "$@" ;;
    en:custom_core_download) printf 'CUSTOM_CORE_URL is set; downloading rw-core from %s\n' "$@" ;;
    zh:custom_core_installed) printf '自定义 rw-core 已安装到 %s\n' "$@" ;;
    en:custom_core_installed) printf 'Custom rw-core installed at %s\n' "$@" ;;
    zh:xray_download) printf '下载 rw-core %s (%s)...\n' "$@" ;;
    en:xray_download) printf 'Download rw-core %s (%s)...\n' "$@" ;;
    zh:asn_skip) printf '提示：未设置 ASN_DB_URL，跳过 ASN 数据库安装。\n' ;;
    en:asn_skip) printf 'ASN_DB_URL is not set; skip ASN database installation.\n' ;;
    zh:asn_download) printf '下载 ASN 前缀数据库：%s\n' "$@" ;;
    en:asn_download) printf 'Download the ASN prefix database: %s\n' "$@" ;;
    zh:asn_installed) printf 'ASN 数据库已安装到 %s\n' "$@" ;;
    en:asn_installed) printf 'ASN database installed at %s\n' "$@" ;;
    zh:xray_complete) printf 'rw-core 安装完成。\n' ;;
    en:xray_complete) printf 'rw-core installation complete.\n' ;;
    zh:download_tool_missing) printf '缺少 curl 或 wget，无法下载 %s\n' "$@" ;;
    en:download_tool_missing) printf 'curl or wget is required to download %s\n' "$@" ;;
    zh:not_executable) printf '下载完成但 %s 不可执行\n' "$@" ;;
    en:not_executable) printf 'Downloaded %s is not executable\n' "$@" ;;
    zh:asn_download_failed) printf '警告：ASN 数据库下载失败，asList 将降级为空。\n' ;;
    en:asn_download_failed) printf 'Warning: ASN database download failed; asList will use an empty fallback.\n' ;;
    zh:asn_tool_missing) printf '警告：缺少 curl/wget，跳过 ASN 数据库下载。\n' ;;
    en:asn_tool_missing) printf 'Warning: curl/wget is missing; skip ASN database download.\n' ;;
    zh:xray_version) printf 'rw-core 版本：%s\n' "$@" ;;
    en:xray_version) printf 'rw-core version: %s\n' "$@" ;;
    zh:xray_missing) printf '警告：/usr/local/bin/rw-core 未找到，请检查安装日志。\n' ;;
    en:xray_missing) printf 'Warning: /usr/local/bin/rw-core was not found; check the installation log.\n' ;;
    zh:geo_missing) printf '警告：缺少 %s\n' "$@" ;;
    en:geo_missing) printf 'Warning: missing %s\n' "$@" ;;
    *) printf 'Missing installer message: %s\n' "$key" >&2; return 65 ;;
  esac
}

rnl_print_usage() {
  local version="${1:?}" repo="${2:?}"
  if [ "$RNL_LANG" = "zh" ]; then
    cat <<EOF
用法：install-node-alpine.sh [选项]

Remnawave Node Lite (Go) ${version} - Alpine / OpenRC 安装、升级与卸载

动作：
  --install           安装
  --upgrade           升级
  --uninstall         卸载

选项：
  --yes, -y           跳过确认
  --dry-run           预览
  --skip-xray         跳过 rw-core
  --low-memory        低内存模式
  --port PORT         监听端口（端口必须为 1-65535，默认 2222）
  --secret-file PATH  从文件导入 Secret Key
  --help, -h          帮助
  --version           版本

安装会配置 vnStat 与 OpenRC。安装完成时会验证监听端口；安装失败会显示阶段与命令。
固定入口：https://raw.githubusercontent.com/${repo}/v${version}/scripts/install-node-alpine.sh
EOF
  else
    cat <<EOF
Usage: install-node-alpine-en.sh [options]

Remnawave Node Lite (Go) ${version} - Alpine / OpenRC install, upgrade, and uninstall

Actions:
  --install           Install
  --upgrade           Upgrade
  --uninstall         Uninstall

Options:
  --yes, -y           Skip confirmation
  --dry-run           Preview changes
  --skip-xray         Skip rw-core
  --low-memory        Low-memory mode
  --port PORT         Listener port (Port must be between 1 and 65535; default 2222)
  --secret-file PATH  Import the Secret Key from a file
  --help, -h          Help
  --version           Version

Installation configures vnStat and OpenRC. Installation complete includes a listener check; Installation failed reports the stage and command.
Pinned entry: https://raw.githubusercontent.com/${repo}/v${version}/scripts/install-node-alpine-en.sh
EOF
  fi
}

rnl_print_xray_usage() {
  if [ "$RNL_LANG" = "zh" ]; then
    cat <<'EOF'
用法：install-xray.sh [--version VERSION] [--upstream REPO] [--dry-run]
安装 rw-core、geo 数据以及可选 ASN 数据库。
EOF
  else
    cat <<'EOF'
Usage: install-xray.sh [--version VERSION] [--upstream REPO] [--dry-run]
Install rw-core, geo data, and the optional ASN database.
EOF
  fi
}

rnl_print_main_menu() {
  local version="${1:?}"
  if [ "$RNL_LANG" = "zh" ]; then
    printf '\nRemnawave Node Lite %s (contract 2.8.0) - Alpine\n  1) 安装\n  2) 升级\n  3) 卸载\n  4) 退出\n\n' "$version"
  else
    printf '\nRemnawave Node Lite %s (contract 2.8.0) - Alpine\n  1) Install\n  2) Upgrade\n  3) Uninstall\n  4) Exit\n\n' "$version"
  fi
}

rnl_print_uninstall_menu() {
  if [ "$RNL_LANG" = "zh" ]; then
    printf '\n卸载选项：\n  1) 仅卸服务（保留 node.env / rw-core）\n  2) 完全卸载（配置、日志与 rw-core）\n  3) 返回\n'
  else
    printf '\nUninstall options:\n  1) Remove service only (keep node.env / rw-core)\n  2) Full uninstall (configuration, logs, and rw-core)\n  3) Back\n'
  fi
}

rnl_print_existing_install_menu() {
  local node_env="${1:?}"
  if [ "$RNL_LANG" = "zh" ]; then
    printf '\n检测到本机已安装 remnawave-node-lite。\n  1) 升级（保留 %s）\n  2) 全新安装（删除配置/日志后重装）\n  3) 取消\n' "$node_env"
  else
    printf '\nremnawave-node-lite is already installed.\n  1) Upgrade (keep %s)\n  2) Clean install (remove configuration and logs)\n  3) Cancel\n' "$node_env"
  fi
}

rnl_print_pre_install_hint() {
  if [ "$RNL_LANG" = "zh" ]; then
    cat <<'EOF'

-------- Panel 接入提示 --------
  1) 在 Panel 创建节点并复制 Secret Key。
  2) 使用该 Secret Key 完成本脚本安装。
  3) TCP 监听就绪后，在 Panel 启用节点。

若超过 30 秒仍离线，请检查防火墙，或将节点禁用后重新启用。
-------------------------------
EOF
  else
    cat <<'EOF'

-------- Panel connection checklist --------
  1) Create the Node in the Panel and copy its Secret Key.
  2) Complete this installation with that Secret Key.
  3) After the TCP listener is ready, enable the Node in the Panel.

If it stays offline for over 30 seconds, check the firewall or toggle the Node off and on once.
--------------------------------------------
EOF
  fi
}

rnl_print_panel_hint() {
  local port="${1:?}" public_ip="${2:-}"
  if [ "$RNL_LANG" = "zh" ]; then
    printf '\n-------- Panel 对接 --------\n  节点端口：%s\n' "$port"
    [ -z "$public_ip" ] || printf '  检测到的公网 IP（参考）：%s\n' "$public_ip"
    printf '  在 Panel 主机测试：nc -zv -w 5 <节点IP> %s\n  节点已就绪，OpenRC 会在重启后自动恢复。\n----------------------------\n' "$port"
  else
    printf '\n-------- Panel connection --------\n  Node port: %s\n' "$port"
    [ -z "$public_ip" ] || printf '  Detected public IP (reference): %s\n' "$public_ip"
    printf '  From the Panel host, test: nc -zv -w 5 <node-ip> %s\n  The Node is ready and will persist across reboot through OpenRC.\n----------------------------------\n' "$port"
  fi
}

rnl_print_env_hint() {
  local node_env="${1:?}" restart_cmd="${2:?}"
  if [ "$RNL_LANG" = "zh" ]; then
    cat <<EOF

----------------------------------------
 在 ${node_env} 配置节点
----------------------------------------
设置 NODE_PORT 与 SECRET_KEY，然后执行：${restart_cmd}
也可在安装时传入：
  SECRET_KEY='eyJ...' NODE_PORT=8443 bash install-*.sh --yes
EOF
  else
    cat <<EOF

----------------------------------------
 Configure the Node in ${node_env}
----------------------------------------
Set NODE_PORT and SECRET_KEY, then run: ${restart_cmd}
You may also pass them during install:
  SECRET_KEY='eyJ...' NODE_PORT=8443 bash install-*.sh --yes
EOF
  fi
}

rnl_print_install_summary() {
  local binary="${1:?}" node_env="${2:?}" port="${3:?}"
  if [ "$RNL_LANG" = "zh" ]; then
    printf '  二进制：    %s\n  环境配置：  %s\n  监听端口：  %s（Panel 须填相同内网端口）\n  服务管理：  rc-service remnawave-node {start|stop|restart|status}\n  日志：      tail -f /var/log/remnanode/openrc.log\n  Xray：      xlogs / xerrors\n  管理：      再次运行 install-node-alpine.sh 可升级或卸载\n' "$binary" "$node_env" "$port"
  else
    printf '  Binary:        %s\n  Environment:   %s\n  Listener port: %s (use the same private port in the Panel)\n  OpenRC:        rc-service remnawave-node {start|stop|restart|status}\n  Logs:          tail -f /var/log/remnanode/openrc.log\n  Xray logs:     xlogs / xerrors\n  Management:    run install-node-alpine-en.sh again to upgrade or uninstall\n' "$binary" "$node_env" "$port"
  fi
}
