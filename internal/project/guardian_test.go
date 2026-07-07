package project

import (
	"io"
	"testing"
	"time"

	"daemon-go/internal/actions"
	"daemon-go/internal/config"
	"daemon-go/internal/logger"
)

func newTestGuardian(t *testing.T, cfg config.Project) *ProjectGuardian {
	log := logger.NewWithWriter(io.Discard, "test")
	return NewGuardian(cfg, actions.NewRegistry(), log)
}

func TestGuardianStatusInitial(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true}
	g := newTestGuardian(t, cfg)
	status := g.Status()
	if status["name"] != "App" {
		t.Errorf("name = %s, want App", status["name"])
	}
	if status["state"] != string(StateInit) {
		t.Errorf("state = %s, want INIT", status["state"])
	}
	if status["startTime"] != "-" {
		t.Errorf("startTime = %s, want -", status["startTime"])
	}
}

func TestGuardianSetPaused(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true}
	g := newTestGuardian(t, cfg)
	g.SetPaused(true)
	status := g.Status()
	if status["state"] != string(StatePaused) {
		t.Errorf("state = %s, want PAUSED", status["state"])
	}
	g.SetPaused(false)
	status = g.Status()
	if status["state"] != string(StateInit) {
		t.Errorf("state = %s, want INIT", status["state"])
	}
}

func TestGuardianResetFailCount(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true, MaxRestartAttempts: 3}
	g := newTestGuardian(t, cfg)
	g.failCount = 5
	g.ResetFailCount()
	status := g.Status()
	if status["failCount"] != 0 {
		t.Errorf("failCount = %d, want 0", status["failCount"])
	}
}

func TestGuardianEnsureStartInfo(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true}
	g := newTestGuardian(t, cfg)
	g.ensureStartInfo()
	if g.startTime.IsZero() {
		t.Error("startTime should be set")
	}
	status := g.Status()
	if status["startTime"] == "-" || status["startTime"] == "" {
		t.Error("startTime in status should be formatted")
	}
}

func TestGuardianIsFailedLocked(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true, MaxRestartAttempts: 2}
	g := newTestGuardian(t, cfg)
	if g.isFailedLocked() {
		t.Error("should not be failed initially")
	}
	g.failCount = 2
	if !g.isFailedLocked() {
		t.Error("should be failed when failCount >= max")
	}
}

func TestGuardianCanAutoRestart(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true, MaxRestartAttempts: 1}
	g := newTestGuardian(t, cfg)
	if !g.CanAutoRestart() {
		t.Error("should be able to auto restart initially")
	}
	g.failCount = 1
	if g.CanAutoRestart() {
		t.Error("should not auto restart after reaching max")
	}
}

func TestGuardianIsAlivePort(t *testing.T) {
	cfg := config.Project{
		Name: "App",
		Monitor: config.MonitorConfig{Type: "port", Port: 0, Host: "127.0.0.1"},
	}
	g := newTestGuardian(t, cfg)
	if g.isAlive() {
		t.Error("port 0 should not be alive")
	}
}

func TestGuardianWaitForStartupStop(t *testing.T) {
	cfg := config.Project{Name: "App", Enabled: true, StartupTimeout: 1}
	g := newTestGuardian(t, cfg)
	g.stopCh = make(chan struct{})
	close(g.stopCh)
	if g.waitForStartup() {
		t.Error("waitForStartup should return false when stopped")
	}
}

func TestGuardianStartProjectMaxAttempts(t *testing.T) {
	cfg := config.Project{
		Name:               "App",
		Enabled:            true,
		MaxRestartAttempts: 0,
	}
	g := newTestGuardian(t, cfg)
	err := g.startProject()
	if err == nil {
		t.Error("expected error when max restart attempts reached")
	}
}

func TestGuardianManualRestart(t *testing.T) {
	cfg := config.Project{
		Name:               "App",
		Enabled:            true,
		MaxRestartAttempts: 3,
		StopScript:         "",
		StartScript:        "nonexistent-script-12345.bat",
	}
	g := newTestGuardian(t, cfg)
	g.failCount = 3
	err := g.ManualRestart()
	if err == nil {
		t.Error("expected error because start script does not exist")
	}
	// ManualRestart 重置失败计数，启动失败后 failCount 会计入一次失败
	if g.failCount != 1 {
		t.Errorf("failCount after manual restart = %d, want 1", g.failCount)
	}
}

func TestGuardianSetState(t *testing.T) {
	cfg := config.Project{Name: "App"}
	g := newTestGuardian(t, cfg)
	g.setState(StateRunning)
	if g.state != StateRunning {
		t.Errorf("state = %s, want RUNNING", g.state)
	}
}

func TestGuardianEnsureStartInfoIdempotent(t *testing.T) {
	cfg := config.Project{Name: "App"}
	g := newTestGuardian(t, cfg)
	first := time.Now()
	g.ensureStartInfo()
	if g.startTime.Before(first) {
		t.Error("startTime should not be before ensure call")
	}
	stored := g.startTime
	g.startTime = stored.Add(-time.Hour)
	g.ensureStartInfo()
	if !g.startTime.Equal(stored.Add(-time.Hour)) {
		t.Error("ensureStartInfo should not overwrite existing startTime")
	}
}
