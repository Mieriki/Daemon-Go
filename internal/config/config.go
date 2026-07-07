package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DaemonConfig 主守护配置
type DaemonConfig struct {
	Instance        string            `yaml:"instance"`
	LogDir          string            `yaml:"logDir"`
	StateFile       string            `yaml:"stateFile"`
	WebPort         int               `yaml:"webPort"`
	WebHost         string            `yaml:"webHost"`
	Peer            PeerConfig        `yaml:"peer"`
	Projects        []Project         `yaml:"projects"`
	AllowedCommands []AllowedCommand  `yaml:"allowedCommands"`
	AuthToken       string            `yaml:"-"`
}

// AllowedCommand runCommand 允许列表
type AllowedCommand struct {
	Path        string `yaml:"path"`
	ArgsPattern string `yaml:"argsPattern"`
}

// PeerConfig 对端互保配置
type PeerConfig struct {
	InstanceID   string `yaml:"instanceId"`
	ServiceName  string `yaml:"serviceName"`
	CheckInterval int   `yaml:"checkInterval"`
	StartScript  string `yaml:"startScript"`
}

// Project 被守护项目配置
type Project struct {
	Name               string            `yaml:"name"`
	Enabled            bool              `yaml:"enabled"`
	Monitor            MonitorConfig     `yaml:"monitor"`
	CheckInterval      int               `yaml:"checkInterval"`
	StartupTimeout     int               `yaml:"startupTimeout"`
	MaxRestartAttempts int               `yaml:"maxRestartAttempts"`
	RestartDelay       int               `yaml:"restartDelay"`
	StartMode          string            `yaml:"startMode"`
	StartScript        string            `yaml:"startScript"`
	StopScript         string            `yaml:"stopScript"`
	PreActions         []ActionConfig    `yaml:"preActions"`
	PostActions        []ActionConfig    `yaml:"postActions"`
}

// MonitorConfig 监控方式配置
type MonitorConfig struct {
	Type        string `yaml:"type"` // port, process, mixed
	Port        int    `yaml:"port"`
	ProcessName string `yaml:"processName"`
	Host        string `yaml:"host"`
}

// ActionConfig 动作配置
type ActionConfig struct {
	Type   string            `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

// ManagerConfig 管理端配置
type ManagerConfig struct {
	WebHost string `yaml:"webHost"`
	WebPort int    `yaml:"webPort"`
}

// Load 从文件加载配置
func Load(path string) (*DaemonConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config failed: %w", err)
	}
	var cfg DaemonConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

func applyDefaults(cfg *DaemonConfig) {
	if cfg.Instance == "" {
		cfg.Instance = "a"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "daemon.state"
	}
	if cfg.WebHost == "" {
		cfg.WebHost = "127.0.0.1"
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = 18080
	}
	if cfg.Peer.CheckInterval == 0 {
		cfg.Peer.CheckInterval = 10
	}
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		if p.CheckInterval == 0 {
			p.CheckInterval = 15
		}
		if p.StartupTimeout == 0 {
			p.StartupTimeout = 60
		}
		if p.MaxRestartAttempts == 0 {
			p.MaxRestartAttempts = 3
		}
		if p.RestartDelay == 0 {
			p.RestartDelay = 10
		}
		if p.StartMode == "" {
			p.StartMode = "async"
		}
		if p.Monitor.Type == "" {
			p.Monitor.Type = "port"
		}
		if p.Monitor.Host == "" {
			p.Monitor.Host = "127.0.0.1"
		}
	}
}

// ServiceName 获取当前实例对应的服务名
func (c *DaemonConfig) ServiceName() string {
	return fmt.Sprintf("ProcessGuard-%s", c.Instance)
}

// PeerServiceName 获取对端服务名
func (c *DaemonConfig) PeerServiceName() string {
	if c.Peer.ServiceName != "" {
		return c.Peer.ServiceName
	}
	return fmt.Sprintf("ProcessGuard-%s", c.Peer.InstanceID)
}

// Save 保存配置到 YAML 文件
func (c *DaemonConfig) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config failed: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// SaveState 保存状态
func (c *DaemonConfig) SaveState(state string) error {
	return os.WriteFile(c.StateFile, []byte(state+"\n"), 0644)
}

// LoadState 加载状态
func (c *DaemonConfig) LoadState() string {
	data, err := os.ReadFile(c.StateFile)
	if err != nil {
		return "RUNNING"
	}
	s := string(data)
	if s == "PAUSED\n" || s == "PAUSED" {
		return "PAUSED"
	}
	return "RUNNING"
}
