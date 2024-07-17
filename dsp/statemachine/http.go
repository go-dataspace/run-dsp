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

package statemachine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-dataspace/run-dsp/internal/authforwarder"
	"github.com/go-dataspace/run-dsp/logging"
)

type Requester interface {
	SendHTTPRequest(ctx context.Context, method string, url *url.URL, reqBody []byte) ([]byte, error)
}

type HTTPRequester struct {
	Client *http.Client
}

func (hr *HTTPRequester) setDefaultClient() *http.Client {
	return &http.Client{
		Transport: authforwarder.AuthRoundTripper{Proxied: http.DefaultTransport},
	}
}

func (hr *HTTPRequester) SendHTTPRequest(
	ctx context.Context, method string, url *url.URL, reqBody []byte,
) ([]byte, error) {
	if hr.Client == nil {
		hr.setDefaultClient()
	}
	logger := logging.Extract(ctx).With("method", method, "target_url", url)
	logger.Debug("Doing HTTP request")
	req, err := http.NewRequestWithContext(ctx, method, url.String(), bytes.NewReader(reqBody))
	if err != nil {
		logger.Error("Failed to create request", "error", err)
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	resp, err := hr.Client.Do(req)
	if err != nil {
		logger.Error("Failed to send request", "error", err)
		return nil, err
	}
	defer resp.Body.Close()
	// In the future we might want to return the reader to handle big bodies.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read body", "error", err)
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		logger.Error("Received non-200 status code", "status_code", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("non-200 status code: %d", resp.StatusCode)
	}

	return respBody, nil
}
