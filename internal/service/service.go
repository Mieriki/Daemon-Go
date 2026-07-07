package service

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// Service 服务接口
type Service interface {
	Start() error
	Stop() error
}

// DaemonService 守护服务
type DaemonService struct {
	Name        string
	DisplayName string
	Description string
	ExePath     string
	Args        []string
	Handler     Service
}

// InstallService 安装 Windows 服务
func InstallService(name, displayName, description, exePath string, args []string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager failed: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err == nil {
		_ = s.Close()
		return fmt.Errorf("service %s already exists", name)
	}

	cfg := mgr.Config{
		DisplayName: displayName,
		Description: description,
		StartType:   mgr.StartAutomatic,
	}

	s, err = m.CreateService(name, exePath, cfg, args...)
	if err != nil {
		return fmt.Errorf("create service failed: %w", err)
	}
	defer s.Close()

	// 确保事件源存在
	_ = eventlog.InstallAsEventCreate(name, eventlog.Error|eventlog.Warning|eventlog.Info)
	return nil
}

// UninstallService 卸载 Windows 服务
func UninstallService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager failed: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service failed: %w", err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service failed: %w", err)
	}
	_ = eventlog.Remove(name)
	return nil
}

// StartService 启动服务
func StartService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service failed: %w", err)
	}
	return nil
}

// StopService 停止服务
func StopService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stop service failed: %w", err)
	}
	_ = status
	return nil
}

// QueryServiceStatus 查询服务状态
func QueryServiceStatus(name string) (svc.State, error) {
	m, err := mgr.Connect()
	if err != nil {
		return svc.Stopped, err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return svc.Stopped, err
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return svc.Stopped, err
	}
	return status.State, nil
}

// Run 作为服务运行。cmd 为 install/uninstall/start/stop 时直接执行命令。
func (s *DaemonService) Run(cmd string) error {
	if cmd != "" {
		switch cmd {
		case "install":
			return InstallService(s.Name, s.DisplayName, s.Description, s.ExePath, s.Args)
		case "uninstall":
			return UninstallService(s.Name)
		case "start":
			return StartService(s.Name)
		case "stop":
			return StopService(s.Name)
		}
	}

	// 如果以服务方式运行，进入服务调度
	interactive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return fmt.Errorf("failed to determine interactive session: %w", err)
	}
	if interactive {
		// 控制台模式直接运行，并保持阻塞
		if err := s.Handler.Start(); err != nil {
			return err
		}
		// 等待中断信号或一直阻塞
		waitForever()
		return s.Handler.Stop()
	}

	return svc.Run(s.Name, &serviceHandler{handler: s.Handler})
}

// serviceHandler 适配 svc.Handler
type serviceHandler struct {
	handler Service
}

func (h *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	sendStatus := func(s svc.Status) {
		select {
		case changes <- s:
		case <-time.After(2 * time.Second):
		}
	}

	sendStatus(svc.Status{State: svc.StartPending})

	go func() {
		_ = h.handler.Start()
	}()

	sendStatus(svc.Status{State: svc.Running, Accepts: cmdsAccepted})

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				sendStatus(c.CurrentStatus)
			case svc.Stop, svc.Shutdown:
				sendStatus(svc.Status{State: svc.StopPending})
				go func() {
					_ = h.handler.Stop()
					sendStatus(svc.Status{State: svc.Stopped})
				}()
				return false, 0
			default:
				sendStatus(c.CurrentStatus)
			}
		}
	}
}

func waitForever() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
}

// ExePath 获取当前可执行文件路径
func ExePath() string {
	exe, _ := os.Executable()
	if exe == "" {
		exe = os.Args[0]
	}
	abs, _ := filepath.Abs(exe)
	return abs
}
