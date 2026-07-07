package project

import (
	"fmt"
	"sync"
	"time"

	"daemon-go/internal/actions"
	"daemon-go/internal/config"
	"daemon-go/internal/logger"
)

// Manager 管理所有项目守护者
type Manager struct {
	cfg       *config.DaemonConfig
	registry  *actions.Registry
	logger    *logger.Logger
	guardians map[string]*ProjectGuardian
	mu        sync.RWMutex
	paused    bool
}

// NewManager 创建项目守护管理器
func NewManager(cfg *config.DaemonConfig, logger *logger.Logger) *Manager {
	r := actions.NewRegistry()
	r.AllowedCommands = cfg.AllowedCommands
	return &Manager{
		cfg:       cfg,
		registry:  r,
		logger:    logger,
		guardians: make(map[string]*ProjectGuardian),
	}
}

// Logger 获取日志器
func (m *Manager) Logger() *logger.Logger {
	return m.logger
}

// Start 启动所有项目守护
func (m *Manager) Start() {
	m.logger.Info("project manager starting")
	for _, p := range m.cfg.Projects {
		if !p.Enabled {
			m.logger.Infof("project %s disabled, skip", p.Name)
			continue
		}
		g := NewGuardian(p, m.registry, m.logger)
		m.mu.Lock()
		m.guardians[p.Name] = g
		m.mu.Unlock()
		g.Start()
	}
}

// Stop 停止所有项目守护，最多等待 10 秒
func (m *Manager) Stop() {
	m.logger.Info("project manager stopping")
	m.mu.Lock()
	guardians := make([]*ProjectGuardian, 0, len(m.guardians))
	for _, g := range m.guardians {
		guardians = append(guardians, g)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, g := range guardians {
		wg.Add(1)
		go func(g *ProjectGuardian) {
			defer wg.Done()
			g.Stop()
		}(g)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("project manager stopped")
	case <-time.After(10 * time.Second):
		m.logger.Warn("project manager stop timeout")
	}
}

// SetPaused 设置全局暂停状态
func (m *Manager) SetPaused(paused bool) {
	m.mu.Lock()
	m.paused = paused
	m.mu.Unlock()
	for _, g := range m.guardians {
		g.SetPaused(paused)
	}
	m.logger.Infof("all projects paused=%v", paused)
}

// IsPaused 获取暂停状态
func (m *Manager) IsPaused() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.paused
}

// Status 获取所有项目状态
func (m *Manager) Status() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []map[string]interface{}
	for _, g := range m.guardians {
		result = append(result, g.Status())
	}
	return result
}

// ManualStart 手动启动项目
func (m *Manager) ManualStart(name string) error {
	m.mu.RLock()
	g, ok := m.guardians[name]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return g.ManualStart()
}

// ManualStop 手动停止项目
func (m *Manager) ManualStop(name string) error {
	m.mu.RLock()
	g, ok := m.guardians[name]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return g.ManualStop()
}

// ManualRestart 手动重启项目（会重置失败计数）
func (m *Manager) ManualRestart(name string) error {
	m.mu.RLock()
	g, ok := m.guardians[name]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return g.ManualRestart()
}

// ResetFailCount 重置所有项目失败计数
func (m *Manager) ResetFailCount() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, g := range m.guardians {
		g.ResetFailCount()
	}
}

// ResetFailCountByName 重置指定项目失败计数
func (m *Manager) ResetFailCountByName(name string) error {
	m.mu.RLock()
	g, ok := m.guardians[name]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("project %s not found", name)
	}
	g.ResetFailCount()
	g.logger.Events.Add(logger.EventManualRestart, name, fmt.Sprintf("重置项目 %s 失败计数", name), "")
	return nil
}
