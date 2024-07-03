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

package dspstatemachine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-dataspace/run-dsp/internal/authforwarder"
	"github.com/go-dataspace/run-dsp/logging"
)

type httpService struct {
	Context context.Context
}

func (h *httpService) configureRequest(r *http.Request) {
	r.Header.Add("Authorization", authforwarder.ExtractValue(h.Context))
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Accept", "application/json")
}

func (h *httpService) sendPostRequest(ctx context.Context, url string, reqBody []byte) ([]byte, error) {
	logger := logging.Extract(ctx)
	logger.Debug("Going to send POST request", "target_url", url)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	h.configureRequest(req)
	client := &http.Client{}
	resp, err := client.Do(req)
	// NOTE: If the URL is incorrect (in my test missing a / for http://) we get context cancelled message
	//       instead of the real one.
	if err != nil {
		return nil, err
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("Invalid error code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
