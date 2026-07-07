package logger

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventStoreAddAndRecent(t *testing.T) {
	dir := t.TempDir()
	es := NewEventStore(dir, "a", 5)

	es.Add(EventProjectDown, "App", "app down", "detail")
	time.Sleep(10 * time.Millisecond)
	es.Add(EventProjectStart, "App", "app start", "")
	time.Sleep(10 * time.Millisecond)
	es.Add(EventProjectStarted, "App", "app started", "pid=123")

	recent := es.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("recent count = %d, want 2", len(recent))
	}
	if recent[0].Type != EventProjectStart {
		t.Errorf("recent[0].type = %s, want %s", recent[0].Type, EventProjectStart)
	}
	if recent[1].Type != EventProjectStarted {
		t.Errorf("recent[1].type = %s, want %s", recent[1].Type, EventProjectStarted)
	}
}

func TestEventStoreMaxSize(t *testing.T) {
	dir := t.TempDir()
	es := NewEventStore(dir, "a", 3)

	for i := 0; i < 5; i++ {
		es.Add(EventProjectDown, "App", "down", "")
	}

	all := es.All()
	if len(all) != 3 {
		t.Errorf("all count = %d, want 3", len(all))
	}

	recent := es.Recent(10)
	if len(recent) != 3 {
		t.Errorf("recent count = %d, want 3", len(recent))
	}
}

func TestEventStorePersistence(t *testing.T) {
	dir := t.TempDir()
	es := NewEventStore(dir, "a", 10)
	es.Add(EventPeerDown, "b", "peer down", "")

	// 重新创建，验证从文件加载
	es2 := NewEventStore(dir, "a", 10)
	all := es2.All()
	if len(all) != 1 {
		t.Fatalf("loaded count = %d, want 1", len(all))
	}
	if all[0].Type != EventPeerDown {
		t.Errorf("loaded type = %s, want %s", all[0].Type, EventPeerDown)
	}
}

func TestLoadAllRecentEvents(t *testing.T) {
	dir := t.TempDir()
	esA := NewEventStore(dir, "a", 10)
	esB := NewEventStore(dir, "b", 10)

	past := time.Now().Add(-time.Hour)
	esA.events = append(esA.events, Event{Time: past, Type: EventProjectDown, Target: "App", Message: "old"})
	esA.persistLocked()

	now := time.Now()
	esB.events = append(esB.events, Event{Time: now, Type: EventPeerRecover, Target: "a", Message: "new"})
	esB.persistLocked()

	merged := LoadAllRecentEvents(dir, 10)
	if len(merged) != 2 {
		t.Fatalf("merged count = %d, want 2", len(merged))
	}
	if merged[0].Type != EventProjectDown {
		t.Errorf("merged[0].type = %s, want project_down", merged[0].Type)
	}
	if merged[1].Type != EventPeerRecover {
		t.Errorf("merged[1].type = %s, want peer_recover", merged[1].Type)
	}

	limited := LoadAllRecentEvents(dir, 1)
	if len(limited) != 1 {
		t.Fatalf("limited count = %d, want 1", len(limited))
	}
	if limited[0].Type != EventPeerRecover {
		t.Errorf("limited[0].type = %s, want peer_recover", limited[0].Type)
	}
}

func TestRotateWriter(t *testing.T) {
	dir := t.TempDir()
	rw, err := newRotateWriter(dir, "test", 10, 1)
	if err != nil {
		t.Fatalf("new rotate writer failed: %v", err)
	}
	defer rw.Close()

	// 直接检查 open() 创建的文件路径
	today := time.Now().Format("20060102")
	path := filepath.Join(dir, "daemon-test-"+today+".log")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected log file not created: %s", path)
	}

	data := []byte("hello world\n")
	if _, err := rw.Write(data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	// 确保文件写入并刷新
	if f := rw.file; f != nil {
		_ = f.Sync()
	}

	// 读取验证
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("log content = %s, want %s", string(content), string(data))
	}
}

func TestNewLogger(t *testing.T) {
	log := NewWithWriter(io.Discard, "test")
	if log == nil {
		t.Fatal("logger is nil")
	}
	if log.Events == nil {
		t.Fatal("event store is nil")
	}
	log.Info("test message")
	if len(log.RecentLogs()) == 0 {
		t.Error("expected recent logs not empty")
	}
}
