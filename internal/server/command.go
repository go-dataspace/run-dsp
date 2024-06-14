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
	"net/url"
	"time"

	"github.com/go-dataspace/run-dsp/dsp"
	"github.com/go-dataspace/run-dsp/internal/auth"
	"github.com/go-dataspace/run-dsp/internal/cli"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine"
	"github.com/go-dataspace/run-dsp/internal/fsprovider"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/justinas/alice"

	sloghttp "github.com/samber/slog-http"
)

// Command contains all the server parameters.
type Command struct {
	ListenAddr string `help:"Listen address" default:"0.0.0.0" env:"LISTEN_ADDR"`
	Port       int    `help:"Listen port" default:"8080" env:"PORT"`
	ServerURL  string `help:"External URL of the server, e.g. https://example.org" default:"http://127.0.0.1:8080/" env:"SERVER_URL"` //nolint:lll
	ServePath  string `help:"Path for fileserver" default:"/var/tmp/run-dsp/fileserver" env:"SERVE_PATH"`                             //nolint:lll
}

// Run starts the server.
func (c *Command) Run(p cli.Params) error {
	ctx := p.Context()
	logger := logging.Extract(ctx)

	logger.Info("Starting server", "listenAddr", c.ListenAddr, "port", c.Port)

	idChannel := make(chan dspstatemachine.StateStorageChannelMessage, 10)
	stateStorage := dspstatemachine.GetStateStorage(ctx)
	go stateStorage.AllowStateToProgress(idChannel)
	url, err := url.Parse(c.ServerURL)
	if err != nil {
		return fmt.Errorf("Invalid URL: %s", c.ServerURL)
	}
	publisher := fsprovider.NewPublisher(ctx, constants.FileServePath, url)
	provider := fsprovider.New(c.ServePath, publisher)

	mux := http.NewServeMux()

	baseMW := alice.New(
		sloghttp.Recovery,
		sloghttp.New(logger),
		logging.NewMiddleware(logger),
	)
	mux.Handle(constants.FileServePath+"/", http.StripPrefix(
		constants.FileServePath,
		baseMW.Then(publisher.Mux()),
	))
	mux.Handle("/.well-known/", http.StripPrefix(
		"/.well-known",
		baseMW.Append(jsonHeaderMiddleware).Then(dsp.GetWellKnownRoutes()),
	))
	mux.Handle(constants.APIPath+"/", http.StripPrefix(
		constants.APIPath,
		baseMW.Append(
			jsonHeaderMiddleware,
			auth.NonsenseUserInjector,
			dspstatemachine.NewMiddleware(idChannel),
		).Then(dsp.GetDSPRoutes(provider)),
	))

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", c.ListenAddr, c.Port),
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	return srv.ListenAndServe()
}
