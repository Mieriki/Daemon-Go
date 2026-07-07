package watcher

import (
	"io"
	"net/http"
	"testing"
	"time"

	"daemon-go/internal/config"
	"daemon-go/internal/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	return logger.NewWithWriter(io.Discard, "test")
}

func TestPeerWatcherStartNoPeer(t *testing.T) {
	cfg := &config.DaemonConfig{}
	log := newTestLogger(t)
	w := NewPeerWatcher(cfg, log)
	// 不应 panic
	w.Start()
	w.Stop()
}

func TestIsPeerAliveRunning(t *testing.T) {
	cfg := &config.DaemonConfig{Instance: "b", Peer: config.PeerConfig{InstanceID: "a", CheckInterval: 1}, WebHost: "127.0.0.1", WebPort: 0}
	log := newTestLogger(t)
	w := NewPeerWatcher(cfg, log)

	// 没有服务在运行，应该返回 false
	if w.isPeerAlive() {
		t.Error("isPeerAlive should return false when no service running")
	}
}

func TestCheckAndRecoverStateChange(t *testing.T) {
	cfg := &config.DaemonConfig{
		Instance: "b",
		Peer:     config.PeerConfig{InstanceID: "a", CheckInterval: 1},
		WebHost:  "127.0.0.1",
		WebPort:  0,
	}
	log := newTestLogger(t)
	w := NewPeerWatcher(cfg, log)
	w.lastAlive = true
	w.checkAndRecover()
	if w.lastAlive {
		t.Error("lastAlive should be false after peer down")
	}
}

func TestQueryServiceStatus(t *testing.T) {
	cfg := &config.DaemonConfig{
		Instance: "a",
		Peer:     config.PeerConfig{InstanceID: "b", CheckInterval: 1},
	}
	log := newTestLogger(t)
	w := NewPeerWatcher(cfg, log)
	status := w.queryServiceStatus()
	// 服务可能运行也可能未安装，只要返回预定义值之一即可
	valid := status == "Unknown" || status == "Stopped" || status == "Running" || status == "StartPending"
	if !valid {
		t.Errorf("unexpected status: %s", status)
	}
}

func TestPeerWatcherLoopStop(t *testing.T) {
	cfg := &config.DaemonConfig{
		Instance: "a",
		Peer:     config.PeerConfig{InstanceID: "b", CheckInterval: 1},
	}
	log := newTestLogger(t)
	w := NewPeerWatcher(cfg, log)
	w.Start()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	select {
	case <- w.stoppedCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Error("peer watcher did not stop in time")
	}
}

func TestIsPeerAliveHealthCheck(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Addr: "127.0.0.1:54321", Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	defer srv.Close()

	// 等待服务启动
	time.Sleep(200 * time.Millisecond)

	cfg := &config.DaemonConfig{
		Instance: "b",
		Peer:     config.PeerConfig{InstanceID: "a", CheckInterval: 1},
		WebHost:  "127.0.0.1",
		WebPort:  54321,
	}
	log := newTestLogger(t)
	w := NewPeerWatcher(cfg, log)
	if !w.isPeerAlive() {
		t.Error("isPeerAlive should return true for healthy endpoint")
	}
}
