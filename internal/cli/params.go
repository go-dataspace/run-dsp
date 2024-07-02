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

package cli

import (
	"context"
	"log/slog"

	"github.com/go-dataspace/run-dsp/logging"
)

// Params is a simple interface for passing parameters to subcommands.
type Params interface {
	Debug() bool
	Context() context.Context
}

// ContreteParams are parameters for subcommands, they include global options and things like
// loggers, contexts, etc.
type ConcreteParams struct {
	logger *slog.Logger
	debug  bool
	ctx    context.Context
}

// GenParams generates a new Params object based on the global options.
func GenParams(g GlobalOptions) *ConcreteParams {
	ctx := context.Background()
	logLevel := g.LogLevel
	humanReadable := false
	if g.Debug {
		logLevel = "debug"
		humanReadable = true
	}
	return &ConcreteParams{
		ctx:   logging.Inject(ctx, logging.NewJSON(logLevel, humanReadable)),
		debug: g.Debug,
	}
}

// Logger returns the logger.
func (p *ConcreteParams) Logger() *slog.Logger {
	return p.logger
}

// Debug returns the debug value.
func (p *ConcreteParams) Debug() bool {
	return p.debug
}

// Context returns the context.
func (p *ConcreteParams) Context() context.Context {
	return p.ctx
}
