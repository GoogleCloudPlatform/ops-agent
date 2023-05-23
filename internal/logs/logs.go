// Copyright 2020, Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	messageKey        = "message"
	severityKey       = "logging.googleapis.com/severity"
	sourceLocationKey = "logging.googleapis.com/sourceLocation"
	timeKey           = "timestamp"
)

type StructuredLogger interface {
	Infof(format string, v ...any)
	Errorf(format string, v ...any)
	Println(v ...any)
}

type ZapStructuredLogger struct {
	logger *zap.SugaredLogger
}

func severityEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var severity string

	switch level {
	case zapcore.ErrorLevel:
		severity = "ERROR"
	case zapcore.WarnLevel:
		severity = "WARNING"
	case zapcore.InfoLevel:
		severity = "INFO"
	case zapcore.DebugLevel:
		severity = "DEBUG"
	default:
		severity = "DEFAULT"
	}
	enc.AppendString(severity)
}

type stringMap map[string]string

func (sm stringMap) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for k, v := range sm {
		enc.AddString(k, v)
	}
	return nil
}
func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	ae, ok := enc.(zapcore.ArrayEncoder)
	if !ok {
		zapcore.RFC3339NanoTimeEncoder(t, enc)
		return
	}
	nanos := t.UnixNano()
	sec := float64(nanos) / float64(time.Second)
	ae.AppendObject(stringMap{
		"seconds": fmt.Sprintf("%f", sec),
		"nanos":   fmt.Sprintf("%d", nanos),
	})
}

func sourceLocationEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	ae, ok := enc.(zapcore.ArrayEncoder)
	if !ok {
		zapcore.FullCallerEncoder(caller, enc)
		return
	}
	ae.AppendObject(stringMap{
		"file":     caller.File,
		"function": caller.Function,
		"line":     strconv.Itoa(caller.Line),
	})
}

func New(file string) *ZapStructuredLogger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.CallerKey = sourceLocationKey
	cfg.EncoderConfig.MessageKey = messageKey
	cfg.EncoderConfig.LevelKey = severityKey
	cfg.EncoderConfig.TimeKey = timeKey
	cfg.EncoderConfig.EncodeTime = timeEncoder
	cfg.EncoderConfig.EncodeLevel = severityEncoder
	cfg.EncoderConfig.EncodeCaller = sourceLocationEncoder

	cfg.OutputPaths = []string{
		file,
	}
	logger, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return Default()
	}

	sugar := logger.Sugar().With(
		zap.String("ops-agent-version", version.Version))
	return &ZapStructuredLogger{
		logger: sugar,
	}
}

func DiscardLogger() (*ZapStructuredLogger, *observer.ObservedLogs) {
	observedZapCore, observedLogs := observer.New(zap.InfoLevel)
	observedLogger := zap.New(observedZapCore)
	fileLogger := &ZapStructuredLogger{
		logger: observedLogger.Sugar(),
	}
	return fileLogger, observedLogs
}

func Default() *ZapStructuredLogger {
	logger, err := zap.NewProduction()
	if err != nil {
		logger, _ := DiscardLogger()
		return logger
	}
	sugar := logger.Sugar().With(
		zap.String("version", version.Version))
	return &ZapStructuredLogger{
		logger: sugar,
	}
}

func (f ZapStructuredLogger) Infof(format string, v ...any) {
	f.logger.Infof(format, v...)
}

func (f ZapStructuredLogger) Errorf(format string, v ...any) {
	f.logger.Errorf(format, v...)
}

func (f ZapStructuredLogger) Println(v ...any) {
	f.logger.Infoln(v...)
}

type SimpleLogger struct {
	l *log.Logger
}

func (sl SimpleLogger) Fatalf(format string, v ...any) {
	sl.l.Fatalf(format, v...)
}

func (sl SimpleLogger) Printf(format string, v ...any) {
	sl.l.Printf(format, v...)
}

func (sl SimpleLogger) Infof(format string, v ...any) {
	sl.l.Printf(format, v...)
}

func (sl SimpleLogger) Errorf(format string, v ...any) {
	sl.l.Printf(format, v...)
}

func (sl SimpleLogger) Println(v ...any) {
	sl.l.Println(v...)
}

func NewSimpleLogger() SimpleLogger {
	return SimpleLogger{log.New(os.Stdout, log.Prefix(), log.Flags())}
}
