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
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
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
	"github.com/go-dataspace/run-dsp/dsp/persistence"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/internal/authforwarder"
	"github.com/go-dataspace/run-dsp/internal/cfg"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
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

	contractServiceEnabled       = "server.contrctService.enabled"
	contractServiceAddress       = "server.contractService.address"
	contractServiceInsecure      = "server.contractService.insecure"
	contractServiceCACert        = "server.contractService.caCert"
	contractServiceClientCert    = "server.contractService.clientCert"
	contractServiceClientCertKey = "server.contractService.clientCertKey"

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

	cfg.AddPersistentFlag(
		Command,
		contractServiceEnabled,
		"contract-service-enabled",
		"Connect to a contract service to coordinate contract negotiation.",
		false,
	)
	cfg.AddPersistentFlag(
		Command, contractServiceAddress, "contract-service-address", "Address of the contract service gRPC endpoint", "")
	cfg.AddPersistentFlag(
		Command, contractServiceInsecure, "contract-service-insecure", "Disable TLS when connecting to provider", false)
	cfg.AddPersistentFlag(
		Command, contractServiceCACert, "contract-service-ca-cert", "CA certificate of provider cert issuer", "")
	cfg.AddPersistentFlag(
		Command, contractServiceClientCert, "contract-service-client-cert", "Client certificate to use with provider", "")
	cfg.AddPersistentFlag(
		Command, contractServiceClientCertKey, "contract-service-client-cert-key", "Key for client certificate", "")

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
		true,
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

		if viper.GetBool(contractServiceEnabled) {
			err = cfg.CheckConnectAddr(viper.GetString(contractServiceAddress))
			if err != nil {
				return err
			}

			if !viper.GetBool(contractServiceInsecure) {
				err = cfg.CheckFilesExist(
					viper.GetString(contractServiceCACert),
					viper.GetString(contractServiceClientCert),
					viper.GetString(contractServiceClientCertKey),
				)
				if err != nil {
					return err
				}
			}
		}

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
			ListenAddr:      viper.GetString(dspAddress),
			Port:            viper.GetInt(dspPort),
			ExternalURL:     u,
			ProviderAddress: viper.GetString(providerAddress),
			ProviderTLSConfig: tlsConfig{
				Insecure:      viper.GetBool(providerInsecure),
				CACert:        viper.GetString(providerCACert),
				ClientCert:    viper.GetString(providerClientCert),
				ClientCertKey: viper.GetString(providerClientCertKey),
			},
			ContractServiceEnabled: viper.GetBool(contractServiceEnabled),
			ContractServiceAddress: viper.GetString(contractServiceAddress),
			ContractServiceTLSConfig: tlsConfig{
				Insecure:      viper.GetBool(contractServiceInsecure),
				CACert:        viper.GetString(contractServiceCACert),
				ClientCert:    viper.GetString(contractServiceClientCert),
				ClientCertKey: viper.GetString(contractServiceClientCertKey),
			},
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

