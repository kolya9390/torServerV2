package log

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	RequestIDHeader     = "X-Request-ID"
	ContextKeyRequestID = "request_id"
)

var (
	logger      *zap.SugaredLogger
	errorLogger *zap.SugaredLogger
	once        sync.Once
	globalLevel zapcore.Level = zapcore.InfoLevel
)

// Init инициализирует логгер с указанными путями.
func Init(logPath, webLogPath string) {
	once.Do(func() {
		config := zap.NewProductionConfig()

		config.Level = zap.NewAtomicLevelAt(globalLevel)

		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		var writers []zapcore.WriteSyncer
		writers = append(writers, zapcore.AddSync(os.Stdout))

		if logPath != "" {
			file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				writers = append(writers, zapcore.AddSync(file))
			}
		}

		core := zapcore.NewCore(
			zapcore.NewConsoleEncoder(config.EncoderConfig),
			zapcore.NewMultiWriteSyncer(writers...),
			config.Level,
		)

		logger = zap.New(core).Sugar()

		if webLogPath != "" && webLogPath != logPath {
			errFile, err := os.OpenFile(webLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				errCore := zapcore.NewCore(
					zapcore.NewJSONEncoder(config.EncoderConfig),
					zapcore.AddSync(errFile),
					zapcore.ErrorLevel,
				)
				errorLogger = zap.New(errCore).Sugar()
			}
		}
	})
}

// Close закрывает логгер.
func Close() {
	if logger != nil {
		_ = logger.Sync()
	}
}

// TLogln логирует информационное сообщение (совместимость со старым API).
func TLogln(args ...any) {
	if logger != nil {
		logger.Info(args...)
	} else {
		// Fallback если логгер не инициализирован
		fmt.Println(args...)
	}
}

// TLoglnF логирует форматированное информационное сообщение.
func TLoglnF(format string, args ...any) {
	if logger != nil {
		logger.Infof(format, args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// WebLogln логирует сообщения веб-сервера (совместимость).
func WebLogln(args ...any) {
	TLogln(args...)
}

// Info логирует информационное сообщение.
func Info(msg string, keysAndValues ...any) {
	if logger != nil {
		logger.Infow(msg, keysAndValues...)
	}
}

// Warn логирует предупреждение.
func Warn(msg string, keysAndValues ...any) {
	if logger != nil {
		logger.Warnw(msg, keysAndValues...)
	}
}

// Error логирует ошибку.
func Error(msg string, keysAndValues ...any) {
	if logger != nil {
		logger.Errorw(msg, keysAndValues...)
	}
}

// Debug логирует отладочное сообщение (только если включен debug уровень).
func Debug(msg string, keysAndValues ...any) {
	if logger != nil {
		logger.Debugw(msg, keysAndValues...)
	}
}

// SetLevel устанавливает уровень логирования.
func SetLevel(level string) error {
	var l zapcore.Level

	switch level {
	case "debug":
		l = zapcore.DebugLevel
	case "info":
		l = zapcore.InfoLevel
	case "warn":
		l = zapcore.WarnLevel
	case "error":
		l = zapcore.ErrorLevel
	default:
		return fmt.Errorf("unknown log level: %s", level)
	}

	globalLevel = l

	return nil
}

// RequestIDMiddleware добавляет correlation ID в каждый запрос.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Set(ContextKeyRequestID, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func generateRequestID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(8))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}

	return string(b)
}

func GetRequestID(c *gin.Context) string {
	if id, exists := c.Get(ContextKeyRequestID); exists {
		return id.(string)
	}

	return ""
}

// WebLogger возвращает Gin middleware для логирования HTTP запросов.
func WebLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		requestID := GetRequestID(c)

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		fields := []any{
			"request_id", requestID,
			"status", statusCode,
			"method", method,
			"path", path,
			"query", query,
			"ip", clientIP,
			"latency_ms", latency.Milliseconds(),
		}

		if statusCode >= 500 {
			Error("HTTP request",
				fields...,
			)

			if errorLogger != nil {
				errorLogger.Errorw("HTTP request error",
					fields...,
				)
			}
		} else if statusCode >= 400 {
			Warn("HTTP request",
				fields...,
			)
		} else {
			Info("HTTP request",
				fields...,
			)
		}
	}
}
