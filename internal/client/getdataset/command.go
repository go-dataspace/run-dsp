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

// Package getdataset offers a command to get the information of specific dataset.
package getdataset

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go-dataspace.eu/run-dsp/internal/client/shared"
	"go-dataspace.eu/run-dsp/internal/ui"
	dspcontrol "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

func init() {
	Command.Flags().BoolVarP(&printJSON, "json", "j", false, "output catalog in JSON format")
}

var (
	printJSON bool
	Command   = &cobra.Command{
		Use:   "getdataset <provider_url> <dataset_id>",
		Short: "Get dataset info from dataspace provider.",
		Long:  "Uses RUN-DSP instance to get dataset information of a dataspace provider",
		Args:  cobra.ExactArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := url.Parse(args[0])
			if err != nil {
				return fmt.Errorf("argument needs to be a valid URL")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, ok := viper.Get("initCTX").(context.Context)
			if !ok {
				return fmt.Errorf("couldn't fetch initial context")
			}

			provider := args[0]

			client, conn, err := shared.GetRUNDSPClient()
			if err != nil {
				return fmt.Errorf("couldn't initialise gRPC client: %w", err)
			}
			defer conn.Close()

			ui.Info(fmt.Sprintf("Fetching dataset %s from %s", args[1], provider))
			resp, err := client.GetProviderDataset(ctx, &dspcontrol.GetProviderDatasetRequest{
				ProviderUrl: provider,
				DatasetId:   args[1],
			})
			if err != nil {
				return fmt.Errorf("could not get dataset %s from %s: %w", args[1], provider, err)
			}
			ui.Info("Catalogue received")
			return shared.PrintDataset(resp.Dataset, printJSON)
		},
	}
)
