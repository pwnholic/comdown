package internal

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
	SUCCESS
)

type Logger struct {
	*log.Logger
	mu     sync.Mutex
	level  LogLevel
	writer io.Writer
}

var (
	defaultLogger *Logger
	once          sync.Once
)

func NewLogger(out io.Writer, level LogLevel) *Logger {
	return &Logger{
		Logger: log.New(out, "", 0),
		level:  level,
		writer: out,
	}
}

func InitDefaultLogger(level LogLevel) {
	once.Do(func() {
		defaultLogger = NewLogger(os.Stdout, level)
	})
}

func GetDefaultLogger() *Logger {
	if defaultLogger == nil {
		InitDefaultLogger(INFO)
	}
	return defaultLogger
}

func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) logInternal(level LogLevel, levelStr, format string, v ...any) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format(time.DateTime)
	msg := fmt.Sprintf(format, v...)
	logEntry := fmt.Sprintf("%s [%s] %s\n", timestamp, levelStr, msg)

	_, _ = l.writer.Write([]byte(logEntry))
}

func (l *Logger) Debug(format string, v ...any) {
	l.logInternal(DEBUG, "DEBUG", format, v...)
}

func (l *Logger) Info(format string, v ...any) {
	l.logInternal(INFO, "INFO", format, v...)
}

func (l *Logger) Warn(format string, v ...any) {
	l.logInternal(WARNING, "WARNING", format, v...)
}

func (l *Logger) Error(format string, v ...any) {
	l.logInternal(ERROR, "ERROR", format, v...)
}

func (l *Logger) Success(format string, v ...any) {
	l.logInternal(SUCCESS, "SUCCESS", format, v...)
}

func Debug(format string, v ...any) {
	GetDefaultLogger().Debug(format, v...)
}

func Info(format string, v ...any) {
	GetDefaultLogger().Info(format, v...)
}

func Warn(format string, v ...any) {
	GetDefaultLogger().Warn(format, v...)
}

func Error(format string, v ...any) {
	GetDefaultLogger().Error(format, v...)
}

func Success(format string, v ...any) {
	GetDefaultLogger().Success(format, v...)
}
