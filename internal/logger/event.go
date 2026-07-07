package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType 事件类型
type EventType string

const (
	EventPeerDown     EventType = "peer_down"
	EventPeerRecover  EventType = "peer_recover"
	EventProjectDown  EventType = "project_down"
	EventProjectStop  EventType = "project_stop"
	EventProjectStart EventType = "project_start"
	EventProjectStarted EventType = "project_started"
	EventProjectFailed EventType = "project_failed"
	EventDaemonPause  EventType = "daemon_pause"
	EventDaemonResume EventType = "daemon_resume"
	EventServiceRestart EventType = "service_restart"
	EventConfigSaved  EventType = "config_saved"
	EventManualStart  EventType = "manual_start"
	EventManualStop   EventType = "manual_stop"
	EventManualRestart EventType = "manual_restart"
)

// Event 关键事件
type Event struct {
	Time    time.Time `json:"time"`
	Type    EventType `json:"type"`
	Target  string    `json:"target"`
	Message string    `json:"message"`
	Detail  string    `json:"detail,omitempty"`
}

// EventStore 关键事件存储
type EventStore struct {
	mu       sync.Mutex
	path     string
	maxSize  int
	events   []Event
	instance string
}

// NewEventStore 创建事件存储
func NewEventStore(logDir, instance string, maxSize int) *EventStore {
	if maxSize <= 0 {
		maxSize = 100
	}
	_ = os.MkdirAll(logDir, 0755)
	es := &EventStore{
		path:     filepath.Join(logDir, fmt.Sprintf("events-%s.jsonl", instance)),
		maxSize:  maxSize,
		instance: instance,
	}
	es.load()
	return es
}

// Add 添加事件
func (s *EventStore) Add(t EventType, target, message, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := Event{
		Time:    time.Now(),
		Type:    t,
		Target:  target,
		Message: message,
		Detail:  detail,
	}
	s.events = append(s.events, e)
	if len(s.events) > s.maxSize {
		s.events = s.events[len(s.events)-s.maxSize:]
	}
	s.persistLocked()

	// 文件行数超过阈值时触发重写裁剪，避免无限增长
	if s.persistedCount() >= 1000 {
		s.rewriteLocked()
	}
}

// persistedCount 估算文件中的事件行数
func (s *EventStore) persistedCount() int {
	info, err := os.Stat(s.path)
	if err != nil || info.Size() == 0 {
		return len(s.events)
	}
	// 粗略估算：用当前事件平均 JSON 长度估算行数
	if len(s.events) == 0 {
		return 0
	}
	var total int
	for _, e := range s.events {
		b, _ := json.Marshal(e)
		total += len(b) + 1
	}
	avg := total / len(s.events)
	if avg == 0 {
		avg = 200
	}
	return int(info.Size()) / avg
}

// Recent 获取最近 n 条事件
func (s *EventStore) Recent(n int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 {
		return nil
	}
	if n > len(s.events) {
		n = len(s.events)
	}
	start := len(s.events) - n
	result := make([]Event, n)
	copy(result, s.events[start:])
	return result
}

// All 返回所有事件（调试用）
func (s *EventStore) All() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

func (s *EventStore) load() {
	f, err := os.Open(s.path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		s.events = append(s.events, e)
	}
	if len(s.events) > s.maxSize {
		s.events = s.events[len(s.events)-s.maxSize:]
		s.rewriteLocked()
	}
}

func (s *EventStore) persistLocked() {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "persist events failed: %v\n", err)
		return
	}
	defer f.Close()

	if len(s.events) == 0 {
		return
	}
	e := s.events[len(s.events)-1]
	b, _ := json.Marshal(e)
	_, _ = f.Write(b)
	_, _ = f.WriteString("\n")
}

func (s *EventStore) rewriteLocked() {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rewrite events failed: %v\n", err)
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, e := range s.events {
		b, _ := json.Marshal(e)
		_, _ = w.Write(b)
		_, _ = w.WriteString("\n")
	}
	_ = w.Flush()
}
