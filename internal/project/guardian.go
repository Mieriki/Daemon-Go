package project

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"daemon-go/internal/actions"
	"daemon-go/internal/config"
	"daemon-go/internal/logger"
)

// State 项目状态
type State string

const (
	StateInit     State = "INIT"
	StateRunning  State = "RUNNING"
	StateStopped  State = "STOPPED"
	StateStarting State = "STARTING"
	StateFailed   State = "FAILED"
	StatePaused   State = "PAUSED"
)

// ProjectGuardian 项目守护者
type ProjectGuardian struct {
	cfg       config.Project
	registry  *actions.Registry
	logger    *logger.Logger
	state     State
	stateMu   sync.RWMutex
	stopCh    chan struct{}
	stoppedCh chan struct{}
	paused    bool
	pid       int
	startTime time.Time
	failCount int
	lastAlive bool
}

// NewGuardian 创建守护者
func NewGuardian(cfg config.Project, registry *actions.Registry, logger *logger.Logger) *ProjectGuardian {
	return &ProjectGuardian{
		cfg:       cfg,
		registry:  registry,
		logger:    logger,
		state:     StateInit,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Start 启动守护循环
func (g *ProjectGuardian) Start() {
	g.logger.Infof("start guardian for project %s", g.cfg.Name)
	go g.loop()
}

// Stop 停止守护循环
func (g *ProjectGuardian) Stop() {
	close(g.stopCh)
	<-g.stoppedCh
}

// SetPaused 设置暂停状态
func (g *ProjectGuardian) SetPaused(paused bool) {
	g.stateMu.Lock()
	g.paused = paused
	if paused {
		g.state = StatePaused
	} else {
		g.state = StateInit
	}
	g.stateMu.Unlock()
}

// Status 获取当前状态
func (g *ProjectGuardian) Status() map[string]interface{} {
	g.stateMu.RLock()
	defer g.stateMu.RUnlock()
	startTimeStr := "-"
	if !g.startTime.IsZero() {
		startTimeStr = g.startTime.Format("2006-01-02 15:04:05")
	}
	return map[string]interface{}{
		"name":       g.cfg.Name,
		"state":      string(g.state),
		"enabled":    g.cfg.Enabled,
		"pid":        g.pid,
		"startTime":  startTimeStr,
		"failCount":  g.failCount,
	}
}

// ManualStart 手动启动项目（会重置失败计数）
func (g *ProjectGuardian) ManualStart() error {
	g.ResetFailCount()
	g.logger.Events.Add(logger.EventManualStart, g.cfg.Name, fmt.Sprintf("手动启动项目 %s", g.cfg.Name), "")
	return g.startProject()
}

// ManualStop 手动停止项目
func (g *ProjectGuardian) ManualStop() error {
	g.logger.Events.Add(logger.EventManualStop, g.cfg.Name, fmt.Sprintf("手动停止项目 %s", g.cfg.Name), "")
	return g.stopProject()
}

// ManualRestart 手动重启项目（会重置失败计数）
func (g *ProjectGuardian) ManualRestart() error {
	g.ResetFailCount()
	g.logger.Events.Add(logger.EventManualRestart, g.cfg.Name, fmt.Sprintf("手动重启项目 %s", g.cfg.Name), "")
	_ = g.stopProject()
	time.Sleep(2 * time.Second)
	g.ResetFailCount()
	return g.startProject()
}
func (g *ProjectGuardian) loop() {
	defer close(g.stoppedCh)
	ticker := time.NewTicker(time.Duration(g.cfg.CheckInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			g.logger.Infof("guardian for %s stopped", g.cfg.Name)
			return
		case <-ticker.C:
			if g.isPaused() {
				continue
			}
			if !g.cfg.Enabled {
				continue
			}
			if g.isFailedLocked() {
				// 已连续失败达到上限，等待人工干预
				continue
			}
			if !g.isAlive() {
				g.logger.Warnf("project %s is not alive, try to start", g.cfg.Name)
				g.logger.Events.Add(logger.EventProjectDown, g.cfg.Name, fmt.Sprintf("项目 %s 掉线，准备重启", g.cfg.Name), "")
				g.startProject()
			} else {
				g.setState(StateRunning)
				g.ensureStartInfo()
			}
		}
	}
}

func (g *ProjectGuardian) isFailedLocked() bool {
	g.stateMu.RLock()
	defer g.stateMu.RUnlock()
	return g.failCount >= g.cfg.MaxRestartAttempts
}

// CanAutoRestart 检查是否还能自动重启
func (g *ProjectGuardian) CanAutoRestart() bool {
	return !g.isFailedLocked()
}

// ResetFailCount 重置失败计数
func (g *ProjectGuardian) ResetFailCount() {
	g.stateMu.Lock()
	g.failCount = 0
	g.stateMu.Unlock()
}

// MarkFailed 手动标记为失败
func (g *ProjectGuardian) MarkFailed() {
	g.setState(StateFailed)
}

func (g *ProjectGuardian) isPaused() bool {
	g.stateMu.RLock()
	defer g.stateMu.RUnlock()
	return g.paused
}

func (g *ProjectGuardian) ensureStartInfo() {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()
	if g.startTime.IsZero() {
		g.startTime = time.Now()
	}
	if g.pid == 0 {
		g.pid = g.findPid()
	}
}

func (g *ProjectGuardian) setState(s State) {
	g.stateMu.Lock()
	g.state = s
	g.stateMu.Unlock()
}

func (g *ProjectGuardian) isAlive() bool {
	alive := false
	if g.cfg.Monitor.Type == "port" || g.cfg.Monitor.Type == "mixed" {
		if g.cfg.Monitor.Port > 0 {
			if actions.CheckPort(g.cfg.Monitor.Host, g.cfg.Monitor.Port) {
				alive = true
			}
		}
	}
	if !alive && (g.cfg.Monitor.Type == "process" || g.cfg.Monitor.Type == "mixed") {
		if g.cfg.Monitor.ProcessName != "" {
			if actions.CheckProcess(g.cfg.Monitor.ProcessName) {
				alive = true
			}
		}
	}
	g.lastAlive = alive
	return alive
}

func (g *ProjectGuardian) startProject() error {
	if !g.CanAutoRestart() {
		g.logger.Warnf("project %s has reached max restart attempts (%d), skip auto start", g.cfg.Name, g.cfg.MaxRestartAttempts)
		g.setState(StateFailed)
		return fmt.Errorf("max restart attempts reached")
	}

	g.setState(StateStarting)
	g.logger.Events.Add(logger.EventProjectStart, g.cfg.Name, fmt.Sprintf("项目 %s 开始启动", g.cfg.Name), fmt.Sprintf("脚本: %s", g.cfg.StartScript))
	ctx := &actions.Context{
		Project: g.cfg.Name,
		Logger:  g.logger.WithField("project", g.cfg.Name),
	}

	// 前置动作
	for _, action := range g.cfg.PreActions {
		if err := g.registry.Execute(ctx, action); err != nil {
			g.logger.Warnf("preAction failed for %s: %v", g.cfg.Name, err)
		}
	}

	// 启动脚本
	if g.cfg.StartScript == "" {
		g.setState(StateFailed)
		g.failCount++
		g.logFailCount()
		return fmt.Errorf("project %s start script is empty", g.cfg.Name)
	}
	scriptDir := filepath.Dir(g.cfg.StartScript)
	if err := actions.RunStartScript(ctx, g.cfg.StartScript, scriptDir, g.cfg.StartMode == "sync"); err != nil {
		g.setState(StateFailed)
		g.failCount++
		g.logFailCount()
		return err
	}

	// 等待启动成功
	if g.waitForStartup() {
		g.setState(StateRunning)
		g.ResetFailCount()
		g.startTime = time.Now()
		g.pid = g.findPid()
		g.logger.Events.Add(logger.EventProjectStarted, g.cfg.Name, fmt.Sprintf("项目 %s 启动成功", g.cfg.Name), fmt.Sprintf("PID: %d", g.pid))

		// 后置动作
		for _, action := range g.cfg.PostActions {
			if err := g.registry.Execute(ctx, action); err != nil {
				g.logger.Warnf("postAction failed for %s: %v", g.cfg.Name, err)
			}
		}
	} else {
		g.setState(StateFailed)
		g.failCount++
		g.logFailCount()
		return fmt.Errorf("project %s startup timeout", g.cfg.Name)
	}
	return nil
}

func (g *ProjectGuardian) logFailCount() {
	if g.failCount >= g.cfg.MaxRestartAttempts {
		msg := fmt.Sprintf("项目 %s 连续失败 %d 次，停止自动重启，请人工检查", g.cfg.Name, g.cfg.MaxRestartAttempts)
		g.logger.Errorf("project %s reached max restart attempts (%d), stop auto restart. please check manually.", g.cfg.Name, g.cfg.MaxRestartAttempts)
		g.logger.Events.Add(logger.EventProjectFailed, g.cfg.Name, msg, "")
	} else {
		g.logger.Warnf("project %s startup failed, current fail count: %d/%d", g.cfg.Name, g.failCount, g.cfg.MaxRestartAttempts)
	}
}

func (g *ProjectGuardian) stopProject() error {
	g.logger.Events.Add(logger.EventProjectStop, g.cfg.Name, fmt.Sprintf("手动停止项目 %s", g.cfg.Name), "")
	ctx := &actions.Context{
		Project: g.cfg.Name,
		Logger:  g.logger.WithField("project", g.cfg.Name),
	}
	err := actions.StopProject(ctx, g.cfg.StopScript, "")
	g.stateMu.Lock()
	g.state = StateStopped
	g.pid = 0
	g.startTime = time.Time{}
	g.stateMu.Unlock()
	if err != nil {
		g.logger.Warnf("stop script for %s failed: %v", g.cfg.Name, err)
	}
	return err
}

func (g *ProjectGuardian) waitForStartup() bool {
	deadline := time.Now().Add(time.Duration(g.cfg.StartupTimeout) * time.Second)
	for time.Now().Before(deadline) {
		if g.isAlive() {
			return true
		}
		if g.isStopped() {
			return false
		}
		select {
		case <-g.stopCh:
			return false
		case <-time.After(2 * time.Second):
		}
	}
	return g.isAlive()
}

func (g *ProjectGuardian) isStopped() bool {
	select {
	case <-g.stopCh:
		return true
	default:
		return false
	}
}

func (g *ProjectGuardian) findPid() int {
	// 简单通过端口查找 PID
	if g.cfg.Monitor.Type == "port" && g.cfg.Monitor.Port > 0 {
		pid := findPidByPort(g.cfg.Monitor.Port)
		if pid > 0 {
			return pid
		}
	}
	// 通过进程名查找 PID
	if g.cfg.Monitor.ProcessName != "" {
		pid := findPidByName(g.cfg.Monitor.ProcessName)
		if pid > 0 {
			return pid
		}
	}
	return 0
}
