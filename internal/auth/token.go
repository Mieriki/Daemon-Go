// Package auth 提供本地访问的 Token 认证能力
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TokenFileName Token 文件名称
const TokenFileName = ".guard"

// Manager Token 管理器
type Manager struct {
	mu    sync.RWMutex
	token string
	path  string
}

// NewManager 创建或加载 Token
func NewManager(dir string) (*Manager, error) {
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, TokenFileName)
	m := &Manager{path: path}

	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		s := strings.TrimSpace(string(data))
		if idx := strings.Index(s, "="); idx >= 0 {
			s = strings.TrimSpace(s[idx+1:])
		}
		if s != "" {
			m.token = s
			return m, nil
		}
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token failed: %w", err)
	}
	m.token = token
	if err := os.WriteFile(path, []byte("token="+token+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("write token file failed: %w", err)
	}
	return m, nil
}

// Token 返回当前 Token
func (m *Manager) Token() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.token
}

// Validate 校验 Token
func (m *Manager) Validate(token string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.token == "" || token == "" {
		return false
	}
	return m.token == token
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
