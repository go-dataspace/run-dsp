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
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/go-dataspace/run-dsp/dsp"
	"github.com/go-dataspace/run-dsp/dsp/control"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/internal/authforwarder"
	"github.com/go-dataspace/run-dsp/internal/cfg"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/justinas/alice"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	sloghttp "github.com/samber/slog-http"
)

// init initialises all the flags for the command.
func init() {
	cfg.AddPersistentFlag(
		Command, "server.dsp.address", "dsp-address", "address to listen on for dataspace operations", "0.0.0.0")
	cfg.AddPersistentFlag(
		Command, "server.dsp.port", "dsp-port", "port to listen on for dataspace operations", 8080)
	cfg.AddPersistentFlag(
		Command,
		"server.dsp.externalURL",
		"external-url",
		"URL that the dataspace service is reachable by from the dataspace",
		"",
	)

	cfg.AddPersistentFlag(
		Command, "server.provider.address", "provider-address", "Address of the provider gRPC endpoint", "")
	cfg.AddPersistentFlag(
		Command, "server.provider.insecure", "provider-insecure", "Disable TLS when connecting to provider", false)
	cfg.AddPersistentFlag(
		Command, "server.provider.caCert", "provider-ca-cert", "CA certificate of provider cert issuer", "")
	cfg.AddPersistentFlag(
		Command, "server.provider.clientCert", "provider-client-cert", "Client certificate to use with provider", "")
	cfg.AddPersistentFlag(
		Command, "server.provider.clientCertKey", "provider-client-cert-key", "Key for client certificate", "")

	cfg.AddPersistentFlag(Command, "server.control.enabled", "control-enabled", "enable gRPC control service", false)
	cfg.AddPersistentFlag(
		Command, "server.control.address", "control-address", "address for the control service to listen on", "0.0.0.0")
	cfg.AddPersistentFlag(
		Command, "server.control.port", "control-port", "port for the control service to listen on", 8081)
	cfg.AddPersistentFlag(
		Command, "server.control.insecure", "control-insecure", "disable TLS for the control service", false)
	cfg.AddPersistentFlag(
		Command, "server.control.cert", "control-cert", "TLS certificate for the control service", "")
	cfg.AddPersistentFlag(
		Command, "server.control.certKey", "control-cert-key", "Key for control service certificate", "")
	cfg.AddPersistentFlag(
		Command,
		"server.control.verifyClientCerts",
		"control-verify-client-certs",
		"Require CA issued client certificates",
		false,
	)
	cfg.AddPersistentFlag(
		Command, "server.control.clientCACert", "control-client-ca-cert", "CA certificate of client cert issuer", "")
}

// Command validates the configuration and then runs the server.
var Command = &cobra.Command{
	Use:   "server",
	Short: "Start the RUN-DSP server",
	Long: `Starts the RUN-DSP connector, which then connects to the provider and will start
				serving dataspace requests`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := url.Parse(viper.GetString("server.dsp.externalURL"))
		if err != nil {
			return fmt.Errorf("Invalid external URL: %w", err)
		}
		err = cfg.CheckListenPort(viper.GetString("server.dsp.address"), viper.GetInt("server.dsp.port"))
		if err != nil {
			return err
		}

		err = cfg.CheckConnectAddr(viper.GetString("server.provider.address"))
		if err != nil {
			return err
		}

		if !viper.GetBool("server.provider.insecure") {
			err = cfg.CheckFilesExist(
				viper.GetString("server.provider.caCert"),
				viper.GetString("server.provider.clientCert"),
				viper.GetString("server.provider.clientCertKey"),
			)
			if err != nil {
				return err
			}
		}

		if viper.GetBool("server.control.enabled") {
			err = cfg.CheckListenPort(viper.GetString("server.control.address"), viper.GetInt("server.control.port"))
			if err != nil {
				return err
			}
			if !viper.GetBool("server.control.insecure") {
				err = cfg.CheckFilesExist(
					viper.GetString("server.control.cert"),
					viper.GetString("server.control.certKey"),
				)
				if err != nil {
					return err
				}
				if viper.GetBool("server.control.verifyClientCerts") {
					err = cfg.CheckFilesExist(viper.GetString("server.control.clientCACert"))
					if err != nil {
						return err
					}
				}
			}

		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		u, err := url.Parse(viper.GetString("server.dsp.externalURL"))
		if err != nil {
			panic(err.Error())
		}
		c := command{
			ListenAddr:                      viper.GetString("server.dsp.address"),
			Port:                            viper.GetInt("server.dsp.port"),
			ExternalURL:                     u,
			ProviderAddress:                 viper.GetString("server.provider.address"),
			ProviderInsecure:                viper.GetBool("server.provider.insecure"),
			ProviderCACert:                  viper.GetString("server.provider.caCert"),
			ProviderClientCert:              viper.GetString("server.provider.clientCert"),
			ProviderClientCertKey:           viper.GetString("server.provider.clientCert"),
			ControlEnabled:                  viper.GetBool("server.control.enabled"),
			ControlListenAddr:               viper.GetString("server.control.address"),
			ControlPort:                     viper.GetInt("server.control.port"),
			ControlInsecure:                 viper.GetBool("server.control.insecure"),
			ControlCert:                     viper.GetString("server.control.cert"),
			ControlCertKey:                  viper.GetString("server.control.certKey"),
			ControlVerifyClientCertificates: viper.GetBool("server.control.verifyClientCerts"),
			ControlClientCACert:             viper.GetString("server.control.clientCACert"),
		}
		ctx, ok := viper.Get("initCTX").(context.Context)
		if !ok {
			return fmt.Errorf("couldn't fetch initial context")
		}
		return c.Run(ctx)
	},
}

type command struct {
	ListenAddr string
	Port       int

	ExternalURL *url.URL

	// GRPC settings for the provider
	ProviderAddress       string
	ProviderInsecure      bool
	ProviderCACert        string
	ProviderClientCert    string
	ProviderClientCertKey string

	// GRPC control interface settings.
	ControlEnabled                  bool
	ControlListenAddr               string
	ControlPort                     int
	ControlInsecure                 bool
	ControlCert                     string
	ControlCertKey                  string
	ControlVerifyClientCertificates bool
	ControlClientCACert             string
}

// Run starts the server.
func (c *command) Run(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	logger := logging.Extract(ctx)
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
	defer cancel()

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
	// TODO: Shut down webserver gracefully and use the reconciler waitgroup.
	return srv.ListenAndServe()
}

func (c *command) startControl(
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
			logging.UnaryServerInterceptor(logger),
			authforwarder.UnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			grpclog.StreamServerInterceptor(interceptorLogger(logger), logOpts...),
			logging.StreamServerInterceptor(logger),
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

func (c *command) loadControlTLSCredentials() (credentials.TransportCredentials, error) {
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

func (c *command) getProvider(ctx context.Context) (providerv1.ProviderServiceClient, *grpc.ClientConn, error) {
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

func (c *command) loadTLSCredentials() (credentials.TransportCredentials, error) {
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
