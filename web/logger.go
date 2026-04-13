package web

import (
	"sync"
	"time"
)

// LogEntry 日志条目
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // INFO | WARN | ERROR | TRADE
	Symbol  string    `json:"symbol"`
	Message string    `json:"message"`
}

// Logger 内存日志（最多保留 500 条）
type Logger struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

var GlobalLogger = &Logger{maxSize: 500}

// Add 添加日志
func (l *Logger) Add(level, symbol, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Symbol:  symbol,
		Message: msg,
	}
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
}

// GetAll 获取所有日志（倒序）
func (l *Logger) GetAll() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]LogEntry, len(l.entries))
	// 倒序返回（最新的在前）
	for i, e := range l.entries {
		result[len(l.entries)-1-i] = e
	}
	return result
}

// GetLast 获取最近 n 条
func (l *Logger) GetLast(n int) []LogEntry {
	all := l.GetAll()
	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}

// Info 记录信息日志
func Info(symbol, msg string) {
	GlobalLogger.Add("INFO", symbol, msg)
}

// Warn 记录警告日志
func Warn(symbol, msg string) {
	GlobalLogger.Add("WARN", symbol, msg)
}

// Error 记录错误日志
func Error(symbol, msg string) {
	GlobalLogger.Add("ERROR", symbol, msg)
}

// Trade 记录交易日志
func Trade(symbol, msg string) {
	GlobalLogger.Add("TRADE", symbol, msg)
}
