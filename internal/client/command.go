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

// Package client contains a client for RUN-DSP, this is the base of all client subcommands.
package client

import (
	"github.com/fatih/color"
	"github.com/go-dataspace/run-dsp/internal/cfg"
	"github.com/go-dataspace/run-dsp/internal/client/downloaddataset"
	"github.com/go-dataspace/run-dsp/internal/client/getcatalog"
	"github.com/go-dataspace/run-dsp/internal/client/getdataset"
	"github.com/go-dataspace/run-dsp/internal/client/shared"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	noColour bool
	Command  = &cobra.Command{
		Use:   "client",
		Short: "Run a RUN-DSP client command.",
		Long:  `Run a RUN-DSP client command.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.CheckConnectAddr(viper.GetString(shared.Address)); err != nil {
				return err
			}

			if !viper.GetBool(shared.InsecureConn) {
				if err := cfg.CheckFilesExist(viper.GetString(shared.CACert)); err != nil {
					return err
				}
				if viper.GetString(shared.ClientCert) != "" && viper.GetString(shared.ClientCertKey) != "" {
					if err := cfg.CheckFilesExist(
						viper.GetString(shared.ClientCert),
						viper.GetString(shared.ClientCertKey),
					); err != nil {
						return err
					}
				}
			}

			if noColour {
				color.NoColor = true
			}

			return nil
		},
	}
)

func init() {
	cfg.AddPersistentFlag(
		Command, shared.Address, "address", "Address of the RUN-DSP gRPC controls service endpoint.", "127.0.0.1:8081")
	cfg.AddPersistentFlag(
		Command, shared.InsecureConn, "insecure", "Disable TLS when connecting.", false)
	cfg.AddPersistentFlag(
		Command, shared.CACert, "ca-cert", "CA certificate of endpoint cert issuer", "")
	cfg.AddPersistentFlag(
		Command, shared.ClientCert, "client-cert", "Client certificate to use with endpoint", "")
	cfg.AddPersistentFlag(
		Command, shared.ClientCertKey, "client-cert-key", "Key for client certificate", "")
	cfg.AddPersistentFlag(
		Command, shared.AuthMD, "authorization-metadata", "Auth metadata to add to gRPC requests.", "")

	Command.Flags().BoolVar(&noColour, "no-colour", false, "Disable colour in output.")
	Command.AddCommand(getcatalog.Command)
	Command.AddCommand(getdataset.Command)
	Command.AddCommand(downloaddataset.Command)
}
