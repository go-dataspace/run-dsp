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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/internal/constants"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type dspHandlers struct {
	store      statemachine.Archiver
	provider   providerv1.ProviderServiceClient
	reconciler *statemachine.Reconciler
	selfURL    *url.URL
}

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

func returnError(w http.ResponseWriter, status int, e string) {
	errResp := errorString(e)
	returnContent(w, status, errResp)
}

func routeNotImplemented(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	method := req.Method
	returnError(w, http.StatusNotImplemented, fmt.Sprintf("%s %s has not been implemented", method, path))
}

func grpcErrorHandler(err error) CatalogError {
	switch status.Code(err) { //nolint:exhaustive
	case codes.Unauthenticated:
		return catalogError(err.Error(), http.StatusForbidden, "403", "Not authenticated")
	case codes.PermissionDenied:
		return catalogError(err.Error(), http.StatusUnauthorized, "401", "Permission denied")
	case codes.InvalidArgument:
		return catalogError(err.Error(), http.StatusBadRequest, "400", "Invalid argument")
	case codes.NotFound:
		return catalogError(err.Error(), http.StatusNotFound, "404", "Not found")
	default:
		return catalogError(err.Error(), http.StatusInternalServerError, "500", "Internal server error")
	}
}

func dspaceVersionHandler(w http.ResponseWriter, req *http.Request) error {
	vResp := shared.VersionResponse{
		Context: shared.GetDSPContext(),
		ProtocolVersions: []shared.ProtocolVersion{
			{
				Version: constants.DSPVersion,
				Path:    constants.APIPath,
			},
		},
	}
	data, err := shared.ValidateAndMarshal(req.Context(), vResp)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(data))
	return nil
}
