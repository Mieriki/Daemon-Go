package actions

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"daemon-go/internal/config"
)

// Context 动作执行上下文
type Context struct {
	Project         string
	Logger          *logrus.Entry
	AllowedCommands []config.AllowedCommand
}

// Action 动作接口
type Action interface {
	Type() string
	Execute(ctx *Context, params map[string]string) error
}

// Registry 动作注册表
type Registry struct {
	actions         map[string]Action
	AllowedCommands []config.AllowedCommand
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	r := &Registry{actions: make(map[string]Action)}
	r.Register(&KillProcessAction{})
	r.Register(&KillPortAction{})
	r.Register(&KillWindowAction{})
	r.Register(&WaitAction{})
	r.Register(&RunCommandAction{})
	return r
}

// Register 注册动作
func (r *Registry) Register(a Action) {
	r.actions[a.Type()] = a
}

// Execute 按配置执行动作
func (r *Registry) Execute(ctx *Context, cfg config.ActionConfig) error {
	a, ok := r.actions[cfg.Type]
	if !ok {
		return fmt.Errorf("unknown action type: %s", cfg.Type)
	}
	ctx.AllowedCommands = r.AllowedCommands
	ctx.Logger.Infof("execute action %s with params %v", cfg.Type, cfg.Params)
	if err := a.Execute(ctx, cfg.Params); err != nil {
		ctx.Logger.Errorf("action %s failed: %v", cfg.Type, err)
		return err
	}
	ctx.Logger.Infof("action %s done", cfg.Type)
	return nil
}

// KillProcessAction 按进程名结束进程
type KillProcessAction struct{}

func (a *KillProcessAction) Type() string { return "killProcess" }
func (a *KillProcessAction) Execute(ctx *Context, params map[string]string) error {
	name := params["processName"]
	if name == "" {
		return fmt.Errorf("processName is required")
	}
	cmd := exec.Command("taskkill", "/F", "/IM", name)
	out, err := cmd.CombinedOutput()
	ctx.Logger.Debugf("killProcess output: %s", string(out))
	if err != nil {
		// 进程不存在时 taskkill 会返回错误，不一定是真失败
		if strings.Contains(string(out), "没有找到进程") || strings.Contains(string(out), "not found") || strings.Contains(string(out), "没有找到") {
			ctx.Logger.Infof("process %s not found, nothing to kill", name)
			return nil
		}
		return fmt.Errorf("kill process %s failed: %w", name, err)
	}
	return nil
}

// KillPortAction 按端口结束占用进程
type KillPortAction struct{}

func (a *KillPortAction) Type() string { return "killPort" }
func (a *KillPortAction) Execute(ctx *Context, params map[string]string) error {
	port := params["port"]
	if port == "" {
		return fmt.Errorf("port is required")
	}
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%s", port))
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) == 0 {
		ctx.Logger.Infof("no process using port %s", port)
		return nil
	}
	lines := strings.Split(string(out), "\n")
	killed := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		// 只结束处于 LISTENING 状态的进程，避免误杀 TIME_WAIT 连接
		if !strings.Contains(fields[3], "LISTENING") {
			continue
		}
		pid := strings.TrimSpace(fields[len(fields)-1])
		if pid == "" || pid == "0" {
			continue
		}
		killCmd := exec.Command("taskkill", "/F", "/PID", pid)
		killOut, killErr := killCmd.CombinedOutput()
		ctx.Logger.Debugf("kill port %s pid %s output: %s", port, pid, string(killOut))
		if killErr != nil {
			ctx.Logger.Warnf("kill port %s pid %s failed: %v", port, pid, killErr)
		} else {
			killed = true
			ctx.Logger.Infof("killed process %s listening on port %s", pid, port)
		}
	}
	if !killed {
		ctx.Logger.Infof("no LISTENING process found on port %s", port)
	}
	return nil
}

// KillWindowAction 按窗口标题关闭窗口
type KillWindowAction struct{}

func (a *KillWindowAction) Type() string { return "killWindow" }
func (a *KillWindowAction) Execute(ctx *Context, params map[string]string) error {
	title := params["windowTitle"]
	if title == "" {
		return fmt.Errorf("windowTitle is required")
	}
	// 使用 powershell 关闭窗口
	ps := fmt.Sprintf(`Get-Process | Where-Object {$_.MainWindowTitle -like '*%s*'} | ForEach-Object { $_.CloseMainWindow() }`, title)
	cmd := exec.Command("powershell", "-Command", ps)
	out, err := cmd.CombinedOutput()
	ctx.Logger.Debugf("killWindow output: %s", string(out))
	if err != nil {
		// CloseMainWindow 可能返回错误，窗口关闭即可
		if strings.Contains(string(out), title) || strings.Contains(string(out), "CloseMainWindow") {
			return nil
		}
		return fmt.Errorf("kill window %s failed: %w", title, err)
	}
	return nil
}

// WaitAction 等待指定毫秒
type WaitAction struct{}

