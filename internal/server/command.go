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
	"errors"
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

	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/justinas/alice"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp"
	"go-dataspace.eu/run-dsp/dsp/control"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	"go-dataspace.eu/run-dsp/internal/cfg"
	"go-dataspace.eu/run-dsp/internal/constants"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	sloghttp "github.com/samber/slog-http"
)

// Viper keys.
const (
	dspAddress = "server.dsp.address"
	dspPort    = "server.dsp.port"

	otelEnabled     = "server.tracing.enabled"
	otelGRPC        = "server.tracing.grpc"
	otelEndpointUrl = "server.tracing.endpointUrl"
	otelServiceName = "server.tracing.serviceName"

	prometheusEnabled = "server.prometheusEnabled"
	prometheusAddress = "server.prometheusAddress"
	prometheusPort    = "server.prometheusPort"
	dspExternalURL    = "server.dsp.externalURL"

	providerAddress       = "server.provider.address"
	providerInsecure      = "server.provider.insecure"
	providerCACert        = "server.provider.caCert"
	providerClientCert    = "server.provider.clientCert"
	providerClientCertKey = "server.provider.clientCertKey"

	contractServiceEnabled       = "server.contractService.enabled"
	contractServiceAddress       = "server.contractService.address"
	contractServiceInsecure      = "server.contractService.insecure"
	contractServiceCACert        = "server.contractService.caCert"
	contractServiceClientCert    = "server.contractService.clientCert"
	contractServiceClientCertKey = "server.contractService.clientCertKey"

	authnServiceEnabled       = "server.authnService.enabled"
	authnServiceAddress       = "server.authnService.address"
	authnServiceInsecure      = "server.authnService.insecure"
	authnServiceCACert        = "server.authnService.caCert"
	authnServiceClientCert    = "server.authnService.clientCert"
	authnServiceClientCertKey = "server.authnService.clientCertKey"

	controlAddr                     = "server.control.address"
	controlPort                     = "server.control.port"
	controlExternalAddr             = "server.control.externalAddress"
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
		Command, otelEnabled, "tracing-enabled", "enable sending opentelemetry tracing data", false)
	cfg.AddPersistentFlag(
		Command, otelGRPC, "tracing-grpc", "Use grpc to write opentelemetry tracing data", false)
	cfg.AddPersistentFlag(
		Command,
		otelEndpointUrl,
		"tracing-endpoint-url",
		"endpoint URL for opentelemetry receiver",
		"http://localhost:4318/v1/traces")
	cfg.AddPersistentFlag(
		Command,
		otelServiceName,
		"tracing-service-name",
		"Name of the service in opentelemetry.",
		"run-dsp")
	cfg.AddPersistentFlag(
		Command, prometheusAddress, "otel-service-name", "service name for display purposes", "run-dsp")
	cfg.AddPersistentFlag(
		Command, prometheusEnabled, "prometheus-enabled", "enable prometheus metrics", false)
	cfg.AddPersistentFlag(
		Command, prometheusAddress, "prometheus-address", "address to listen on for prometheus metrics", "0.0.0.0")
	cfg.AddPersistentFlag(
		Command, prometheusPort, "prometheus-port", "port to listen on for prometheus metrics", 8082)
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
		Command, contractServiceInsecure, "contract-service-insecure", "Disable TLS for contract service", false)
	cfg.AddPersistentFlag(
		Command, contractServiceCACert, "contract-service-ca-cert", "CA certificate of cert issuer", "")
	cfg.AddPersistentFlag(
		Command, contractServiceClientCert, "contract-service-client-cert", "Client certificate for contract service", "")
	cfg.AddPersistentFlag(
		Command, contractServiceClientCertKey, "contract-service-client-cert-key", "Key for client certificate", "")

	cfg.AddPersistentFlag(
		Command,
		authnServiceEnabled,
		"authn-service-enabled",
		"Connect to a authn service to coordinate authn negotiation.",
		false,
	)
	cfg.AddPersistentFlag(
		Command, authnServiceAddress, "authn-service-address", "Address of the authn service gRPC endpoint", "")
	cfg.AddPersistentFlag(
		Command, authnServiceInsecure, "authn-service-insecure", "Disable TLS when connecting to authn service", false)
	cfg.AddPersistentFlag(
		Command, authnServiceCACert, "authn-service-ca-cert", "CA certificate of cert issuer", "")
	cfg.AddPersistentFlag(
		Command, authnServiceClientCert, "authn-service-client-cert", "Client certificate to use with authn service", "")
	cfg.AddPersistentFlag(
		Command, authnServiceClientCertKey, "authn-service-client-cert-key", "Key for client certificate", "")

	cfg.AddPersistentFlag(
		Command, controlAddr, "control-address", "address for the control service to listen on", "0.0.0.0")
	cfg.AddPersistentFlag(
		Command, controlPort, "control-port", "port for the control service to listen on", 8081)
	cfg.AddPersistentFlag(
		Command, controlExternalAddr,
		"control-external-address", "ip address / DNS name that the control service is reachable by", "0.0.0.0:8081")
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
			return fmt.Errorf("invalid external URL: %w", err)
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
			if viper.GetString(providerCACert) == "" {
				return errors.New("provider service CA cert path is empty")
			}
			if viper.GetString(providerClientCert) == "" {
				return errors.New("provider service cert path is empty")
			}
			if viper.GetString(providerClientCertKey) == "" {
				return errors.New("provider service cert key path is empty")
			}

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
				if viper.GetString(contractServiceClientCert) == "" {
					return errors.New("contract service cert path is empty")
				}
				if viper.GetString(contractServiceClientCertKey) == "" {
					return errors.New("contract service cert key path is empty")
				}
				err = cfg.CheckFilesExist(
					viper.GetString(contractServiceClientCert),
					viper.GetString(contractServiceClientCertKey),
				)
				if err != nil {
					return err
				}
				if viper.GetString(contractServiceCACert) != "" {
					err = cfg.CheckFilesExist(
						viper.GetString(contractServiceCACert),
					)
					if err != nil {
						return err
					}
				}

			}
		}

		err = cfg.CheckListenPort(viper.GetString(controlAddr), viper.GetInt(controlPort))
		if err != nil {
			return err
		}
		if !viper.GetBool(controlInsecure) {
			if viper.GetString(controlCert) == "" {
				return errors.New("control cert path is empty")
			}
			if viper.GetString(controlCertKey) == "" {
				return errors.New("control cert key path is empty")
			}
			err = cfg.CheckFilesExist(
				viper.GetString(controlCert),
				viper.GetString(controlCertKey),
			)
			if err != nil {
				return err
			}
			if viper.GetBool(controlVerifyClientCertificates) {
				if viper.GetString(controlClientCACert) == "" {
					return errors.New("control CA cert path is empty")
				}
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
			ListenAddr:        viper.GetString(dspAddress),
			Port:              viper.GetInt(dspPort),
			OtelEnabled:       viper.GetBool(otelEnabled),
			OtelGRPC:          viper.GetBool(otelGRPC),
			OtelEndpointUrl:   viper.GetString(otelEndpointUrl),
			OtelServiceName:   viper.GetString(otelServiceName),
			PrometheusEnabled: viper.GetBool(prometheusEnabled),
			PrometheusAddr:    viper.GetString(prometheusAddress),
			PrometheusPort:    viper.GetInt(prometheusPort),
			ExternalURL:       u,
			ProviderAddress:   viper.GetString(providerAddress),
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
			AuthNServiceEnabled: viper.GetBool(authnServiceEnabled),
			AuthNServiceAddress: viper.GetString(authnServiceAddress),
			AuthNServiceTLSConfig: tlsConfig{
				Insecure:      viper.GetBool(authnServiceInsecure),
				CACert:        viper.GetString(authnServiceCACert),
				ClientCert:    viper.GetString(authnServiceClientCert),
				ClientCertKey: viper.GetString(authnServiceClientCertKey),
			},
			ControlListenAddr:               viper.GetString(controlAddr),
			ControlPort:                     viper.GetInt(controlPort),
			ControlExternalAddr:             viper.GetString(controlExternalAddr),
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
	ListenAddr        string
	Port              int
	OtelEnabled       bool
	OtelGRPC          bool
	OtelEndpointUrl   string
	OtelServiceName   string
	PrometheusEnabled bool
	PrometheusAddr    string
	PrometheusPort    int

	ExternalURL *url.URL

	// GRPC settings for the provider.
	ProviderAddress   string
	ProviderTLSConfig tlsConfig

	// GRPC settings for the contract service.
	ContractServiceEnabled   bool
	ContractServiceAddress   string
	ContractServiceTLSConfig tlsConfig

	// GRPC settings for the authn service.
	AuthNServiceEnabled   bool
	AuthNServiceAddress   string
	AuthNServiceTLSConfig tlsConfig

	// GRPC control interface settings.
	ControlListenAddr               string
	ControlPort                     int
	ControlExternalAddr             string
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
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
	defer cancel()
	ctxslog.Info(ctx, "Starting server",
		"listenAddr", c.ListenAddr,
		"port", c.Port,
		"externalURL", c.ExternalURL,
		"prometheusAddr", c.PrometheusAddr,
		"prometheusPort", c.PrometheusPort,
	)

	// Set up OpenTelemetry.
	otelShutdown, err := setupOTelSDK(ctx, c.OtelEnabled, c.OtelGRPC, c.OtelEndpointUrl, c.OtelServiceName)
	if err != nil {
		return nil
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	reconciler, httpServices, grpcConnections, err := c.setupServices(ctx, wg)
	if err != nil {
		return err
	}

	for _, s := range httpServices {
		go func() {
			defer wg.Done()
			err = s.ListenAndServe()
			if err != nil {
				ctxslog.Error(ctx, "error from http service", err)
			}
		}()
	}

	for _, conn := range grpcConnections {
		defer func() { _ = conn.Close() }()
	}

	wg.Add(len(httpServices))
	reconciler.WaitGroup.Wait()
	for _, s := range httpServices {
		err = s.Shutdown(ctx)
		if err != nil {
			return err
		}
	}
	wg.Wait()
	return err
}

func (c *command) setupServices(ctx context.Context, //nolint:cyclop
	wg *sync.WaitGroup,
) (*statemachine.HTTPReconciler, []*http.Server, []*grpc.ClientConn, error) {
	srvMetrics, clMetrics := setupMetrics()
	grpcConnections := []*grpc.ClientConn{}
	provider, conn, pingResponse, err := c.getProvider(ctx, clMetrics)
	if err != nil {
		return nil, []*http.Server{}, grpcConnections, err
	}
	grpcConnections = append(grpcConnections, conn)
	store, err := c.getStorageProvider(ctx)
	if err != nil {
		return nil, []*http.Server{}, grpcConnections, err
	}
	authnService, authnConn, err := c.getAuthNService(ctx, clMetrics)
	if err != nil {
		return nil, []*http.Server{}, grpcConnections, err
	}
	if authnConn != nil {
		grpcConnections = append(grpcConnections, authnConn)
	}
	httpClient := &shared.HTTPRequester{
		Client:       nil,
		AuthNService: authnService,
	}
	reconciler := statemachine.NewReconciler(ctx, httpClient, store)
	reconciler.Run()
	selfURL := cloneURL(c.ExternalURL)
	selfURL.Path = path.Join(selfURL.Path, constants.APIPath)
	contractService, csConn, err := c.getContractService(ctx, clMetrics)
	if err != nil {
		return nil, []*http.Server{}, grpcConnections, err
	}
	if csConn != nil {
		grpcConnections = append(grpcConnections, csConn)
	}

	ctlSVC := control.New(httpClient, store, reconciler, provider, contractService, selfURL)
	err = c.startControl(ctx, wg, ctlSVC, srvMetrics)
	if err != nil {
		return nil, []*http.Server{}, grpcConnections, err
	}

	if contractService != nil {
		err = c.configureContractService(ctx, store, contractService)
		if err != nil {
			return nil, []*http.Server{}, grpcConnections, err
		}
	}

	// Setup HTTP services
	httpServices := []*http.Server{
		c.getDataspaceServer(ctx, authnService, provider, contractService, store, reconciler, selfURL, pingResponse),
	}

	if c.PrometheusEnabled {
		httpServices = append(httpServices, c.getMetricsServer(ctx))
	}
	return reconciler, httpServices, grpcConnections, nil
}

func (c *command) getMetricsServer(ctx context.Context) *http.Server {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.Handler())
	ctxslog.Info(ctx, "configuring metrics HTTP service")
	return &http.Server{
		Addr:              fmt.Sprintf("%s:%d", c.PrometheusAddr, c.PrometheusPort),
		ReadHeaderTimeout: 2 * time.Second,
		Handler:           metricsMux,
	}
}

func (c *command) getDataspaceServer(ctx context.Context,
	authnService dsrpc.AuthNServiceClient,
	provider dsrpc.ProviderServiceClient,
	contractService dsrpc.ContractServiceClient,
	store persistence.StorageProvider,
	reconciler *statemachine.HTTPReconciler,
	selfURL *url.URL,
	pingResponse *dsrpc.PingResponse,
) *http.Server {
	mux := http.NewServeMux()
	baseMW := alice.New(
		sloghttp.Recovery,
		sloghttp.New(ctxslog.Extract(ctx).With("component", "dspRequests")),
		ctxslog.NewMiddleware(ctxslog.Extract(ctx).With("component", "dspContext"), true),
	)

	mux.Handle("/.well-known/", http.StripPrefix(
		"/.well-known",
		baseMW.Append(jsonHeaderMiddleware).Then(dsp.GetWellKnownRoutes()),
	))
	mux.Handle(constants.APIPath+"/", http.StripPrefix(
		constants.APIPath,
		baseMW.Append(
			jsonHeaderMiddleware,
			authforwarder.HTTPMiddleware,
			authforwarder.NewAuthNMiddleware(authnService)).Then(
			dsp.GetDSPRoutes(authnService, provider, contractService, store, reconciler, selfURL, pingResponse)),
	))

	// 	Add HTTP instrumentation for the whole server.
	handler := otelhttp.NewHandler(mux, "/")

	ctxslog.Info(ctx, "configuring main HTTP service")
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", c.ListenAddr, c.Port),
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
	}
	return srv
}

