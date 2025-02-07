// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package logging provides logging utilities.
package logging

import (
	"context"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

// create a new type for the context key, as context doesn't allow string as the key for
// collision reasons.
type contextKeyType string

const contextKey contextKeyType = "logger"

// New will initialise a new structured logger with JSON output, logging at the desired level.
// If the requested level doesn't exist, it panics.
func NewJSON(requestedLevel string, humanReadable bool) *slog.Logger {
	var level slog.Level
	switch requestedLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		panic("unknown log level")
	}
	opts := slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}
	var handler slog.Handler
	handler = slog.NewJSONHandler(os.Stdout, &opts)
	if humanReadable {
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			AddSource: true,
			Level:     level,
		})
	}
	return slog.New(handler)
}

// Inject will inject a logger into the context.
func Inject(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey, logger)
}

// Extract will extract a logger from the context. If no logger is found, a default logger
// with level info is returned.
func Extract(ctx context.Context) *slog.Logger {
	ctxVal := ctx.Value(contextKey)
	if ctxVal == nil {
		logger := slog.Default()
		logger.Debug("logger not found in context, using default logger")
		return logger
	}
	logger, ok := ctxVal.(*slog.Logger)
	if !ok {
		panic("logger in context is not of type *slog.Logger")
	}
	return logger
}

// InjectLabels injects labels to the logger living in the context, it will return both the context
// and the logger.
func InjectLabels(ctx context.Context, labels ...any) (context.Context, *slog.Logger) {
	logger := Extract(ctx).With(labels...)
	ctx = Inject(ctx, logger)
	return ctx, logger
}
