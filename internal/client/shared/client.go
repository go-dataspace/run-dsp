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

package shared

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/go-dataspace/run-dsp/internal/client/authinjector"
	dspcontrol "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// GetRUNDSPClient returns a configured RUNDSP client and its connection.
func GetRUNDSPClient() (dspcontrol.ControlServiceClient, *grpc.ClientConn, error) {
	tlsCredentials, err := loadTLSCredentials()
	if err != nil {
		return nil, nil, err
	}

	conn, err := grpc.NewClient(
		viper.GetString(Address),
		grpc.WithTransportCredentials(tlsCredentials),
		grpc.WithChainUnaryInterceptor(
			authinjector.InjectUnaryAuthInterceptor(viper.GetString(AuthMD)),
		),
		grpc.WithChainStreamInterceptor(
			authinjector.InjectStreamAuthInterceptor(viper.GetString(AuthMD)),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to endpoint: %w", err)
	}

	client := dspcontrol.NewControlServiceClient(conn)
	return client, conn, nil
}

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	if viper.GetBool(InsecureConn) {
		return insecure.NewCredentials(), nil
	}

	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cert := viper.GetString(CACert); cert != "" {
		pemServerCA, err := os.ReadFile(cert)
		if err != nil {
			return nil, fmt.Errorf("couldn't read CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(pemServerCA) {
			return nil, fmt.Errorf("failed tp add server CA certificate")
		}
		config.RootCAs = certPool
	}

	if viper.GetString(ClientCert) != "" && viper.GetString(ClientCertKey) != "" {
		cert, err := tls.LoadX509KeyPair(viper.GetString(ClientCert), viper.GetString(ClientCertKey))
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(config), nil
}
