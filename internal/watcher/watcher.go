package watcher

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"daemon-go/internal/config"
	"daemon-go/internal/logger"
)

// PeerWatcher 对端守护者
type PeerWatcher struct {
	cfg       *config.DaemonConfig
	logger    *logger.Logger
	stopCh    chan struct{}
	stoppedCh chan struct{}
	lastAlive bool
}

// NewPeerWatcher 创建对端守护者
func NewPeerWatcher(cfg *config.DaemonConfig, logger *logger.Logger) *PeerWatcher {
	return &PeerWatcher{
		cfg:       cfg,
		logger:    logger,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Start 启动监控
func (w *PeerWatcher) Start() {
	if w.cfg.Peer.InstanceID == "" {
		w.logger.Info("no peer configured, skip peer watcher")
		return
	}
	w.logger.Infof("start peer watcher for %s", w.cfg.Peer.InstanceID)
	go w.loop()
}

// Stop 停止监控
func (w *PeerWatcher) Stop() {
	close(w.stopCh)
	select {
	case <-w.stoppedCh:
	case <-time.After(2 * time.Second):
	}
}

func (w *PeerWatcher) loop() {
	defer close(w.stoppedCh)
	interval := time.Duration(w.cfg.Peer.CheckInterval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 首次立即检查
	w.checkAndRecover()

	for {
		select {
		case <-w.stopCh:
			w.logger.Info("peer watcher stopped")
			return
		case <-ticker.C:
			w.checkAndRecover()
		}
	}
}

func (w *PeerWatcher) checkAndRecover() {
	self := w.cfg.Instance
	peer := w.cfg.Peer.InstanceID
	alive := w.isPeerAlive()
	if alive {
		if !w.lastAlive {
			w.lastAlive = true
			w.logger.Debugf("peer %s is alive", peer)
			w.logger.Events.Add(logger.EventPeerRecover, self, fmt.Sprintf("本端 %s 检测到对端 %s 已恢复", self, peer), peer)
		}
		return
	}
	if w.lastAlive {
		w.lastAlive = false
		w.logger.Warnf("peer %s is not alive", peer)
		w.logger.Events.Add(logger.EventPeerDown, self, fmt.Sprintf("本端 %s 检测到对端 %s 掉线", self, peer), peer)
	}
	w.recoverPeer()
}

func (w *PeerWatcher) isPeerAlive() bool {
	// 1. 检查服务状态
	status := w.queryServiceStatus()
	if status == "Running" {
		// 2. 进一步检查健康接口
		if w.cfg.Peer.InstanceID == "a" {
			url := fmt.Sprintf("http://%s:%d/api/health", w.cfg.WebHost, w.cfg.WebPort)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return false
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				w.logger.Warnf("peer health check failed: %v", err)
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		}
		return true
	}
	return false
}

func (w *PeerWatcher) queryServiceStatus() string {
	cmd := exec.Command("sc", "query", w.cfg.PeerServiceName())
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 查询失败通常是服务尚未启动或权限问题，不需要频繁打印
		return "Unknown"
	}
	output := strings.ToUpper(string(out))
	if strings.Contains(output, "RUNNING") {
		return "Running"
	}
	if strings.Contains(output, "STOPPED") {
		return "Stopped"
	}
	if strings.Contains(output, "START_PENDING") {
		return "StartPending"
	}
	return "Unknown"
}

func (w *PeerWatcher) recoverPeer() {
	self := w.cfg.Instance
	peer := w.cfg.Peer.InstanceID
	cmd := exec.Command("sc", "start", w.cfg.PeerServiceName())
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.logger.Errorf("start peer service failed: %v, output: %s", err, string(out))
		w.logger.Events.Add(logger.EventPeerDown, self, fmt.Sprintf("本端 %s 检测到对端 %s 恢复失败", self, peer), string(out))
		if w.cfg.Peer.StartScript != "" {
			w.logger.Infof("try to run peer start script: %s", w.cfg.Peer.StartScript)
			scriptCmd := exec.Command("cmd", "/c", w.cfg.Peer.StartScript)
			scriptOut, scriptErr := scriptCmd.CombinedOutput()
			if scriptErr != nil {
				w.logger.Errorf("peer start script failed: %v, output: %s", scriptErr, string(scriptOut))
			}
		}
	} else {
		w.logger.Infof("peer service %s started", w.cfg.PeerServiceName())
	}
}
