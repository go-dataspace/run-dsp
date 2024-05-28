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

// Package server provides the server subcommand.
package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-dataspace/run-dsp/dsp"
	"github.com/go-dataspace/run-dsp/internal/cli"
	"github.com/go-dataspace/run-dsp/logging"

	sloghttp "github.com/samber/slog-http"
)

// Command contains all the server parameters.
type Command struct {
	ListenAddr string `help:"Listen address" default:"0.0.0.0" env:"LISTEN_ADDR"`
	Port       int    `help:"Listen port" default:"8080" env:"PORT"`
}

// Run starts the server.
func (c *Command) Run(p cli.Params) error {
	ctx := p.Context()
	logger := logging.Extract(ctx)

	logger.Info("Starting server", "listenAddr", c.ListenAddr, "port", c.Port)

	mux := dsp.GetRoutes()
	handler := sloghttp.Recovery(mux)
	handler = sloghttp.New(logger)(handler)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", c.ListenAddr, c.Port),
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
	}
	return srv.ListenAndServe()
}
