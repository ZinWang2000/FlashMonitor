package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const ringSize = 1000

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type Logger struct {
	mu      sync.Mutex
	ring    [ringSize]LogEntry
	head    int
	count   int
	logDir  string
	curDate string
	file    *os.File
}

var global *Logger

// Init 初始化全局日志记录器，在 logDir 中创建日志目录。
//
// 输入:
//   - logDir: 日志文件存储目录（不存在时自动创建）
//
// 输出:
//   - error: 目录创建失败的原因
//
// 注意事项:
//   - 调用后 Global() 返回有效实例；程序启动时应调用一次
func Init(logDir string) error {
	l := &Logger{logDir: logDir}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("logger: cannot create log dir: %w", err)
	}
	global = l
	return nil
}

func Global() *Logger { return global }

func Info(msg string) {
	if global != nil {
		global.log("INFO", msg)
	}
}

func Warn(msg string) {
	if global != nil {
		global.log("WARN", msg)
	}
}

func Error(msg string) {
	if global != nil {
		global.log("ERROR", msg)
	}
}

// Infof 以 INFO 级别记录格式化日志。
//
// 输入:
//   - format, args: 同 fmt.Sprintf 的格式字符串和参数
//
// 注意事项:
//   - 全局日志器未初始化时静默忽略
func Infof(format string, args ...interface{}) {
	Info(fmt.Sprintf(format, args...))
}

// Errorf 以 ERROR 级别记录格式化日志。
//
// 输入:
//   - format, args: 同 fmt.Sprintf 的格式字符串和参数
func Errorf(format string, args ...interface{}) {
	Error(fmt.Sprintf(format, args...))
}

func (l *Logger) log(level, msg string) {
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.ring[l.head%ringSize] = entry
	l.head++
	if l.count < ringSize {
		l.count++
	}

	l.writeToFile(entry)
}

func (l *Logger) writeToFile(entry LogEntry) {
	date := entry.Time.Format("20060102")
	if date != l.curDate {
		if l.file != nil {
			_ = l.file.Close()
			l.file = nil
		}
		l.curDate = date
	}
	if l.file == nil {
		path := fmt.Sprintf("%s/flashmonitor_%s.log", l.logDir, date)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		l.file = f
	}
	data, _ := json.Marshal(entry)
	_, _ = l.file.Write(append(data, '\n'))
}

// GetLast 返回最近 n 条日志，按时间正序排列。
//
// 输入:
//   - n: 请求条数；≤0 或超过缓冲区已有条数时返回全部
//
// 输出:
//   - []LogEntry: 最新 n 条日志，时间从旧到新
//
// 注意事项:
//   - 环形缓冲区最多保存 1000 条，超出后覆盖最旧条目
func (l *Logger) GetLast(n int) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	size := l.count
	if size > ringSize {
		size = ringSize
	}
	if n <= 0 || n > size {
		n = size
	}

	result := make([]LogEntry, n)
	for i := 0; i < n; i++ {
		pos := (l.head - 1 - i + ringSize*2) % ringSize
		result[i] = l.ring[pos]
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Close 关闭当前打开的日志文件，释放文件句柄。
//
// 注意事项:
//   - 程序退出前应调用，确保最后一批日志落盘
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}
