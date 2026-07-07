package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"daemon-go/internal/config"
)

func newTestContext() *Context {
	log := logrus.New()
	return &Context{Project: "Test", Logger: log.WithField("project", "Test")}
}

func TestRegistryUnknownAction(t *testing.T) {
	r := NewRegistry()
	ctx := newTestContext()
	err := r.Execute(ctx, config.ActionConfig{Type: "notExist", Params: map[string]string{}})
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestWaitAction(t *testing.T) {
	a := &WaitAction{}
	ctx := newTestContext()
	err := a.Execute(ctx, map[string]string{"millis": "10"})
	if err != nil {
		t.Errorf("wait action failed: %v", err)
	}
}

func TestWaitActionInvalid(t *testing.T) {
	a := &WaitAction{}
	ctx := newTestContext()
	err := a.Execute(ctx, map[string]string{"millis": "abc"})
	if err == nil {
		t.Error("expected error for invalid millis")
	}
}

func TestKillProcessActionMissingParam(t *testing.T) {
	a := &KillProcessAction{}
	ctx := newTestContext()
	err := a.Execute(ctx, map[string]string{})
	if err == nil {
		t.Error("expected error when processName missing")
	}
}

func TestResolveWorkDir(t *testing.T) {
	if resolveWorkDir("") != "" {
		t.Error("empty path should return empty workdir")
	}
	if resolveWorkDir("Start.bat") != "." {
		t.Errorf("relative path workdir = %s, want .", resolveWorkDir("Start.bat"))
	}
	abs := filepath.Join("C:", "App", "Start.bat")
	expected := filepath.Join("C:", "App")
	if resolveWorkDir(abs) != expected {
		t.Errorf("absolute path workdir = %s, want %s", resolveWorkDir(abs), expected)
	}
}

func TestCheckPortNotListening(t *testing.T) {
	// 使用一个大概率未监听的端口
	if CheckPort("127.0.0.1", 0) {
		t.Error("port 0 should not be listening")
	}
}

func TestRunCommandActionMissing(t *testing.T) {
	a := &RunCommandAction{}
	ctx := newTestContext()
	err := a.Execute(ctx, map[string]string{})
	if err == nil {
		t.Error("expected error when command missing")
	}
}

func TestRunStartScriptSyncWorkDir(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.bat")
	content := "@echo off\necho hello\n"
	if err := os.WriteFile(script, []byte(content), 0644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	ctx := newTestContext()
	err := RunStartScript(ctx, script, "", true)
	if err != nil {
		t.Errorf("run script failed: %v", err)
	}
}

func TestStopProjectEmpty(t *testing.T) {
	ctx := newTestContext()
	err := StopProject(ctx, "", "")
	if err != nil {
		t.Errorf("empty stop script should return nil, got %v", err)
	}
}
