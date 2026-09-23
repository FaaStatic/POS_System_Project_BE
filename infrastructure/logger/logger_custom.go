package logger

import (
	"context"
	"errors"
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

func LoggerCustom(env string) {

	var (
		err error
	)

	if env == "production" {
		Logger, err = zap.NewProduction()
		if err != nil {
			panic("Failed to initialize logger!")
		}
	} else {

		config := zap.NewDevelopmentConfig()

		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		Logger, err = config.Build()
		if err != nil {
			panic("Failed to initialize logger!")
		}
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
