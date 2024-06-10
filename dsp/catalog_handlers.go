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

package dsp

import (
	"io"
	"net/http"

	"github.com/go-dataspace/run-dsp/logging"
)

func catalogRequestHandler(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	logger := logging.Extract(req.Context())
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	catalogReq, err := unmarshalAndValidate(req.Context(), body, CatalogRequestMessage{})
	if err != nil {
		logger.Error("Non validating catalog request", "error", err)
		returnError(w, http.StatusBadRequest, "Request did not validate")
	}
	logger.Debug("Got catalog request", "req", catalogReq)
	// TODO: Actually get stuff to show here
}
