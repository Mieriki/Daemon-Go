package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"daemon-go/internal/auth"
	"daemon-go/internal/config"
	"daemon-go/internal/logger"
	"daemon-go/internal/project"
)

//go:embed all:static
var staticFS embed.FS

// Server Web 控制台服务器
type Server struct {
	cfg     *config.DaemonConfig
	manager *project.Manager
	logger  *logger.Logger
	auth    *auth.Manager
	srv     *http.Server
}

// NewServer 创建 Web 服务器
func NewServer(cfg *config.DaemonConfig, manager *project.Manager, logger *logger.Logger, auth *auth.Manager) *Server {
	return &Server{
		cfg:     cfg,
		manager: manager,
		logger:  logger,
		auth:    auth,
	}
}

// Start 启动 Web 服务
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.withLocalIP(s.handleHealth))
	mux.HandleFunc("/api/auth", s.withLocalIP(s.handleAuth))
	mux.HandleFunc("/api/status", s.withLocalIP(s.withToken(s.handleStatus)))
	mux.HandleFunc("/api/logs", s.withLocalIP(s.withToken(s.handleLogs)))
	mux.HandleFunc("/api/events", s.withLocalIP(s.withToken(s.handleEvents)))
	mux.HandleFunc("/api/pause", s.withLocalIP(s.withToken(s.handlePause)))
	mux.HandleFunc("/api/resume", s.withLocalIP(s.withToken(s.handleResume)))
	mux.HandleFunc("/api/projects", s.withLocalIP(s.withToken(s.handleProjects)))
	mux.HandleFunc("/api/projects/start", s.withLocalIP(s.withToken(s.handleProjectStart)))
	mux.HandleFunc("/api/projects/stop", s.withLocalIP(s.withToken(s.handleProjectStop)))
	mux.HandleFunc("/api/projects/restart", s.withLocalIP(s.withToken(s.handleProjectRestart)))
	mux.HandleFunc("/api/projects/reset", s.withLocalIP(s.withToken(s.handleProjectReset)))
	mux.HandleFunc("/api/config", s.withLocalIP(s.withToken(s.handleConfig)))
	mux.HandleFunc("/api/restart", s.withLocalIP(s.withToken(s.handleRestart)))
	mux.HandleFunc("/", s.withLocalIP(s.handleStatic))

	addr := fmt.Sprintf("%s:%d", s.cfg.WebHost, s.cfg.WebPort)
	s.srv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	s.logger.Infof("web server listening on %s", addr)
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Errorf("web server error: %v", err)
		}
	}()
	return nil
}

// Stop 停止 Web 服务
func (s *Server) Stop() error {
	if s.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	state := s.cfg.LoadState()
	writeJSON(w, map[string]interface{}{
		"instance": s.cfg.Instance,
		"state":    state,
		"projects": s.manager.Status(),
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs := s.logger.RecentLogs()
	writeJSON(w, map[string]interface{}{
		"logs": logs,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	events := logger.LoadAllRecentEvents(s.cfg.LogDir, 10)
	writeJSON(w, map[string]interface{}{
		"events": events,
	})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.cfg.SaveState("PAUSED"); err != nil {
		writeError(w, err)
		return
	}
	s.manager.SetPaused(true)
	s.logger.Info("daemon paused by web")
	s.logger.Events.Add(logger.EventDaemonPause, s.cfg.Instance, "守护进程已暂停", "")
	writeJSON(w, map[string]string{"state": "PAUSED"})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.cfg.SaveState("RUNNING"); err != nil {
		writeError(w, err)
		return
	}
	s.manager.SetPaused(false)
	s.logger.Info("daemon resumed by web")
	s.logger.Events.Add(logger.EventDaemonResume, s.cfg.Instance, "守护进程已恢复", "")
	writeJSON(w, map[string]string{"state": "RUNNING"})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.Status())
}

func (s *Server) handleProjectStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, fmt.Errorf("name is required"))
		return
	}
	if err := s.manager.ManualStart(name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"result": "started", "name": name})
}

func (s *Server) handleProjectStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, fmt.Errorf("name is required"))
		return
	}
	if err := s.manager.ManualStop(name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"result": "stopped", "name": name})
}

func (s *Server) handleProjectRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, fmt.Errorf("name is required"))
		return
	}
	if err := s.manager.ManualRestart(name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"result": "restarted", "name": name})
}

