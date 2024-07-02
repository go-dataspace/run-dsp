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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-dataspace/run-dsp/dsp"
	"github.com/go-dataspace/run-dsp/internal/auth"
	"github.com/go-dataspace/run-dsp/internal/cli"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/justinas/alice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	sloghttp "github.com/samber/slog-http"
)

// Command contains all the server parameters.
//
//nolint:lll
type Command struct {
	ListenAddr string `help:"Listen address" default:"0.0.0.0" env:"LISTEN_ADDR"`
	Port       int    `help:"Listen port" default:"8080" env:"PORT"`

	ProviderAddress       string `help:"Address of provider GRPC endpoint" required:"" env:"PROVIDER_URL"`
	ProviderInsecure      bool   `help:"Provider connection does not use TLS" default:"false" env:"PROVIDER_INSECURE"`
	ProviderCACert        string `help:"Custom CA certificate for provider's TLS certificate" env:"PROVIDER_CA"`
	ProviderClientCert    string `help:"Client certificate to use to authenticate with provider" env:"PROVIDER_CLIENT_CERT"`
	ProviderClientCertKey string `help:"Key to the client certificate" env:"PROVIDER_CLIENT_CERT_KEY"`
}

// Validate checks if the parameters are correct.
func (c *Command) Validate() error {
	if c.ProviderInsecure {
		return nil
	}

	_, err := checkFile(c.ProviderCACert)
	if err != nil {
		return fmt.Errorf("Provider CA certificate: %w", err)
	}

	certSupplied, err := checkFile(c.ProviderClientCert)
	if err != nil {
		return fmt.Errorf("Provider client certificate: %w", err)
	}
	keySupplied, err := checkFile(c.ProviderClientCertKey)
	if err != nil {
		return fmt.Errorf("Provider client certificate key: %w", err)
	}

	if certSupplied != keySupplied {
		return fmt.Errorf("Need to supply both client certificate and its key.")
	}
	return nil
}

func checkFile(l string) (bool, error) {
	if l != "" {
		f, err := os.Stat(l)
		if err != nil {
			return true, fmt.Errorf("could not read %s: %w", l, err)
		}
		if f.IsDir() {
			return true, fmt.Errorf("%s is a directory", l)
		}
		return true, nil
	}
	return false, nil
}

// Run starts the server.
func (c *Command) Run(p cli.Params) error {
	ctx := p.Context()
	logger := logging.Extract(ctx)

	logger.Info("Starting server", "listenAddr", c.ListenAddr, "port", c.Port)

	idChannel := make(chan dspstatemachine.StateStorageChannelMessage, 10)
	stateStorage := dspstatemachine.GetStateStorage(ctx)
	go stateStorage.AllowStateToProgress(idChannel)
	provider, conn, err := c.getProvider(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	mux := http.NewServeMux()

	baseMW := alice.New(
		sloghttp.Recovery,
		sloghttp.New(logger),
		logging.NewMiddleware(logger),
	)
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

func (c *Command) getProvider(ctx context.Context) (providerv1.ProviderServiceClient, *grpc.ClientConn, error) {
	logger := logging.Extract(ctx)
	tlsCredentials, err := c.loadTLSCredentials()
	if err != nil {
		return nil, nil, err
	}

	logOpts := []grpclog.Option{
		grpclog.WithLogOnEvents(grpclog.StartCall, grpclog.FinishCall),
	}
	conn, err := grpc.NewClient(
		c.ProviderAddress,
		grpc.WithTransportCredentials(tlsCredentials),
		grpc.WithChainUnaryInterceptor(
			grpclog.UnaryClientInterceptor(interceptorLogger(logger), logOpts...),
		),
		grpc.WithStreamInterceptor(
			grpclog.StreamClientInterceptor(interceptorLogger(logger), logOpts...),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to the provider: %w", err)
	}

	provider := providerv1.NewProviderServiceClient(conn)
	_, err = provider.Ping(ctx, &providerv1.PingRequest{})
	if err != nil {
		return nil, nil, fmt.Errorf("could not ping provider: %w", err)
	}
	return provider, conn, nil
}

func (c *Command) loadTLSCredentials() (credentials.TransportCredentials, error) {
	if c.ProviderInsecure {
		return insecure.NewCredentials(), nil
	}

	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if c.ProviderCACert != "" {
		pemServerCA, err := os.ReadFile(c.ProviderCACert)
		if err != nil {
			return nil, fmt.Errorf("couldn't read CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(pemServerCA) {
			return nil, fmt.Errorf("failed to add server CA certificate")
		}
		config.RootCAs = certPool
	}

	if c.ProviderClientCert != "" {
		clientCert, err := tls.LoadX509KeyPair(c.ProviderClientCert, c.ProviderClientCertKey)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{clientCert}
	}

	return credentials.NewTLS(config), nil
}

func interceptorLogger(l *slog.Logger) grpclog.Logger {
	return grpclog.LoggerFunc(func(ctx context.Context, lvl grpclog.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}