func (a *WaitAction) Type() string { return "wait" }
func (a *WaitAction) Execute(ctx *Context, params map[string]string) error {
	ms := params["millis"]
	n, err := strconv.Atoi(ms)
	if err != nil {
		return fmt.Errorf("invalid millis: %s", ms)
	}
	time.Sleep(time.Duration(n) * time.Millisecond)
	return nil
}

// RunCommandAction 执行允许列表中的命令
type RunCommandAction struct{}

func (a *RunCommandAction) Type() string { return "runCommand" }
func (a *RunCommandAction) Execute(ctx *Context, params map[string]string) error {
	cmdLine := params["command"]
	if cmdLine == "" {
		return fmt.Errorf("command is required")
	}

	parts, err := splitCommand(cmdLine)
	if err != nil {
		return fmt.Errorf("parse command failed: %w", err)
	}
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	executable := parts[0]
	args := parts[1:]

	if !isCommandAllowed(executable, args, ctx.AllowedCommands) {
		ctx.Logger.Errorf("command not allowed: %s", cmdLine)
		return fmt.Errorf("command not allowed")
	}

	if _, err := os.Stat(executable); err != nil {
		return fmt.Errorf("executable not found: %s", executable)
	}

	ctx.Logger.Infof("run command: %s %v", executable, args)
	cmd := exec.Command(executable, args...)
	hideWindow(cmd)
	if params["workDir"] != "" {
		cmd.Dir = params["workDir"]
	}
	out, err := cmd.CombinedOutput()
	ctx.Logger.Debugf("runCommand output: %s", string(out))
	return err
}

// splitCommand 拆分命令行为可执行文件与参数，支持双引号
func splitCommand(line string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inQuote := false

	for _, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unbalanced quote")
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts, nil
}

func isCommandAllowed(executable string, args []string, allowed []config.AllowedCommand) bool {
	if len(allowed) == 0 {
		return false
	}
	absExec, err := filepath.Abs(executable)
	if err != nil {
		absExec = executable
	}
	argsStr := strings.Join(args, " ")
	for _, ac := range allowed {
		if !strings.EqualFold(ac.Path, absExec) {
			continue
		}
		if ac.ArgsPattern == "" {
			return len(args) == 0
		}
		re, err := regexp.Compile(ac.ArgsPattern)
		if err != nil {
			continue
		}
		return re.MatchString(argsStr)
	}
	return false
}

// CheckPort 检查端口是否监听
func CheckPort(host string, port int) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// CheckProcess 检查进程是否存在
func CheckProcess(name string) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", name))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), name)
}

// resolveWorkDir 根据脚本路径解析工作目录
func resolveWorkDir(scriptPath string) string {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return ""
	}
	return filepath.Dir(scriptPath)
}

// sanitizeScriptPath 清洗脚本路径，移除 Windows 路径中可能混入的 Unicode 控制字符
func sanitizeScriptPath(scriptPath string) string {
	return strings.TrimFunc(scriptPath, func(r rune) bool {
		return r == '\u202A' || r == '\u202B' || r == '\u202C' || r == '\u202D' || r == '\u202E' || r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069' || r == '\uFEFF'
	})
}

// RunStartScript 执行启动脚本
func RunStartScript(ctx *Context, scriptPath, workDir string, sync bool) error {
	scriptPath = sanitizeScriptPath(scriptPath)
	if scriptPath == "" {
		return fmt.Errorf("start script is empty")
	}
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("start script not found: %s", scriptPath)
	}
	if workDir == "" {
		workDir = resolveWorkDir(scriptPath)
	} else {
		workDir = sanitizeScriptPath(workDir)
	}
	ctx.Logger.Infof("run start script: %s (sync=%v, workDir=%s)", scriptPath, sync, workDir)

	cmd := exec.Command("cmd", "/c", scriptPath)
	hideWindow(cmd)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if sync {
		out, err := cmd.CombinedOutput()
		ctx.Logger.Debugf("start script output: %s", string(out))
		if err != nil {
			return fmt.Errorf("start script failed: %w", err)
		}
		return nil
	}
	// 异步模式下直接启动子进程并立即返回
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start script failed: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

// StopProject 执行停止脚本
func StopProject(ctx *Context, scriptPath, workDir string) error {
	scriptPath = sanitizeScriptPath(scriptPath)
	if scriptPath == "" {
		return nil
	}
	if workDir == "" {
		workDir = resolveWorkDir(scriptPath)
	} else {
		workDir = sanitizeScriptPath(workDir)
	}
	ctx.Logger.Infof("run stop script: %s (workDir=%s)", scriptPath, workDir)
	cmd := exec.Command("cmd", "/c", scriptPath)
	hideWindow(cmd)
	if workDir != "" {
		cmd.Dir = workDir
	}
	_, err := cmd.CombinedOutput()
	return err
}

// CtxWithTimeout 带超时的 context
func CtxWithTimeout(seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}
