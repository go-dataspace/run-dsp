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
	"net/http"
)

// GetRoutes gets all the dataspace routes.
func GetRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/dspace-version", dspaceVersionHandler)
	// This is an optional proof endpoint for protected datasets.
	mux.HandleFunc("GET /.well-known/dspace-trust", routeNotImplemented)

	// Catalog endpoints
	mux.HandleFunc("POST /catalog/request", catalogRequestHandler)
	mux.HandleFunc("GET /catalog/datasets/{id}", datasetRequestHandler)

	// Contract negotiation endpoints
	mux.HandleFunc("GET /negotiations/{providerPID}", providerContractStateHandler)
	mux.HandleFunc("POST /negotiations/request", providerContractRequestHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/request", providerContractSpecificRequestHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/events", providerContractEventHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/agreement/verification", providerContractVerificationHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/termination", providerContractTerminationHandler)

	// Contract negotiation consumer callbacks
	mux.HandleFunc("POST /negotiations/offers", consumerContractOfferHandler)
	mux.HandleFunc("POST /negotiations/{consumerPID}/offers", consumerContractSpecificOfferHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/agreement", consumerContractAgreementHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/events", consumerContractEventHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/termination", consumerContractTerminationHandler)

	// Transfer process endpoints
	mux.HandleFunc("GET /transfers/{providerPID}", providerTransferProcessHandler)
	mux.HandleFunc("POST /transfers/request", providerTransferRequestHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/start", providerTransferStartHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/completion", providerTransferCompletionHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/termination", providerTransferTerminationHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/suspension", providerTransferSuspensionHandler)
	// Transfer process consumer callbacks
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/start", consumerTransferStartHandler)
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/completion", consumerTransferCompletionHandler)
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/termination", consumerTransferTerminationHandler)
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/suspension", consumerTransferSuspensionHandler)

	mux.HandleFunc("GET /types", returnAllTypes)

	return mux
}
