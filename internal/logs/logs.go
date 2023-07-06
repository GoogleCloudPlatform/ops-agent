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
	"log"
	"os"
	"strconv"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	messageKey        = "message"
	severityKey       = "severity"
	sourceLocationKey = "sourceLocation"
	timeKey           = "time"
)

type StructuredLogger interface {
	// Infof, Warnf, Errorf use fmt.Sprintf to log a templated message.
	Infof(format string, v ...any)
	Warnf(format string, v ...any)
	Errorf(format string, v ...any)
	// Infow, Warnw, Errorw log a string message and optionally
	// can add key, value pairs of metadata to the log depending
	// on the underlying implementation of the interface.
	Infow(msg string, keysAndValues ...any)
	Warnw(msg string, keysAndValues ...any)
	Errorw(msg string, keysAndValues ...any)
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
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.CallerKey = sourceLocationKey
	cfg.EncoderConfig.MessageKey = messageKey
	cfg.EncoderConfig.LevelKey = severityKey
	cfg.EncoderConfig.TimeKey = timeKey
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.EncoderConfig.EncodeLevel = severityEncoder
	cfg.EncoderConfig.EncodeCaller = sourceLocationEncoder

	cfg.OutputPaths = []string{
		file,
	}
	logger, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return Default()
	}
	return &ZapStructuredLogger{
		logger: logger.Sugar(),
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
	return &ZapStructuredLogger{
		logger: logger.Sugar(),
	}
}

func (f ZapStructuredLogger) Infof(format string, v ...any) {
	f.logger.Infof(format, v...)
}

func (f ZapStructuredLogger) Warnf(format string, v ...any) {
	f.logger.Warnf(format, v...)
}

func (f ZapStructuredLogger) Errorf(format string, v ...any) {
	f.logger.Errorf(format, v...)
}

// A zap logger has an implementation of Infow, Warnw, Errorw that will
// add a key, value pairs to the output json structured log.
// Link: https://pkg.go.dev/go.uber.org/zap#SugaredLogger.Infow
func (f ZapStructuredLogger) Infow(msg string, keysAndValues ...any) {
	f.logger.Infow(msg, keysAndValues...)
}

func (f ZapStructuredLogger) Warnw(msg string, keysAndValues ...any) {
	f.logger.Warnw(msg, keysAndValues...)
}

func (f ZapStructuredLogger) Errorw(msg string, keysAndValues ...any) {
	f.logger.Errorw(msg, keysAndValues...)
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

func (sl SimpleLogger) Warnf(format string, v ...any) {
	sl.l.Printf(format, v...)
}

func (sl SimpleLogger) Errorf(format string, v ...any) {
	sl.l.Printf(format, v...)
}

// A SimpleLogger doesn't have the capability of adding metadata
// to the resulting log.
func (sl SimpleLogger) Infow(msg string, keysAndValues ...any) {
	sl.l.Println(msg)
}

func (sl SimpleLogger) Warnw(msg string, keysAndValues ...any) {
	sl.l.Println(msg)
}

func (sl SimpleLogger) Errorw(msg string, keysAndValues ...any) {
	sl.l.Println(msg)
}

func (sl SimpleLogger) Println(v ...any) {
	sl.l.Println(v...)
}

func NewSimpleLogger() SimpleLogger {
	return SimpleLogger{log.New(os.Stdout, log.Prefix(), log.Flags())}
}
