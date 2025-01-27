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

// Package downloaddataset offers a command to download a dataset.
package downloaddataset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"

	"github.com/go-dataspace/run-dsp/internal/client/shared"
	"github.com/go-dataspace/run-dsp/internal/ui"
	dspcontrol "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	Command.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Directory to download dataset to")
}

var (
	outputDir string
	Command   = &cobra.Command{
		Use:   "downloaddataset <provider_url> <dataset_id>",
		Short: "Download dataset from dataspace provider.",
		Long: `Uses RUN-DSP instance to get dataset download information of a dataset
and then downloads it into an output directory.`,
		Args: cobra.ExactArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := url.Parse(args[0])
			if err != nil {
				return fmt.Errorf("argument needs to be a valid URL")
			}

			s, err := os.Stat(outputDir)
			if err != nil {
				return fmt.Errorf("could not stat directory %s: %w", outputDir, err)
			}
			if !s.IsDir() {
				return fmt.Errorf("not a directory: %s", outputDir)
			}

			// Quick and dirty portable way to see if we can write in the output directory.
			var errStat error
			var testFile string
			for errStat == nil {
				testFile = path.Join(outputDir, strconv.Itoa(int(rand.Uint64()))) //nolint:gosec
				_, errStat = os.Stat(testFile)
			}
			if !errors.Is(errStat, os.ErrNotExist) {
				return fmt.Errorf("Could not find a test file to write to: %w", errStat)
			}

			err = os.WriteFile(testFile, []byte{1}, 0o600)
			if err != nil {
				return fmt.Errorf("Could not write to file %s: %w", testFile, err)
			}
			err = os.Remove(testFile)
			if err != nil {
				return fmt.Errorf("Could not remove file %s: %w", testFile, err)
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
			resp, err := client.GetProviderDatasetDownloadInformation(
				ctx,
				&dspcontrol.GetProviderDatasetDownloadInformationRequest{
					ProviderUrl: provider,
					DatasetId:   args[1],
				},
			)
			if err != nil {
				return fmt.Errorf("failed to get download information: %w", err)
			}
			defer func() {
				_, err := client.SignalTransferComplete(ctx, &dspcontrol.SignalTransferCompleteRequest{
					TransferId: resp.TransferId,
				})
				if err != nil {
					panic(err.Error())
				}
			}()
			ui.Info("Downloading file")
			location, err := downloadFile(outputDir, resp.PublishInfo)
			if err != nil {
				return fmt.Errorf("could not get dataset %s from %s: %w", args[1], provider, err)
			}
			ui.Info(fmt.Sprintf("%s successfully downloaded to %s", resp.PublishInfo.Url, location))
			return nil
		},
	}
)

func downloadFile(o string, pi *dspcontrol.PublishInfo) (string, error) {
	fName := path.Base(pi.Url)
	fName, err := url.PathUnescape(fName)
	if err != nil {
		return "", fmt.Errorf("couldn't unescape file name %s: %w", fName, err)
	}
	dlPath := path.Join(o, fName)
	fh, err := os.Create(dlPath)
	if err != nil {
		return "", fmt.Errorf("couldn't create file %s for downloading: %w", dlPath, err)
	}
	defer fh.Close()

	c := http.Client{}
	req, err := http.NewRequest(http.MethodGet, pi.Url, nil)
	if err != nil {
		return "", fmt.Errorf("could not create HTTP request: %w", err)
	}
	setAuth(pi, req)

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't fetch file at %s: %w", pi.Url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode <= 199 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("invalid status given when trying to download %s: %d", pi.Url, resp.StatusCode)
	}
	_, err = io.Copy(fh, resp.Body)
	if err != nil {
		return "", fmt.Errorf("download of %s failed, i/o error: %w", pi.Url, err)
	}
	return dlPath, nil
}

func setAuth(pi *dspcontrol.PublishInfo, req *http.Request) {
	switch pi.AuthenticationType {
	case dspcontrol.AuthenticationType_AUTHENTICATION_TYPE_BASIC:
		req.SetBasicAuth(pi.Username, pi.Password)
	case dspcontrol.AuthenticationType_AUTHENTICATION_TYPE_BEARER:
		req.Header.Set("Authorization", "Bearer "+pi.Password)
	case dspcontrol.AuthenticationType_AUTHENTICATION_TYPE_UNSPECIFIED:
		return
	default:
		panic(fmt.Sprintf("unexpected dspv1alpha1.AuthenticationType: %#v", pi.AuthenticationType))
	}
}
