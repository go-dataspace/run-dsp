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
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"github.com/go-dataspace/run-dsp/dsp"
	"github.com/go-dataspace/run-dsp/dsp/control"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/internal/authforwarder"
	"github.com/go-dataspace/run-dsp/internal/cli"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
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

	ExternalURL *url.URL `help:"URL that RUN-DSP uses in the dataspace." required:"" env:"EXTERNAL_URL"`

	// GRPC settings for the provider
	ProviderAddress       string `help:"Address of provider GRPC endpoint" required:"" env:"PROVIDER_URL"`
	ProviderInsecure      bool   `help:"Provider connection does not use TLS" default:"false" env:"PROVIDER_INSECURE"`
	ProviderCACert        string `help:"Custom CA certificate for provider's TLS certificate" env:"PROVIDER_CA"`
	ProviderClientCert    string `help:"Client certificate to use to authenticate with provider" env:"PROVIDER_CLIENT_CERT"`
	ProviderClientCertKey string `help:"Key to the client certificate" env:"PROVIDER_CLIENT_CERT_KEY"`

	// GRPC control interface settings.
	ControlEnabled                  bool   `help:"Enable to GRPC control interface" default:"false" env:"CONTROL_ENABLED"`
	ControlListenAddr               string `help:"Listen address for the control interface" default:"0.0.0.0" env:"CONTROL_ADDR"`
	ControlPort                     int    `help:"Port for the control interface" default:"8081" env:"CONTROL_PORT"`
	ControlInsecure                 bool   `help:"Disable TLS for the control interface" default:"false" env:"CONTROL_INSECURE"`
	ControlCert                     string `help:"Certificate to use for the control interface" env:"CONTROL_CERT"`
	ControlCertKey                  string `help:"Key to the control interface" env:"CONTROL_CERT_KEY"`
	ControlVerifyClientCertificates bool   `help:"Require validated client certificates to connect to control interface" default:"false" env:"CONTROL_VERIFY_CLIENT_CERTIFICATES" `
	ControlClientCACert             string `help:"Custom CA certificate to verify client certificates with" env:"CONTROL_PROVIDER_CA"`
}

// Validate checks if the parameters are correct.
func (c *Command) Validate() error {
	if c.ExternalURL == nil {
		u, err := url.Parse(fmt.Sprintf("http://%s:%d", c.ListenAddr, c.Port))
		if err != nil {
			return fmt.Errorf("broken default URL, see your listen address and port")
		}
		c.ExternalURL = u
	}

	if !c.ProviderInsecure {
		err := c.validateTLS()
		if err != nil {
			return err
		}
	}

	if c.ControlEnabled {
		return c.validateControl()
	}
	return nil
}

func (c *Command) validateControl() error {
	if c.ControlInsecure {
		return nil
	}

	certSupplied, err := checkFile(c.ControlCert)
	if err != nil {
		return fmt.Errorf("Control interface certificate: %w", err)
	}
	keySupplied, err := checkFile(c.ControlCertKey)
	if err != nil {
		return fmt.Errorf("Control interface certificate key: %w", err)
	}

	if !certSupplied {
		return fmt.Errorf("Control interface certificate not supplied")
	}

	if !keySupplied {
		return fmt.Errorf("Control interface certificate key not supplied")
	}

	if c.ControlVerifyClientCertificates {
		caSupplied, err := checkFile(c.ControlClientCACert)
		if err != nil {
			return fmt.Errorf("Control interface client CA certificate: %w", err)
		}
		if !caSupplied {
			return fmt.Errorf("Control interface client CA certificate required when using client cert auth")
		}
	}

	return nil
}

