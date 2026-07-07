package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// rotateWriter 按日期 + 按大小滚动的日志写入器
type rotateWriter struct {
	mu         sync.Mutex
	logDir     string
	instance   string
	baseName   string // daemon-a
	currentDay string
	file       *os.File
	size       int64

	maxSizeMB int64
	maxAge    int
}

func newRotateWriter(logDir, instance string, maxSizeMB int64, maxAge int) (*rotateWriter, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	rw := &rotateWriter{
		logDir:    logDir,
		instance:  instance,
		baseName:  fmt.Sprintf("daemon-%s", instance),
		maxSizeMB: maxSizeMB,
		maxAge:    maxAge,
	}
	if err := rw.open(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (w *rotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("20060102")
	if today != w.currentDay || w.size >= w.maxSizeMB*1024*1024 {
		if err := w.rotate(today); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Sync()
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *rotateWriter) open() error {
	w.currentDay = time.Now().Format("20060102")
	path := w.currentPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotateWriter) currentPath() string {
	return filepath.Join(w.logDir, fmt.Sprintf("%s-%s.log", w.baseName, w.currentDay))
}

func (w *rotateWriter) rotate(today string) error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	// 如果不是新的一天，按大小滚动当前日期的文件
	if today == w.currentDay {
		_ = w.rotateBySize()
	}

	w.currentDay = today
	return w.open()
}

func (w *rotateWriter) rotateBySize() error {
	src := w.currentPath()
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	for i := 1; i < 1000; i++ {
		dst := filepath.Join(w.logDir, fmt.Sprintf("%s-%s.%03d.log", w.baseName, w.currentDay, i))
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			return os.Rename(src, dst)
		}
	}
	return fmt.Errorf("too many log rotations")
}

func (w *rotateWriter) cleanOld() {
	if w.maxAge <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -w.maxAge)
	entries, err := os.ReadDir(w.logDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, w.baseName+"-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(w.logDir, name))
		}
	}
}

// Logger 增强日志器，支持内存缓冲、文件输出和关键事件存储
type Logger struct {
	*logrus.Logger
	buffer *logBuffer
	Events *EventStore
}

type logBuffer struct {
	mu      sync.Mutex
	lines   []string
	maxSize int
}

func newLogBuffer(maxSize int) *logBuffer {
	return &logBuffer{lines: make([]string, 0, maxSize), maxSize: maxSize}
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	b.lines = append(b.lines, line)
	if len(b.lines) > b.maxSize {
		b.lines = b.lines[len(b.lines)-b.maxSize:]
	}
	return len(p), nil
}

func (b *logBuffer) Get() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]string, len(b.lines))
	copy(result, b.lines)
	return result
}

// New 创建按实例区分的日志器
func New(logDir, instance string) *Logger {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetLevel(logrus.InfoLevel)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "create log dir failed: %v\n", err)
	}

	var writers []io.Writer

	fileWriter, err := newRotateWriter(logDir, instance, 10, 30)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create rotate writer failed: %v\n", err)
	} else {
		writers = append(writers, fileWriter)
		// 每天首次打开时清理过期日志
		fileWriter.cleanOld()
	}

	buf := newLogBuffer(20)
	writers = append(writers, buf)

	log.SetOutput(io.MultiWriter(writers...))

	events := NewEventStore(logDir, instance, 100)

	return &Logger{Logger: log, buffer: buf, Events: events}
}

// LoadAllRecentEvents 从所有实例事件文件中读取合并后的最近 n 条事件
func LoadAllRecentEvents(logDir string, n int) []Event {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}
	var all []Event
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "events-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(logDir, name))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var e Event
			if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
				continue
			}
			all = append(all, e)
		}
		_ = f.Close()
	}
	if len(all) == 0 {
		return nil
	}
	// 按时间升序
	sort.Slice(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

func NewWithWriter(out io.Writer, instance string) *Logger {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetLevel(logrus.InfoLevel)

	buf := newLogBuffer(20)
	log.SetOutput(io.MultiWriter(out, buf))

	return &Logger{Logger: log, buffer: buf, Events: NewEventStore("", instance, 100)}
}

// RecentLogs 获取最近日志
func (l *Logger) RecentLogs() []string {
	return l.buffer.Get()
}

// Close 关闭日志器底层文件
func (l *Logger) Close() error {
	if l.Logger != nil {
		// 先置空输出，避免后续写入再打开文件
		l.Logger.SetOutput(io.Discard)
		// 再等待一小段时间确保 Windows 释放句柄
		time.Sleep(50 * time.Millisecond)
		if l.Logger.Out != nil {
			if closer, ok := l.Logger.Out.(io.Closer); ok {
				_ = closer.Close()
			}
		}
	}
	return nil
}
