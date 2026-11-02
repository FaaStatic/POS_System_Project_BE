package logger

import (
	"context"
	"errors"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

type GormZapLogger struct {
	ZapLogger                 *zap.Logger
	LogLevel                  gormlogger.LogLevel
	SlowThreshold             time.Duration
	SkipCallerLookup          bool
	IgnoreRecordNotFoundError bool
}

var Logger *zap.Logger

// loggerConfig: production selalu JSON. Development memakai console berwarna
// hanya saat stderr adalah terminal; di container stderr bukan TTY, jadi
// outputnya JSON yang dibaca Alloy baris per baris lalu dikirim ke Loki.
func loggerConfig(env string, tty bool) zap.Config {
	cfg := zap.NewProductionConfig()
	if env != "production" {
		cfg = zap.NewDevelopmentConfig()
	}
	// Sampling bawaan zap membuang log identik di atas 100/detik, sehingga
	// jumlah "HTTP Request" di Loki bisa lebih kecil dari request sebenarnya.
	cfg.Sampling = nil

	if env != "production" && tty {
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
		return cfg
	}

	// Konfigurasi development memakai key satu huruf ("L", "T", "M"); JSON untuk
	// Loki harus memakai key yang sama di semua environment (level, ts, msg).
	// Durasi berupa angka detik (bukan "1.2ms") supaya bisa difilter di LogQL,
	// mis. | json | elapsed > 0.2
	encoder := zap.NewProductionEncoderConfig()
	encoder.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder.EncodeDuration = zapcore.SecondsDurationEncoder
	cfg.Encoding = "json"
	cfg.EncoderConfig = encoder
	return cfg
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func LoggerCustom(env string) {
	var err error
	Logger, err = loggerConfig(env, isTerminal(os.Stderr)).Build()
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
}

func NewGormZapLogger(zapLog *zap.Logger) *GormZapLogger {
	return &GormZapLogger{
		ZapLogger:                 zapLog,
		LogLevel:                  gormlogger.Info,
		SlowThreshold:             200 * time.Millisecond,
		SkipCallerLookup:          false,
		IgnoreRecordNotFoundError: true,
	}
}

func (l *GormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

func (l *GormZapLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	if l.LogLevel >= gormlogger.Info {
		l.ZapLogger.Sugar().Infof(msg, args...)
	}
}

func (l *GormZapLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	if l.LogLevel >= gormlogger.Warn {
		l.ZapLogger.Sugar().Warnf(msg, args...)
	}
}

func (l *GormZapLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	if l.LogLevel >= gormlogger.Error {
		l.ZapLogger.Sugar().Errorf(msg, args...)
	}
}

func (l *GormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	var (
		lvl   zapcore.Level
		msg   string
		extra zap.Field
	)
	switch {
	case err != nil && l.LogLevel >= gormlogger.Error:
		if errors.Is(err, gormlogger.ErrRecordNotFound) && l.IgnoreRecordNotFoundError {
			return
		}
		lvl, msg, extra = zapcore.ErrorLevel, "query error", zap.Error(err)
	case l.SlowThreshold != 0 && elapsed > l.SlowThreshold && l.LogLevel >= gormlogger.Warn:
		lvl, msg, extra = zapcore.WarnLevel, "slow query", zap.Duration("threshold", l.SlowThreshold)
	case l.LogLevel >= gormlogger.Info:
		lvl, msg, extra = zapcore.DebugLevel, "query", zap.Skip()
	default:
		return
	}

	if ce := l.ZapLogger.Check(lvl, msg); ce != nil {
		sql, rows := fc()
		ce.Write(
			zap.String("sql", sql),
			zap.Int64("rows", rows),
			zap.Duration("elapsed", elapsed),
			zap.String("caller", utils.FileWithLineNum()),
			extra,
		)
	}
}
