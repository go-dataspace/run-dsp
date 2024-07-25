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
	"net/url"

	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
)

// GetRoutes gets all the dataspace routes.
func GetWellKnownRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /dspace-version", dspaceVersionHandler)
	// This is an optional proof endpoint for protected datasets.
	mux.HandleFunc("GET /dspace-trust", routeNotImplemented)
	return mux
}

func GetDSPRoutes(
	provider providerv1.ProviderServiceClient,
	store statemachine.Archiver,
	reconciler *statemachine.Reconciler,
	selfURL *url.URL,
) http.Handler {
	mux := http.NewServeMux()

	ch := dspHandlers{provider: provider, store: store, reconciler: reconciler, selfURL: selfURL}
	// Catalog endpoints
	mux.HandleFunc("POST /catalog/request", ch.catalogRequestHandler)
	mux.HandleFunc("GET /catalog/datasets/{id}", ch.datasetRequestHandler)

	// Contract negotiation endpoints
	mux.HandleFunc("GET /negotiations/{providerPID}", ch.providerContractStateHandler)
	mux.HandleFunc("POST /negotiations/request", ch.providerContractRequestHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/request", ch.providerContractSpecificRequestHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/events", ch.providerContractEventHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/agreement/verification", ch.providerContractVerificationHandler)
	mux.HandleFunc("POST /negotiations/{providerPID}/termination", ch.providerContractTerminationHandler)

	// Contract negotiation consumer callbacks
	mux.HandleFunc("POST /negotiations/offers", ch.consumerContractOfferHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/offers", ch.consumerContractSpecificOfferHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/agreement", ch.consumerContractAgreementHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/events", ch.consumerContractEventHandler)
	mux.HandleFunc("POST /callback/negotiations/{consumerPID}/termination", ch.consumerContractTerminationHandler)

	// Transfer process endpoints
	mux.HandleFunc("GET /transfers/{providerPID}", ch.providerTransferProcessHandler)
	mux.HandleFunc("POST /transfers/request", ch.providerTransferRequestHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/start", ch.providerTransferStartHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/completion", ch.providerTransferCompletionHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/termination", ch.providerTransferTerminationHandler)
	mux.HandleFunc("POST /transfers/{providerPID}/suspension", ch.providerTransferSuspensionHandler)
	// Transfer process consumer callbacks
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/start", ch.consumerTransferStartHandler)
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/completion", ch.consumerTransferCompletionHandler)
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/termination", ch.consumerTransferTerminationHandler)
	mux.HandleFunc("POST /callback/transfers/{consumerPID}/suspension", ch.consumerTransferSuspensionHandler)

	mux.HandleFunc("GET /triggerconsumer/{datasetID}", ch.triggerConsumerContractRequestHandler)
	mux.HandleFunc("GET /triggertransfer/{contractProviderPID}", ch.triggerTransferRequestHandler)
	mux.HandleFunc("GET /completetransfer/{providerPID}", ch.completeTransferRequestHandler)

	return mux
}
