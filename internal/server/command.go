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
	"strings"
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

// Viper keys.
const (
	dspAddress     = "sever.dsp.address"
	dspPort        = "server.dsp.port"
	dspExternalURL = "server.dsp.externalURL"

	providerAddress       = "server.provider.address"
	providerInsecure      = "server.provider.insecure"
	providerCACert        = "server.provider.caCert"
	providerClientCert    = "server.provider.clientCert"
	providerClientCertKey = "server.provider.clientCertKey"

	controlEnabled                  = "server.control.enabled"
	controlAddr                     = "server.control.address"
	controlPort                     = "server.control.port"
	controlInsecure                 = "server.control.insecure"
	controlCert                     = "server.control.cert"
	controlCertKey                  = "server.control.certKey"
	controlVerifyClientCertificates = "server.control.verifyClientCerts"
	controlClientCACert             = "server.control.clientCACert"

	persistenceBackend = "server.persistence.backend"

	persistenceBadgerMemory = "server.persistence.badger.memory"
	persistenceBadgerDBPath = "server.persistence.badger.dbPath"
)

// validStorageBackends are all the persistence backends we support. Right now, it's only badger.
var validStorageBackends = []string{"badger"}

// init initialises all the flags for the command.
//
//nolint:funlen // As all the flags live here, this will get rather long, TODO: split up.
func init() {
	cfg.AddPersistentFlag(
		Command, dspAddress, "dsp-address", "address to listen on for dataspace operations", "0.0.0.0")
	cfg.AddPersistentFlag(
		Command, dspPort, "dsp-port", "port to listen on for dataspace operations", 8080)
	cfg.AddPersistentFlag(
		Command,
		dspExternalURL,
		"external-url",
		"URL that the dataspace service is reachable by from the dataspace",
		"",
	)

	cfg.AddPersistentFlag(
		Command, providerAddress, "provider-address", "Address of the provider gRPC endpoint", "")
	cfg.AddPersistentFlag(
		Command, providerInsecure, "provider-insecure", "Disable TLS when connecting to provider", false)
	cfg.AddPersistentFlag(
		Command, providerCACert, "provider-ca-cert", "CA certificate of provider cert issuer", "")
	cfg.AddPersistentFlag(
		Command, providerClientCert, "provider-client-cert", "Client certificate to use with provider", "")
	cfg.AddPersistentFlag(
		Command, providerClientCertKey, "provider-client-cert-key", "Key for client certificate", "")

	cfg.AddPersistentFlag(Command, controlEnabled, "control-enabled", "enable gRPC control service", false)
	cfg.AddPersistentFlag(
		Command, controlAddr, "control-address", "address for the control service to listen on", "0.0.0.0")
	cfg.AddPersistentFlag(
		Command, controlPort, "control-port", "port for the control service to listen on", 8081)
	cfg.AddPersistentFlag(
		Command, controlInsecure, "control-insecure", "disable TLS for the control service", false)
	cfg.AddPersistentFlag(
		Command, controlCert, "control-cert", "TLS certificate for the control service", "")
	cfg.AddPersistentFlag(
		Command, controlCertKey, "control-cert-key", "Key for control service certificate", "")
	cfg.AddPersistentFlag(
		Command,
		controlVerifyClientCertificates,
		"control-verify-client-certs",
		"Require CA issued client certificates",
		false,
	)
	cfg.AddPersistentFlag(
		Command, controlClientCACert, "control-client-ca-cert", "CA certificate of client cert issuer", "")

	cfg.AddPersistentFlag(
		Command,
		persistenceBackend,
		"persistence-backend",
		fmt.Sprintf(
			"What backend to store state in. Options: %s",
			strings.Join(validStorageBackends, ","),
		),
		"badger",
	)

	cfg.AddPersistentFlag(
		Command,
		persistenceBadgerMemory,
		"badger-in-memory",
		"Put badger database in memory, will not survive restarts",
		false,
	)
	cfg.AddPersistentFlag(
		Command, persistenceBadgerDBPath, "badger-dbpath", "Path to store the badger database", "",
	)
}

// Command validates the configuration and then runs the server.
var Command = &cobra.Command{
	Use:   "server",
	Short: "Start the RUN-DSP server",
	Long: `Starts the RUN-DSP connector, which then connects to the provider and will start
				serving dataspace requests`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := url.Parse(viper.GetString(dspExternalURL))
		if err != nil {
			return fmt.Errorf("Invalid external URL: %w", err)
		}
		err = cfg.CheckListenPort(viper.GetString(dspAddress), viper.GetInt(dspPort))
		if err != nil {
			return err
		}

		err = cfg.CheckConnectAddr(viper.GetString(providerAddress))
		if err != nil {
			return err
		}

		if !viper.GetBool(providerInsecure) {
			err = cfg.CheckFilesExist(
				viper.GetString(providerCACert),
				viper.GetString(providerClientCert),
				viper.GetString(providerClientCertKey),
			)
			if err != nil {
				return err
			}
		}

		if viper.GetBool(controlEnabled) {
			err = cfg.CheckListenPort(viper.GetString(controlAddr), viper.GetInt(controlPort))
			if err != nil {
				return err
			}
			if !viper.GetBool(controlInsecure) {
				err = cfg.CheckFilesExist(
					viper.GetString(controlCert),
					viper.GetString(controlCertKey),
				)
				if err != nil {
					return err
				}
				if viper.GetBool(controlVerifyClientCertificates) {
					err = cfg.CheckFilesExist(viper.GetString(controlClientCACert))
					if err != nil {
						return err
					}
				}
			}

		}

		switch viper.GetString(persistenceBackend) {
		case "badger":
			mem := viper.GetBool(persistenceBadgerMemory)
			path := viper.GetString(persistenceBadgerDBPath)
			if mem && path != "" {
				return fmt.Errorf("in-memory database is mutually exclusive with a database path")
			}
		default:
			return fmt.Errorf("invalid persistence backend")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		u, err := url.Parse(viper.GetString(dspExternalURL))
		if err != nil {
			panic(err.Error())
		}
		c := command{
			ListenAddr:                      viper.GetString(dspAddress),
			Port:                            viper.GetInt(dspPort),
			ExternalURL:                     u,
			ProviderAddress:                 viper.GetString(providerAddress),
			ProviderInsecure:                viper.GetBool(providerInsecure),
			ProviderCACert:                  viper.GetString(providerCACert),
			ProviderClientCert:              viper.GetString(providerClientCert),
			ProviderClientCertKey:           viper.GetString(providerClientCertKey),
			ControlEnabled:                  viper.GetBool(controlEnabled),
			ControlListenAddr:               viper.GetString(controlAddr),
			ControlPort:                     viper.GetInt(controlPort),
			ControlInsecure:                 viper.GetBool(controlInsecure),
			ControlCert:                     viper.GetString(controlCert),
			ControlCertKey:                  viper.GetString(controlCertKey),
			ControlVerifyClientCertificates: viper.GetBool(controlVerifyClientCertificates),
			ControlClientCACert:             viper.GetString(controlClientCACert),
			PersistenceBackend:              viper.GetString(persistenceBackend),
			BadgerMemoryDB:                  viper.GetBool(persistenceBadgerMemory),
			BadgerDBPath:                    viper.GetString(persistenceBadgerDBPath),
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

	// GRPC settings for the provider.
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

	// Persistence settings
	PersistenceBackend string

	// Badger backend settings.
	BadgerMemoryDB bool
	BadgerDBPath   string
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
	store, err := c.getStorageProvider(ctx)
	if err != nil {
		return err
	}

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
