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

	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type StructuredLogger interface {
	Infof(format string, v ...any)
	Errorf(format string, v ...any)
	Println(v ...any)
}

type ZapStructuredLogger struct {
	logger *zap.SugaredLogger
}

func New(file string) *ZapStructuredLogger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

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

func DiscardLogger() *ZapStructuredLogger {
	observedZapCore, _ := observer.New(zap.InfoLevel)
	observedLogger := zap.New(observedZapCore)
	fileLogger := &ZapStructuredLogger{
		logger: observedLogger.Sugar(),
	}
	return fileLogger
}

func Default() *ZapStructuredLogger {
	logger, err := zap.NewProduction()
	if err != nil {
		return DiscardLogger()
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