func (c *Command) validateTLS() error {
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
	wg := &sync.WaitGroup{}
	logger := logging.Extract(ctx)

	logger.Info("Starting server",
		"listenAddr", c.ListenAddr,
		"port", c.Port,
		"externalURL", c.ExternalURL,
	)

	provider, conn, err := c.getProvider(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	pingResponse, err := provider.Ping(ctx, &providerv1.PingRequest{})
	if err != nil {
		return fmt.Errorf("could not ping provider: %w", err)
	}

	store := statemachine.NewMemoryArchiver()
	httpClient := &shared.HTTPRequester{}
	reconciler := statemachine.NewReconciler(ctx, httpClient, store)
	reconciler.Run()

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

	selfURL := cloneURL(c.ExternalURL)
	selfURL.Path = path.Join(selfURL.Path, constants.APIPath)
	mux.Handle(constants.APIPath+"/", http.StripPrefix(
		constants.APIPath,
		baseMW.Append(
			jsonHeaderMiddleware,
			authforwarder.HTTPMiddleware,
		).Then(dsp.GetDSPRoutes(provider, store, reconciler, selfURL, pingResponse)),
	))

	if c.ControlEnabled {
		ctlSVC := control.New(httpClient, store, reconciler, provider, selfURL)
		err = c.startControl(ctx, wg, ctlSVC)
		if err != nil {
			return err
		}
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", c.ListenAddr, c.Port),
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	return srv.ListenAndServe()
}

func (c *Command) startControl(
	ctx context.Context, wg *sync.WaitGroup,
	controlSVC *control.Server,
) error {
	wg.Add(1)
	ctx, logger := logging.InjectLabels(ctx,
		"control_addr", c.ControlListenAddr,
		"control_port", c.ControlPort,
	)

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d",
		c.ControlListenAddr, c.ControlPort),
	)
	if err != nil {
		return fmt.Errorf("couldn't listen on port")
	}

	tlsCredentials, err := c.loadControlTLSCredentials()
	if err != nil {
		return fmt.Errorf("could not load TLS credentials: %w", err)
	}

	logOpts := []grpclog.Option{
		grpclog.WithLogOnEvents(grpclog.StartCall, grpclog.FinishCall),
	}
	grpcServer := grpc.NewServer(
		grpc.Creds(tlsCredentials),
		grpc.ChainUnaryInterceptor(
			grpclog.UnaryServerInterceptor(interceptorLogger(logger), logOpts...),
			authforwarder.UnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			grpclog.StreamServerInterceptor(interceptorLogger(logger), logOpts...),
			authforwarder.StreamInterceptor,
		),
	)
	providerv1.RegisterClientServiceServer(grpcServer, controlSVC)

	go func() {
		logger.Info("Starting GRPC service")
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("GRPC service exited with error", "error", err)
		}
		logger.Info("GRPC service shutdown.")
	}()

	// Wait until we get the done signal and then shut down the grpc service.
	go func() {
		defer wg.Done()
		<-ctx.Done()
		logger.Info("Shutting down GRPC service.")
		grpcServer.GracefulStop()
	}()
	return nil
}

func (c *Command) loadControlTLSCredentials() (credentials.TransportCredentials, error) {
	if c.ControlInsecure {
		return insecure.NewCredentials(), nil
	}

	serverCert, err := tls.LoadX509KeyPair(c.ControlCert, c.ControlCertKey)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	if c.ControlVerifyClientCertificates {
		pemServerCA, err := os.ReadFile(c.ControlClientCACert)
		if err != nil {
			return nil, fmt.Errorf("couldn't read CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(pemServerCA) {
			return nil, fmt.Errorf("failed to add server CA certificate")
		}
		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.ClientCAs = certPool
	}

	return credentials.NewTLS(config), nil
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
			authforwarder.UnaryClientInterceptor,
		),
		grpc.WithChainStreamInterceptor(
			grpclog.StreamClientInterceptor(interceptorLogger(logger), logOpts...),
			authforwarder.StreamClientInterceptor,
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to the provider: %w", err)
	}

	provider := providerv1.NewProviderServiceClient(conn)
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

func cloneURL(u *url.URL) *url.URL {
	cu, err := url.Parse(u.String())
	if err != nil {
		panic(fmt.Sprintf("failed to clone URL %s: %s", u.String(), err))
	}
	return cu
}