func (s *Server) handleProjectReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		// 重置所有项目
		s.manager.ResetFailCount()
		s.logger.Info("all projects fail count reset by web")
		s.logger.Events.Add(logger.EventManualRestart, "all", "重置所有项目失败计数", "")
		writeJSON(w, map[string]string{"result": "reset all"})
		return
	}
	if err := s.manager.ResetFailCountByName(name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"result": "reset", "name": name})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dto := configToDTO(s.cfg)
		writeJSON(w, dto)
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, fmt.Errorf("read body failed: %w", err))
			return
		}
		defer r.Body.Close()

		var dto ConfigDTO
		if err := json.Unmarshal(body, &dto); err != nil {
			writeError(w, fmt.Errorf("invalid json: %w", err))
			return
		}

		newCfg, err := dtoToConfig(&dto)
		if err != nil {
			writeError(w, fmt.Errorf("invalid config: %w", err))
			return
		}

		// 备份原配置
		if _, err := os.Stat("config.yaml"); err == nil {
			_ = os.Rename("config.yaml", "config.yaml.bak")
		}

		// 写入新配置
		if err := newCfg.Save("config.yaml"); err != nil {
			// 尝试回滚
			_ = os.Rename("config.yaml.bak", "config.yaml")
			writeError(w, fmt.Errorf("save config failed: %w", err))
			return
		}

		s.logger.Info("config updated by web")
		s.logger.Events.Add(logger.EventConfigSaved, s.cfg.Instance, "配置已更新", "")
		writeJSON(w, map[string]string{"result": "saved", "message": "配置已保存，请重启 ProcessGuard-A 服务生效"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	serviceName := s.cfg.ServiceName()
	s.logger.Infof("restarting service %s via web (stop self, let peer recover)", serviceName)

	// 先立即响应浏览器，再由对端 B 检测并拉起 A
	writeJSON(w, map[string]string{
		"result":  "restarting",
		"service": serviceName,
		"message": "A 服务将停止，由对端 B 自动拉起，请等待约 10-20 秒后刷新页面",
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// 延迟 2 秒后停止 A，让 HTTP 响应发完；B 的 peer watcher 会发现 A 停止并执行 sc start
	go func() {
		time.Sleep(2 * time.Second)
		s.logger.Events.Add(logger.EventServiceRestart, serviceName, "通过 Web 重启 A 服务", "")
		_ = exec.Command("sc", "stop", serviceName).Run()
	}()
}

// ConfigDTO Web 配置传输对象
type ConfigDTO struct {
	WebPort           int          `json:"webPort"`
	PeerCheckInterval int          `json:"peerCheckInterval"`
	Projects          []ProjectDTO `json:"projects"`
}

// ProjectDTO Web 项目传输对象
type ProjectDTO struct {
	Name               string      `json:"name"`
	Enabled            bool        `json:"enabled"`
	MonitorType        string      `json:"monitorType"`
	MonitorPort        int         `json:"monitorPort"`
	MonitorProcessName string      `json:"monitorProcessName"`
	CheckInterval      int         `json:"checkInterval"`
	StartupTimeout     int         `json:"startupTimeout"`
	MaxRestartAttempts int         `json:"maxRestartAttempts"`
	RestartDelay       int         `json:"restartDelay"`
	StartMode          string      `json:"startMode"`
	StartScript        string      `json:"startScript"`
	StopScript         string      `json:"stopScript"`
	PreActions         []ActionDTO `json:"preActions"`
	PostActions        []ActionDTO `json:"postActions"`
}

// ActionDTO Web 动作传输对象
type ActionDTO struct {
	Type   string            `json:"type"`
	Params map[string]string `json:"params"`
}

func configToDTO(cfg *config.DaemonConfig) *ConfigDTO {
	dto := &ConfigDTO{
		WebPort:           cfg.WebPort,
		PeerCheckInterval: cfg.Peer.CheckInterval,
		Projects:          make([]ProjectDTO, 0, len(cfg.Projects)),
	}
	for _, p := range cfg.Projects {
		dto.Projects = append(dto.Projects, projectToDTO(p))
	}
	return dto
}

func projectToDTO(p config.Project) ProjectDTO {
	return ProjectDTO{
		Name:               p.Name,
		Enabled:            p.Enabled,
		MonitorType:        p.Monitor.Type,
		MonitorPort:        p.Monitor.Port,
		MonitorProcessName: p.Monitor.ProcessName,
		CheckInterval:      p.CheckInterval,
		StartupTimeout:     p.StartupTimeout,
		MaxRestartAttempts: p.MaxRestartAttempts,
		RestartDelay:       p.RestartDelay,
		StartMode:          p.StartMode,
		StartScript:        p.StartScript,
		StopScript:         p.StopScript,
		PreActions:         actionsToDTO(p.PreActions),
		PostActions:        actionsToDTO(p.PostActions),
	}
}

func actionsToDTO(actions []config.ActionConfig) []ActionDTO {
	result := make([]ActionDTO, 0, len(actions))
	for _, a := range actions {
		params := a.Params
		if params == nil {
			params = map[string]string{}
		}
		result = append(result, ActionDTO{Type: a.Type, Params: params})
	}
	return result
}

func dtoToConfig(dto *ConfigDTO) (*config.DaemonConfig, error) {
	if dto.WebPort <= 0 || dto.WebPort > 65535 {
		return nil, fmt.Errorf("web port must be between 1 and 65535")
	}
	if dto.PeerCheckInterval <= 0 {
		dto.PeerCheckInterval = 10
	}

	cfg := &config.DaemonConfig{
		Instance:  "a",
		LogDir:    "logs",
		StateFile: "daemon.state",
		WebHost:   "127.0.0.1",
		WebPort:   dto.WebPort,
		Peer: config.PeerConfig{
			InstanceID:    "b",
			CheckInterval: dto.PeerCheckInterval,
		},
		Projects: make([]config.Project, 0, len(dto.Projects)),
	}

	for i, p := range dto.Projects {
		project, err := dtoToProject(p)
		if err != nil {
			return nil, fmt.Errorf("project %d: %w", i+1, err)
		}
		cfg.Projects = append(cfg.Projects, project)
	}

	return cfg, nil
}

func dtoToProject(dto ProjectDTO) (config.Project, error) {
	if strings.TrimSpace(dto.Name) == "" {
		return config.Project{}, fmt.Errorf("project name is required")
	}
	if dto.MonitorType != "port" && dto.MonitorType != "process" && dto.MonitorType != "mixed" {
		return config.Project{}, fmt.Errorf("monitor type must be port, process or mixed")
	}
	if dto.MonitorType == "port" || dto.MonitorType == "mixed" {
		if dto.MonitorPort <= 0 || dto.MonitorPort > 65535 {
			return config.Project{}, fmt.Errorf("monitor port must be between 1 and 65535")
		}
	}
	if dto.CheckInterval <= 0 {
		dto.CheckInterval = 15
	}
	if dto.StartupTimeout <= 0 {
		dto.StartupTimeout = 60
	}
	if dto.MaxRestartAttempts <= 0 {
		dto.MaxRestartAttempts = 3
	}
	if dto.RestartDelay <= 0 {
		dto.RestartDelay = 10
	}

	return config.Project{
		Name:               strings.TrimSpace(dto.Name),
		Enabled:            dto.Enabled,
		Monitor:            config.MonitorConfig{Type: dto.MonitorType, Port: dto.MonitorPort, ProcessName: dto.MonitorProcessName, Host: "127.0.0.1"},
		CheckInterval:      dto.CheckInterval,
		StartupTimeout:     dto.StartupTimeout,
		MaxRestartAttempts: dto.MaxRestartAttempts,
		RestartDelay:       dto.RestartDelay,
		StartMode:          dto.StartMode,
		StartScript:        dto.StartScript,
		StopScript:         dto.StopScript,
		PreActions:         dtoToActions(dto.PreActions),
		PostActions:        dtoToActions(dto.PostActions),
	}, nil
}

func dtoToActions(actions []ActionDTO) []config.ActionConfig {
	result := make([]config.ActionConfig, 0, len(actions))
	for _, a := range actions {
		if a.Type == "" {
			continue
		}
		params := a.Params
		if params == nil {
			params = map[string]string{}
		}
		result = append(result, config.ActionConfig{Type: a.Type, Params: params})
	}
	return result
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		r.URL.Path = "/index.html"
	}
	// 使用 path.Join + Clean 防止路径遍历
	clean := path.Clean(path.Join("static", r.URL.Path))
	if clean == "." || clean == "static" || !strings.HasPrefix(clean, "static/") {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile(clean)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "text/plain"
	switch {
	case strings.HasSuffix(clean, ".html"):
		contentType = "text/html"
	case strings.HasSuffix(clean, ".css"):
		contentType = "text/css"
	case strings.HasSuffix(clean, ".js"):
		contentType = "application/javascript"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	writeJSON(w, map[string]string{"error": err.Error()})
}

func (s *Server) withLocalIP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocalIP(r) {
			s.logger.Warnf("reject remote access from %s", r.RemoteAddr)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) withToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if !s.auth.Validate(token) {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]string{"error": "invalid or missing token"})
			return
		}
		next(w, r)
	}
}

func extractToken(r *http.Request) string {
	if token := r.Header.Get("X-Guard-Token"); token != "" {
		return token
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	if c, err := r.Cookie("guard_token"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

func isLocalIP(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if s.auth.Validate(token) {
		writeJSON(w, map[string]string{"valid": "true"})
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	writeJSON(w, map[string]string{"error": "invalid token"})
}

// StaticFiles 返回静态文件系统（用于测试）
func StaticFiles() fs.FS {
	fsys, _ := fs.Sub(staticFS, "static")
	return fsys
}