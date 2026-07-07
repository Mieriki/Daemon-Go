package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
instance: b
webPort: 18081
peer:
  checkInterval: 5
projects:
  - name: TestApp
    enabled: true
    monitor:
      type: port
      port: 8080
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if cfg.Instance != "b" {
		t.Errorf("instance = %s, want b", cfg.Instance)
	}
	if cfg.WebPort != 18081 {
		t.Errorf("webPort = %d, want 18081", cfg.WebPort)
	}
	if cfg.WebHost != "127.0.0.1" {
		t.Errorf("webHost = %s, want 127.0.0.1", cfg.WebHost)
	}
	if cfg.LogDir != "logs" {
		t.Errorf("logDir = %s, want logs", cfg.LogDir)
	}
	if cfg.StateFile != "daemon.state" {
		t.Errorf("stateFile = %s, want daemon.state", cfg.StateFile)
	}
	if cfg.Peer.CheckInterval != 5 {
		t.Errorf("peer checkInterval = %d, want 5", cfg.Peer.CheckInterval)
	}

	if len(cfg.Projects) != 1 {
		t.Fatalf("projects count = %d, want 1", len(cfg.Projects))
	}
	p := cfg.Projects[0]
	if p.Name != "TestApp" {
		t.Errorf("project name = %s, want TestApp", p.Name)
	}
	if p.CheckInterval != 15 {
		t.Errorf("checkInterval default = %d, want 15", p.CheckInterval)
	}
	if p.StartupTimeout != 60 {
		t.Errorf("startupTimeout default = %d, want 60", p.StartupTimeout)
	}
	if p.MaxRestartAttempts != 3 {
		t.Errorf("maxRestartAttempts default = %d, want 3", p.MaxRestartAttempts)
	}
	if p.RestartDelay != 10 {
		t.Errorf("restartDelay default = %d, want 10", p.RestartDelay)
	}
	if p.StartMode != "async" {
		t.Errorf("startMode default = %s, want async", p.StartMode)
	}
	if p.Monitor.Type != "port" {
		t.Errorf("monitor type default = %s, want port", p.Monitor.Type)
	}
	if p.Monitor.Host != "127.0.0.1" {
		t.Errorf("monitor host default = %s, want 127.0.0.1", p.Monitor.Host)
	}
}

func TestServiceNames(t *testing.T) {
	cfg := &DaemonConfig{Instance: "a", Peer: PeerConfig{InstanceID: "b"}}
	if cfg.ServiceName() != "ProcessGuard-a" {
		t.Errorf("ServiceName = %s, want ProcessGuard-a", cfg.ServiceName())
	}
	if cfg.PeerServiceName() != "ProcessGuard-b" {
		t.Errorf("PeerServiceName = %s, want ProcessGuard-b", cfg.PeerServiceName())
	}
}

func TestPeerServiceNameOverride(t *testing.T) {
	cfg := &DaemonConfig{Instance: "a", Peer: PeerConfig{InstanceID: "b", ServiceName: "CustomPeer"}}
	if cfg.PeerServiceName() != "CustomPeer" {
		t.Errorf("PeerServiceName = %s, want CustomPeer", cfg.PeerServiceName())
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &DaemonConfig{
		Instance: "a",
		WebPort:  18080,
		Projects: []Project{
			{Name: "App1", Enabled: true},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load saved config failed: %v", err)
	}
	if loaded.Instance != "a" || loaded.WebPort != 18080 {
		t.Errorf("loaded config mismatch: %+v", loaded)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "App1" {
		t.Errorf("loaded projects mismatch: %+v", loaded.Projects)
	}
}

func TestState(t *testing.T) {
	dir := t.TempDir()
	cfg := &DaemonConfig{StateFile: filepath.Join(dir, "state")}

	if cfg.LoadState() != "RUNNING" {
		t.Errorf("initial state = %s, want RUNNING", cfg.LoadState())
	}

	if err := cfg.SaveState("PAUSED"); err != nil {
		t.Fatalf("save state failed: %v", err)
	}
	if cfg.LoadState() != "PAUSED" {
		t.Errorf("state after pause = %s, want PAUSED", cfg.LoadState())
	}

	if err := cfg.SaveState("RUNNING"); err != nil {
		t.Fatalf("save state failed: %v", err)
	}
	if cfg.LoadState() != "RUNNING" {
		t.Errorf("state after resume = %s, want RUNNING", cfg.LoadState())
	}
}
