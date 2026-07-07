package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"daemon-go/internal/auth"
	"daemon-go/internal/config"
	"daemon-go/internal/logger"
	"daemon-go/internal/project"
	"daemon-go/internal/service"
	"daemon-go/internal/watcher"
	"daemon-go/internal/web"
)

var instance string
var instanceMutex windows.Handle

func init() {
	flag.StringVar(&instance, "instance", "a", "instance id: a or b")
}

type daemonHandler struct {
	manager     *project.Manager
	server      *web.Server
	peerWatcher *watcher.PeerWatcher
	cfg         *config.DaemonConfig
}

func (h *daemonHandler) Start() error {
	state := h.cfg.LoadState()
	if h.cfg.Instance == "a" {
		if state == "PAUSED" {
			h.manager.SetPaused(true)
		} else {
			h.manager.Start()
		}
	}
	h.peerWatcher.Start()
	return nil
}

func (h *daemonHandler) Stop() error {
	go func() {
		if h.server != nil {
			_ = h.server.Stop()
		}
		if h.manager != nil {
			h.manager.Stop()
		}
		if h.peerWatcher != nil {
			h.peerWatcher.Stop()
		}
	}()
	return nil
}

func main() {
	flag.Parse()

	if instance != "a" && instance != "b" {
		fmt.Fprintf(os.Stderr, "invalid instance: %s, must be a or b\n", instance)
		os.Exit(1)
	}

	// 切换到可执行文件所在目录，确保服务模式下能读取 config.yaml
	if err := changeToExeDir(); err != nil {
		fmt.Fprintf(os.Stderr, "change to exe dir failed: %v\n", err)
		os.Exit(1)
	}

	// 提取 install/uninstall/start/stop 命令（可在 --instance 之后任意位置）
	var cmd string
	for _, arg := range flag.Args() {
		switch arg {
		case "install", "uninstall", "start", "stop":
			cmd = arg
		}
	}

	// 安装/卸载等命令不需要启动完整服务，也不需要加载 config
	if cmd == "install" || cmd == "uninstall" || cmd == "start" || cmd == "stop" {
		cfg := &config.DaemonConfig{Instance: instance}
		svc := &service.DaemonService{
			Name:        cfg.ServiceName(),
			DisplayName: fmt.Sprintf("ProcessGuard Service %s", instance),
			Description: fmt.Sprintf("Process guard instance %s", instance),
			ExePath:     service.ExePath(),
			Args:        []string{"--instance", instance},
		}
		if err := svc.Run(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "%s service failed: %v\n", cmd, err)
			os.Exit(1)
		}
		return
	}

	mutexName := fmt.Sprintf("Global\\ProcessGuard-%s", instance)
	if alreadyRunning(mutexName) {
		fmt.Fprintf(os.Stderr, "daemon instance %s is already running\n", instance)
		os.Exit(1)
	}
	defer func() {
		if instanceMutex != 0 {
			_ = windows.CloseHandle(instanceMutex)
		}
	}()

	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}
	cfg.Instance = instance
	// 自动设置对端实例：a 的对端是 b，b 的对端是 a
	if instance == "a" {
		cfg.Peer.InstanceID = "b"
	} else {
		cfg.Peer.InstanceID = "a"
	}

	// 加载或生成访问 Token
	authManager, err := auth.NewManager(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "init auth failed: %v\n", err)
		os.Exit(1)
	}
	cfg.AuthToken = authManager.Token()

	log := logger.New(cfg.LogDir, instance)
	log.Infof("daemon instance %s starting", instance)

	manager := project.NewManager(cfg, log)

	var server *web.Server
	if instance == "a" {
		server = web.NewServer(cfg, manager, log, authManager)
		if err := server.Start(); err != nil {
			log.Errorf("start web server failed: %v", err)
		}
	}

	peerWatcher := watcher.NewPeerWatcher(cfg, log)

	svc := &service.DaemonService{
		Name:        cfg.ServiceName(),
		DisplayName: fmt.Sprintf("ProcessGuard Service %s", instance),
		Description: fmt.Sprintf("Process guard instance %s", instance),
		ExePath:     service.ExePath(),
		Args:        []string{"--instance", instance},
		Handler: &daemonHandler{
			manager:     manager,
			server:      server,
			peerWatcher: peerWatcher,
			cfg:         cfg,
		},
	}

	if err := svc.Run(""); err != nil {
		log.Errorf("service run failed: %v", err)
		os.Exit(1)
	}
}

func alreadyRunning(name string) bool {
	handle, err := windows.CreateMutex(nil, true, windows.StringToUTF16Ptr(name))
	if err == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(handle)
		return true
	}
	if err != nil {
		return false
	}
	instanceMutex = handle
	return false
}

func changeToExeDir() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	return os.Chdir(dir)
}
