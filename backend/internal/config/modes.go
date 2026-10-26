package config

import (
	"os"
	"path/filepath"
)

// pidDirPinned 是「PID 文件目录由运行模式固定」的取值：非空表示 PID 文件不随
// 运行数据目录变化（openwrt 固定在 /var/run），为空表示 PID 文件与数据目录同址。
var pidDirPinned string

// SetDataDir 设置统一的运行数据目录：
//   - fluxor.log 与全部配置文件（settings.json / rules.json / tunnels.json /
//     subscription-meta.json / tproxy.json / tun.json）都生成在该目录下；
//   - fluxor.pid 与 core.pid 默认也生成在该目录下，但若运行模式已把 PID 目录
//     固定（见 pidDirPinned，openwrt 固定 /var/run），则 PID 文件不受本目录影响。
//
// 旧版本允许通过 FLUXOR_PID_FILE / CORE_PID_FILE / FLUXOR_CONFIG_FILE /
// INFO_LOG_FILE 单独覆盖这些路径，现已移除：面板自身产生的数据必须集中在一处，
// 否则「数据到底落在哪」会随部署方式（是否设置了某个环境变量）漂移，排查问题时
// 无法只凭 FLUXOR_DATA_DIR 推断。
func SetDataDir(dir string) {
	FluxorDataDir = dir
	FluxorLogFile = filepath.Join(dir, "fluxor.log")
	// 新布局：每个类别一个文件（见 config/store.go）
	FluxorSettingsFile = filepath.Join(dir, "settings.json")
	FluxorRulesFile = filepath.Join(dir, "rules.json")
	FluxorTunnelsFile = filepath.Join(dir, "tunnels.json")
	FluxorMetaFile = filepath.Join(dir, "subscription-meta.json")
	FluxorTproxyFile = filepath.Join(dir, "tproxy.json")
	FluxorTunFile = filepath.Join(dir, "tun.json")
	// 旧布局：只读迁移输入，迁移后不再写入
	FluxorConfigFile = filepath.Join(dir, "fluxor.json")

	pidDir := dir
	if pidDirPinned != "" {
		pidDir = pidDirPinned
	}
	FluxorPidFile = filepath.Join(pidDir, "fluxor.pid")
	CorePidFile = filepath.Join(pidDir, "core.pid")
}

// SetDefaults 根据运行模式设置默认路径
func SetDefaults(mode string) {
	var dataDir string
	switch mode {
	case "openwrt":
		SocketPath = ""
		BaseURL = "/"
		// 只绑定回环。面板自身没有任何认证，绑到 0.0.0.0 等于把「改订阅 / 停内核 /
		// 自更新（下载并执行新二进制）」这些写接口开放给同网段任意主机访问。
		// 需要从别的机器访问时，由用户显式指定 -a/--addr 或 FLUXOR_ADDR——
		// 主动写下一个对外地址，比默认对外、出事后再收口安全得多。
		TcpAddr = "127.0.0.1:18080"
		FluxorBinDir = "/etc/fluxor/"
		CoreBin = "/etc/fluxor/mihomo"
		CoreSocket = "/etc/fluxor/core.sock"
		MetaDir = "/etc/fluxor/ui/meta"
		ZashDir = "/etc/fluxor/ui/zash"
		ConfigTarget = "/etc/fluxor/config.yaml"
		CoreWorkDir = "/etc/fluxor"
		dataDir = "/etc/fluxor/"
		// OpenWrt 下 PID 文件固定在 /var/run（tmpfs）：PID 属于纯运行时状态，
		// 不该落到数据目录所在的 flash 上；系统 init 脚本也按该路径查停面板。
		// 因此它不随数据目录（含 FLUXOR_DATA_DIR 覆盖）变化。
		pidDirPinned = "/var/run/"
	default: // fnos 模式（默认）
		SocketPath = "/var/apps/Fluxor/target/app.sock"
		BaseURL = "/app/Fluxor"
		TcpAddr = ""
		FluxorBinDir = "/var/apps/Fluxor/target/bin/"
		CoreBin = "/var/apps/Fluxor/target/bin/mihomo"
		CoreSocket = "/var/apps/Fluxor/target/core.sock"
		MetaDir = "/var/apps/Fluxor/shares/ui/meta"
		ZashDir = "/var/apps/Fluxor/shares/ui/zash"
		ConfigTarget = "/var/apps/Fluxor/shares/Fluxor/config.yaml"
		CoreWorkDir = "/var/apps/Fluxor/shares/Fluxor"
		// 飞牛 OS 的应用框架会注入 TRIM_PKGVAR（应用运行时数据目录，
		// 即 /var/apps/Fluxor/var）；脱离应用框架直接启动时回退到同一路径。
		dataDir = os.Getenv("TRIM_PKGVAR")
		if dataDir == "" {
			dataDir = "/var/apps/Fluxor/var"
		}
		// fnos 下四个文件同址：PID 跟着数据目录走
		pidDirPinned = ""
	}
	SetDataDir(dataDir)
	OriginalBaseURL = BaseURL
}
