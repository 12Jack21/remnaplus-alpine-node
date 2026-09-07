package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/12Jack21/remnaplus-alpine-node/internal/config"
	"github.com/12Jack21/remnaplus-alpine-node/internal/netadmin"
	"github.com/12Jack21/remnaplus-alpine-node/internal/version"
)

const defaultEnvPath = "/etc/remnanode/node.env"
const defaultOpenRCServicePath = "/etc/init.d/remnawave-node"
const defaultSystemdServicePath = "/etc/systemd/system/remnawave-node.service"

type result struct {
	level   string
	title   string
	detail  string
	fixHint string
}

// Run performs deployment health checks and returns exit code 0 (ok) or 1 (errors).
func Run(args []string) int {
	envPath := defaultEnvPath
	for i := 0; i < len(args); i++ {
		if args[i] == "--env" && i+1 < len(args) {
			envPath = args[i+1]
			i++
		}
	}
	if override := strings.TrimSpace(os.Getenv("REMNANODE_ENV")); override != "" {
		envPath = override
	}

	fmt.Println(version.String())
	fmt.Println("── 部署自检 ──")

	var results []result

	results = append(results, checkServiceManager(defaultOpenRCServicePath, defaultSystemdServicePath))
	results = append(results, checkCapNetAdmin())

	cfg, cfgErr := loadConfig(envPath)
	if cfgErr != nil {
		results = append(results, result{
			level:   "ERROR",
			title:   "配置文件",
			detail:  cfgErr.Error(),
			fixHint: "创建 " + envPath + " 或指定 --env PATH",
		})
	} else {
		results = append(results, checkSecret(cfg)...)
		results = append(results, checkXrayBinary(cfg.XrayBin)...)
		results = append(results, checkGeoFiles(cfg.GeoDir)...)
		results = append(results, checkASNDatabase(cfg.ASNDBPath)...)
		results = append(results, checkPersistedStart(cfg.DataDir)...)
		usesSystemd := fileExists(defaultSystemdServicePath)
		results = append(results, checkCommand("nft", "nftables 命令行（插件 IP 封禁）", usesSystemd)...)
		results = append(results, checkCommand("ss", "ss 命令（踢连接 drop-ips）", usesSystemd)...)
	}

	exitCode := 0
	for _, item := range results {
		fmt.Printf("[%s] %s", item.level, item.title)
		if item.detail != "" {
			fmt.Printf(" — %s", item.detail)
		}
		fmt.Println()
		if item.fixHint != "" {
			fmt.Printf("      → %s\n", item.fixHint)
		}
		if item.level == "ERROR" {
			exitCode = 1
		}
	}

	if exitCode == 0 {
		fmt.Println("── 结论：核心项通过（WARN 项不影响 Panel 基本连接）──")
	} else {
		fmt.Println("── 结论：存在 ERROR，请先修复后再接入 Panel ──")
	}
	return exitCode
}

func loadConfig(envPath string) (config.Config, error) {
	if _, err := os.Stat(envPath); err != nil {
		if envPath != ".env" {
			if _, err2 := os.Stat(".env"); err2 == nil {
				return config.Load(".env")
			}
		}
		return config.Config{}, fmt.Errorf("找不到 %s", envPath)
	}
	return config.Load(envPath)
}

func checkCapNetAdmin() result {
	if netadmin.HasCapNetAdmin() {
		return result{level: "OK", title: "CAP_NET_ADMIN", detail: "当前进程已具备"}
	}
	return result{
		level:   "WARN",
		title:   "CAP_NET_ADMIN",
		detail:  "当前进程未具备（nftables / ss -K 不可用）",
		fixHint: "运行 setcap cap_net_admin+ep /usr/local/bin/remnanode-lite，然后 rc-service remnawave-node restart",
	}
}

