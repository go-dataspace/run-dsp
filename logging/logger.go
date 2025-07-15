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
	"log/slog"
	"os"

	"github.com/charmbracelet/log"
)

// New will initialise a new structured logger, logging at the desired level.
// If the requested level doesn't exist, it panics.
// If humanReadable is set, it will use the coloured logger from charmbracelet,
// if not, it will use JSON.
func New(requestedLevel string, humanReadable bool) *slog.Logger {
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
		handler = log.NewWithOptions(os.Stderr, log.Options{
			Level:           log.Level(level),
			ReportTimestamp: true,
			ReportCaller:    true,
		})
	}

	return slog.New(handler)
}
