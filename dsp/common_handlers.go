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
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
)

type errorResponse struct {
	Error string `json:"error"`
}

func errorString(e string) string {
	er := errorResponse{Error: e}
	s, err := json.Marshal(er)
	if err != nil {
		panic(fmt.Sprintf("Couldn't marshal error message: %s", err.Error()))
	}
	return string(s)
}

func returnContent(w http.ResponseWriter, status int, content string) {
	w.WriteHeader(status)
	fmt.Fprint(w, content)
}

func validateMarshalAndReturn[T any](ctx context.Context, w http.ResponseWriter, successStatus int, s T) {
	respBody, err := validateAndMarshal(ctx, s)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Could not render response")
		return
	}
	returnContent(w, successStatus, string(respBody))
}

func returnError(w http.ResponseWriter, status int, e string) {
	errResp := errorString(e)
	returnContent(w, status, errResp)
}

func routeNotImplemented(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	method := req.Method
	returnError(w, http.StatusNotImplemented, fmt.Sprintf("%s %s has not been implemented", method, path))
}

func dspaceVersionHandler(w http.ResponseWriter, req *http.Request) {
	vResp := VersionResponse{
		Context: jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		ProtocolVersions: []ProtocolVersion{
			{
				Version: constants.DSPVersion,
				Path:    constants.APIPath,
			},
		},
	}
	data, err := validateAndMarshal(req.Context(), vResp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, errorString("Error while trying to fetch dataspace versions"))
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(data))
}
