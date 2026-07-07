package logger

import (
	"os"
	"testing"
)

func TestEventStoreAppendWrite(t *testing.T) {
	dir := t.TempDir()
	es := NewEventStore(dir, "a", 5)
	es.Add(EventProjectDown, "App", "down1", "")
	es.Add(EventProjectStart, "App", "start", "")

	path := es.path
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat events file failed: %v", err)
	}

	es.Add(EventProjectStarted, "App", "started", "")
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat events file failed: %v", err)
	}

	if info2.Size() <= info1.Size() {
		t.Errorf("file size did not grow after append: before=%d after=%d", info1.Size(), info2.Size())
	}

	// 验证事件能正确加载
	es2 := NewEventStore(dir, "a", 5)
	all := es2.All()
	if len(all) != 3 {
		t.Fatalf("loaded count = %d, want 3", len(all))
	}
	if all[2].Type != EventProjectStarted {
		t.Errorf("last event type = %s, want %s", all[2].Type, EventProjectStarted)
	}
}

func TestEventStoreLoadTrim(t *testing.T) {
	dir := t.TempDir()
	es := NewEventStore(dir, "a", 10)
	for i := 0; i < 12; i++ {
		es.Add(EventProjectDown, "App", "down", "")
	}
	if len(es.All()) != 10 {
		t.Fatalf("memory count = %d, want 10", len(es.All()))
	}

	// 追加写会让文件里有 12 条，再次加载后应裁剪为 10 条并重写
	es2 := NewEventStore(dir, "a", 10)
	all := es2.All()
	if len(all) != 10 {
		t.Fatalf("after load trim count = %d, want 10", len(all))
	}

	info, err := os.Stat(es2.path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	// 10 条 JSON 行，每条约 80 字节，文件应明显小于 12 条时
	if info.Size() > 2000 {
		t.Errorf("trimmed file too large: %d bytes", info.Size())
	}
}