type tlsConfig struct {
	Insecure      bool
	CACert        string
	ClientCert    string
	ClientCertKey string
}
type command struct {
	ListenAddr string
	Port       int

	ExternalURL *url.URL

	// GRPC settings for the provider.
	ProviderAddress   string
	ProviderTLSConfig tlsConfig

	// GRPC settings for the contract service.
	ContractServiceEnabled   bool
	ContractServiceAddress   string
	ContractServiceTLSConfig tlsConfig

	// GRPC control interface settings.
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
	provider, conn, pingResponse, err := c.getProvider(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	store, err := c.getStorageProvider(ctx)
	if err != nil {
		return err
	}
	httpClient := &shared.HTTPRequester{}
	reconciler := statemachine.NewReconciler(ctx, httpClient, store)
	reconciler.Run()
	selfURL := cloneURL(c.ExternalURL)
	selfURL.Path = path.Join(selfURL.Path, constants.APIPath)
	contractService, csConn, err := c.getContractService(ctx)
	if err != nil {
		return err
	}
	if csConn != nil {
		defer csConn.Close()
	}
	ctlSVC := control.New(httpClient, store, reconciler, provider, contractService, selfURL)
	err = c.startControl(ctx, wg, ctlSVC)
	if err != nil {
		return err
	}
	if contractService != nil {
		err = c.configureContractService(ctx, store, contractService)
		if err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	baseMW := alice.New(sloghttp.Recovery, sloghttp.New(logger), logging.NewMiddleware(logger))
	mux.Handle("/.well-known/", http.StripPrefix(
		"/.well-known",
		baseMW.Append(jsonHeaderMiddleware).Then(dsp.GetWellKnownRoutes()),
	))
	mux.Handle(constants.APIPath+"/", http.StripPrefix(
		constants.APIPath,
		baseMW.Append(jsonHeaderMiddleware, authforwarder.HTTPMiddleware).Then(
			dsp.GetDSPRoutes(provider, contractService, store, reconciler, selfURL, pingResponse)),
	))
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", c.ListenAddr, c.Port),
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	// TODO: Shut down webserver gracefully and use the reconciler waitgroup.
	return srv.ListenAndServe()
}

func (c *command) configureContractService(
	ctx context.Context,
	store persistence.StorageProvider,
	contractService providerv1.ContractServiceClient,
) error {
	token, err := createToken(ctx, store)
	if err != nil {
		return err
	}
	_, err = contractService.Configure(ctx, &providerv1.ContractServiceConfigureRequest{
		ConnectorAddress:  fmt.Sprintf("%s:%d", c.ControlListenAddr, c.ControlPort),
		VerificationToken: token,
	})
	if err != nil {
		return err
	}
	return nil
}

func createToken(ctx context.Context, store persistence.StorageProvider) (string, error) {
	token := generateToken()
	err := store.PutToken(ctx, "contract-token", token)
	if err != nil {
		return "", err
	}
	return token, nil
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
	providerv1.RegisterControlServiceServer(grpcServer, controlSVC)

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

func (c *command) getProvider(ctx context.Context) (
	providerv1.ProviderServiceClient,
	*grpc.ClientConn,
	*providerv1.PingResponse,
	error,
) {
	client, conn, err := getGRPCClient(ctx, c.ProviderAddress, c.ProviderTLSConfig, providerv1.NewProviderServiceClient)
	if err != nil {
		return nil, nil, nil, err
	}
	ping, err := client.Ping(ctx, &providerv1.PingRequest{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not ping provider: %w", err)
	}
	return client, conn, ping, nil
}

func (c *command) getContractService(ctx context.Context) (providerv1.ContractServiceClient, *grpc.ClientConn, error) {
	if c.ContractServiceEnabled {
		return getGRPCClient(ctx, c.ContractServiceAddress, c.ContractServiceTLSConfig, providerv1.NewContractServiceClient)
	}
	return nil, nil, nil
}

func getGRPCClient[T any](
	ctx context.Context,
	address string,
	tlsc tlsConfig,
	clientFunc func(cc grpc.ClientConnInterface) T,
) (T, *grpc.ClientConn, error) {
	var client T
	logger := logging.Extract(ctx)
	tlsCredentials, err := loadTLSCredentials(tlsc)
	if err != nil {
		return client, nil, err
	}

	logOpts := []grpclog.Option{
		grpclog.WithLogOnEvents(grpclog.StartCall, grpclog.FinishCall),
	}
	conn, err := grpc.NewClient(
		address,
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
		return client, nil, fmt.Errorf("could not connect to the provider: %w", err)
	}

	client = clientFunc(conn)
	return client, conn, nil
}

func loadTLSCredentials(t tlsConfig) (credentials.TransportCredentials, error) {
	if t.Insecure {
		return insecure.NewCredentials(), nil
	}

	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if t.CACert != "" {
		pemServerCA, err := os.ReadFile(t.CACert)
		if err != nil {
			return nil, fmt.Errorf("couldn't read CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(pemServerCA) {
			return nil, fmt.Errorf("failed to add server CA certificate")
		}
		config.RootCAs = certPool
	}

	if t.ClientCert != "" {
		clientCert, err := tls.LoadX509KeyPair(t.ClientCert, t.ClientCertKey)
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

func generateToken() string {
	b := make([]byte, 256)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
