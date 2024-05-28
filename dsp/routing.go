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

// Package dsp manages the dataspace protocol.
package dsp

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func routeNotImplemented(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	path := req.URL.Path
	method := req.Method
	fmt.Fprint(w, errorString(fmt.Sprintf("%s %s has not been implemented", method, path)))
}

// GetRoutes gets all the dataspace routes.
func GetRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/dspace-version", routeNotImplemented)
	// This is an optional proof endpoint for protected datasets.
	mux.HandleFunc("GET /.well-known/dspace-trust", routeNotImplemented)

	// Catalog endpoints
	mux.HandleFunc("POST /catalog/request", routeNotImplemented)
	mux.HandleFunc("GET /catalog/datasets/{id}", routeNotImplemented)

	// Contract negotiation endpoints
	mux.HandleFunc("GET /negotiations/{providerPID}", routeNotImplemented)
	mux.HandleFunc("POST /negotiations/request", routeNotImplemented)
	mux.HandleFunc("POST /negotiations/{providerPID}/request", routeNotImplemented)
	mux.HandleFunc("POST /negotiations/{providerPID}/events", routeNotImplemented)
	mux.HandleFunc("POST /negotiations/{providerPID}/agreement/verification", routeNotImplemented)
	mux.HandleFunc("POST /negotiations/{providerPID}/termination", routeNotImplemented)

	// Contract negotiation consumer callbacks
	mux.HandleFunc("POST /negotiations/offers", routeNotImplemented)
	mux.HandleFunc("POST /{callback}/negotiations/{consumerPID}/agreement", routeNotImplemented)
	mux.HandleFunc("POST /{callback}/negotiations/{consumerPID}/events", routeNotImplemented)
	mux.HandleFunc("POST /{callback}/negotiations/{consumerPID}/termination", routeNotImplemented)

	// Transfer process endpoints
	mux.HandleFunc("GET /transfers/{providerPID}", routeNotImplemented)
	mux.HandleFunc("POST /transfers/request", routeNotImplemented)
	mux.HandleFunc("POST /transfers/{providerPID}/start", routeNotImplemented)
	mux.HandleFunc("POST /transfers/{providerPID}/completion", routeNotImplemented)
	mux.HandleFunc("POST /transfers/{providerPID}/termination", routeNotImplemented)
	mux.HandleFunc("POST /transfers/{providerPID}/suspension", routeNotImplemented)
	// Transfer process consumer callbacks
	mux.HandleFunc("POST /{callback}/transfers/{consumerPID}/start", routeNotImplemented)
	mux.HandleFunc("POST /{callback}/transfers/{consumerPID}/completion", routeNotImplemented)
	mux.HandleFunc("POST /{callback}/transfers/{consumerPID}/termination", routeNotImplemented)
	mux.HandleFunc("POST /{callback}/transfers/{consumerPID}/suspension", routeNotImplemented)

	return mux
}