func setupMetrics() (*grpcprom.ServerMetrics, *grpcprom.ClientMetrics) {
	// Setup metrics. copy-pasted from example at:
	//  github.com/grpc-ecosystem/go-grpc-middleware/blob/main/examples/server/main.go
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	clMetrics := grpcprom.NewClientMetrics(
		grpcprom.WithClientHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)

	prometheus.MustRegister(srvMetrics)
	prometheus.MustRegister(clMetrics)
	return srvMetrics, clMetrics
}

func (c *command) configureContractService(
	ctx context.Context,
	store persistence.StorageProvider,
	contractService dsrpc.ContractServiceClient,
) error {
	token, err := createToken(ctx, store)
	if err != nil {
		return err
	}
	_, err = contractService.Configure(ctx, &dsrpc.ContractServiceConfigureRequest{
		ConnectorAddress:  c.ControlExternalAddr,
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
	srvMetrics *grpcprom.ServerMetrics,
) error {
	wg.Add(1)
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d",
		c.ControlListenAddr, c.ControlPort),
	)
	if err != nil {
		return fmt.Errorf("control: couldn't listen on port: %s:%d", c.ControlListenAddr, c.ControlPort)
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
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			srvMetrics.UnaryServerInterceptor(),
			grpclog.UnaryServerInterceptor(
				interceptorLogger(ctxslog.Extract(ctx).With("component", "controlRequests")),
				logOpts...),
			ctxslog.UnaryServerInterceptor(ctxslog.Extract(ctx).With("component", "controlContext"), true),
			authforwarder.UnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			srvMetrics.StreamServerInterceptor(),
			grpclog.StreamServerInterceptor(
				interceptorLogger(ctxslog.Extract(ctx).With("component", "controlRequests")),
				logOpts...),
			ctxslog.StreamServerInterceptor(ctxslog.Extract(ctx).With("component", "controlContext"), true),
			authforwarder.StreamInterceptor,
		),
	)
	dsrpc.RegisterControlServiceServer(grpcServer, controlSVC)

	go func() {
		ctxslog.Info(ctx, "Starting control service", "address", c.ControlListenAddr, "port", c.ControlPort)
		if err := grpcServer.Serve(lis); err != nil {
			_ = ctxslog.ReturnError(ctx, "Control service exited with error", err)
		}
		ctxslog.Info(ctx, "Control service sutdown")
	}()

	// Wait until we get the done signal and then shut down the grpc service.
	go func() {
		defer wg.Done()
		<-ctx.Done()
		ctxslog.Info(ctx, "Shutting down GRPC service.")
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

func (c *command) getProvider(ctx context.Context, clMetrics *grpcprom.ClientMetrics) (
	dsrpc.ProviderServiceClient,
	*grpc.ClientConn,
	*dsrpc.PingResponse,
	error,
) {
	client, conn, err := getGRPCClient(ctx,
		clMetrics,
		c.ProviderAddress,
		c.ProviderTLSConfig,
		dsrpc.NewProviderServiceClient)
	if err != nil {
		return nil, nil, nil, err
	}
	ping, err := client.Ping(ctx, &dsrpc.PingRequest{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not ping provider: %w", err)
	}
	return client, conn, ping, nil
}

func (c *command) getContractService(ctx context.Context,
	clMetrics *grpcprom.ClientMetrics,
) (dsrpc.ContractServiceClient, *grpc.ClientConn, error) {
	if c.ContractServiceEnabled {
		return getGRPCClient(ctx,
			clMetrics,
			c.ContractServiceAddress,
			c.ContractServiceTLSConfig,
			dsrpc.NewContractServiceClient)
	}
	return nil, nil, nil
}

func (c *command) getAuthNService(ctx context.Context,
	clMetrics *grpcprom.ClientMetrics,
) (dsrpc.AuthNServiceClient, *grpc.ClientConn, error) {
	if c.AuthNServiceEnabled {
		return getGRPCClient(ctx, clMetrics, c.AuthNServiceAddress, c.AuthNServiceTLSConfig, dsrpc.NewAuthNServiceClient)
	}
	return nil, nil, nil
}

func getGRPCClient[T any](
	ctx context.Context,
	clMetrics *grpcprom.ClientMetrics,
	address string,
	tlsc tlsConfig,
	clientFunc func(cc grpc.ClientConnInterface) T,
) (T, *grpc.ClientConn, error) {
	var client T
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
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(
			clMetrics.UnaryClientInterceptor(),
			grpclog.UnaryClientInterceptor(
				interceptorLogger(ctxslog.Extract(ctx).With("component", fmt.Sprintf("%TClient", client))),
				logOpts...),
			authforwarder.UnaryClientInterceptor,
		),
		grpc.WithChainStreamInterceptor(
			clMetrics.StreamClientInterceptor(),
			grpclog.StreamClientInterceptor(
				interceptorLogger(ctxslog.Extract(ctx).With("component", fmt.Sprintf("%TClient", client))),
				logOpts...),
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