func checkServiceManager(openRCPath, systemdPath string) result {
	if data, err := os.ReadFile(systemdPath); err == nil {
		content := string(data)
		if strings.Contains(content, "ExecStart=/usr/local/bin/remnanode-lite") {
			return result{level: "OK", title: "systemd service", detail: systemdPath + " 已安装"}
		}
		return result{
			level:   "WARN",
			title:   "systemd service",
			detail:  systemdPath + " 不是预期的 systemd 服务文件",
			fixHint: "重新运行固定版本 install-node.sh，然后 systemctl restart remnawave-node",
		}
	}

	data, err := os.ReadFile(openRCPath)
	if err != nil {
		return result{
			level:   "WARN",
			title:   "Service manager",
			detail:  "未找到 systemd 或 OpenRC 服务文件",
			fixHint: "重新运行适用于当前系统的固定版本安装脚本",
		}
	}
	content := string(data)
	if strings.Contains(content, "#!/sbin/openrc-run") && strings.Contains(content, "remnawave-node-run") {
		return result{level: "OK", title: "OpenRC service", detail: openRCPath + " 已安装"}
	}
	return result{
		level:   "WARN",
		title:   "OpenRC service",
		detail:  openRCPath + " 不是预期的 OpenRC 服务文件",
		fixHint: "重新运行固定版本 upgrade.sh 刷新 OpenRC 服务，然后 rc-service remnawave-node restart",
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func checkSecret(cfg config.Config) []result {
	if strings.TrimSpace(cfg.SecretKey) != "" {
		return []result{{level: "OK", title: "Secret Key", detail: "已配置"}}
	}
	return []result{{
		level:   "ERROR",
		title:   "Secret Key",
		detail:  "未配置（SECRET_KEY 或 SECRET_KEY_FILE 为空）",
		fixHint: "编辑 /etc/remnanode/secret.key 粘贴 Panel 下发的 Key，然后 rc-service remnawave-node restart",
	}}
}

func checkPersistedStart(dataDir string) []result {
	if dataDir == "" {
		dataDir = "/var/lib/remnanode"
	}
	path := filepath.Join(dataDir, "last-start.json")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []result{{
				level:   "WARN",
				title:   "重启自动恢复",
				detail:  path + " 不存在",
				fixHint: "Panel 启用节点一次（若装完仍离线则禁用→启用）；成功 xray/start 后生成 last-start.json，reboot 即可自动恢复 rw-core",
			}}
		}
		return []result{{
			level:  "WARN",
			title:  "重启自动恢复",
			detail: "无法读取 " + path + ": " + err.Error(),
		}}
	}
	return []result{{
		level:  "OK",
		title:  "重启自动恢复",
		detail: fmt.Sprintf("%s 存在 (%d bytes)", path, info.Size()),
	}}
}

func checkXrayBinary(bin string) []result {
	if bin == "" {
		bin = "/usr/local/bin/rw-core"
	}
	info, err := os.Stat(bin)
	if err != nil {
		return []result{{
			level:   "ERROR",
			title:   "rw-core",
			detail:  bin + " 不存在",
			fixHint: "运行 scripts/install-xray.sh 或 install-node-alpine.sh（勿加 --skip-xray）",
		}}
	}
	if info.Mode()&0o111 == 0 {
		return []result{{
			level:   "ERROR",
			title:   "rw-core",
			detail:  bin + " 不可执行",
			fixHint: "sudo chmod +x " + bin,
		}}
	}
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return []result{{
			level:  "WARN",
			title:  "rw-core",
			detail: bin + " 存在但 version 命令失败",
		}}
	}
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return []result{{level: "OK", title: "rw-core", detail: line}}
}

func checkGeoFiles(dir string) []result {
	if dir == "" {
		dir = "/usr/local/share/xray"
	}
	var missing []string
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		detail := dir + " 含 geoip.dat / geosite.dat"
		var extras []string
		for _, name := range []string{"geo-zapret.dat", "ip-zapret.dat"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				extras = append(extras, name)
			}
		}
		if len(extras) > 0 {
			detail += "；可选 " + strings.Join(extras, ", ")
		}
		return []result{{level: "OK", title: "Geo 数据", detail: detail}}
	}
	return []result{{
		level:   "WARN",
		title:   "Geo 数据",
		detail:  "缺少 " + strings.Join(missing, ", "),
		fixHint: "重新运行 install-xray.sh 或从 Xray 发行版复制到 " + dir,
	}}
}

func checkASNDatabase(path string) []result {
	if path == "" {
		path = "/usr/local/share/asn/asn-prefixes.bin"
	}
	if _, err := os.Stat(path); err != nil {
		return []result{{
			level:   "WARN",
			title:   "ASN 数据库",
			detail:  path + " 不存在（插件 asList 共享列表降级为空）",
			fixHint: "设置 ASN_DB_URL 重跑 install-xray.sh，或用 cmd/asn-builder 生成后放到该路径",
		}}
	}
	return []result{{level: "OK", title: "ASN 数据库", detail: path}}
}

func checkCommand(name, purpose string, usesSystemd bool) []result {
	if path, err := exec.LookPath(name); err == nil {
		return []result{{level: "OK", title: name, detail: path + "（" + purpose + "）"}}
	}
	packageName := name
	if name == "ss" {
		packageName = "iproute2"
	}
	fixHint := "Alpine: apk add --no-cache " + packageName
	if usesSystemd {
		fixHint = "Debian: apt-get install -y " + packageName
	}
	return []result{{
		level:   "WARN",
		title:   name,
		detail:  "未安装（" + purpose + "）",
		fixHint: fixHint,
	}}
}
