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

	"github.com/go-dataspace/run-dsp/dsp/persistence"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	provider "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
)

// GetRoutes gets all the dataspace routes.
func GetWellKnownRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /dspace-version", WrapHandlerWithError(dspaceVersionHandler))
	// This is an optional proof endpoint for protected datasets.
	mux.HandleFunc("GET /dspace-trust", routeNotImplemented)
	return mux
}

func GetDSPRoutes(
	provider provider.ProviderServiceClient,
	contractService provider.ContractServiceClient,
	store persistence.StorageProvider,
	reconciler statemachine.Reconciler,
	selfURL *url.URL,
	pingResponse *provider.PingResponse,
) http.Handler {
	mux := http.NewServeMux()

	ch := dspHandlers{
		provider:            provider,
		contractService:     contractService,
		store:               store,
		reconciler:          reconciler,
		selfURL:             selfURL,
		dataserviceID:       pingResponse.GetDataserviceId(),
		dataserviceEndpoint: pingResponse.GetDataserviceUrl(),
	}
	// Catalog endpoints
	mux.Handle("POST /catalog/request", WrapHandlerWithError(ch.catalogRequestHandler))
	mux.Handle("GET /catalog/datasets/{id}", WrapHandlerWithError(ch.datasetRequestHandler))

	// Contract negotiation endpoints
	mux.Handle("GET /negotiations/{providerPID}", WrapHandlerWithError(ch.providerContractStateHandler))
	mux.Handle("POST /negotiations/request", WrapHandlerWithError(ch.providerContractRequestHandler))
	mux.Handle("POST /negotiations/{providerPID}/request", WrapHandlerWithError(ch.providerContractSpecificRequestHandler))
	mux.Handle("POST /negotiations/{providerPID}/events", WrapHandlerWithError(ch.providerContractEventHandler))
	mux.Handle("POST /negotiations/{providerPID}/agreement/verification",
		WrapHandlerWithError(ch.providerContractVerificationHandler))
	mux.Handle("POST /negotiations/{PID}/termination", WrapHandlerWithError(ch.contractTerminationHandler))

	// Contract negotiation consumer callbacks)
	mux.Handle("POST /negotiations/offers", WrapHandlerWithError(ch.consumerContractOfferHandler))
	mux.Handle("POST /callback/negotiations/{consumerPID}/offers",
		WrapHandlerWithError(ch.consumerContractSpecificOfferHandler))
	mux.Handle("POST /callback/negotiations/{consumerPID}/agreement",
		WrapHandlerWithError(ch.consumerContractAgreementHandler))
	mux.Handle("POST /callback/negotiations/{consumerPID}/events", WrapHandlerWithError(ch.consumerContractEventHandler))

	mux.Handle("POST /callback/negotiations/{PID}/termination", WrapHandlerWithError(ch.contractTerminationHandler))

	// Transfer process endpoints
	mux.Handle("GET /transfers/{providerPID}", WrapHandlerWithError(ch.providerTransferProcessHandler))
	mux.Handle("POST /transfers/request", WrapHandlerWithError(ch.providerTransferRequestHandler))
	mux.Handle("POST /transfers/{providerPID}/start", WrapHandlerWithError(ch.providerTransferStartHandler))
	mux.Handle("POST /transfers/{providerPID}/completion", WrapHandlerWithError(ch.providerTransferCompletionHandler))
	mux.Handle("POST /transfers/{providerPID}/termination", WrapHandlerWithError(ch.providerTransferTerminationHandler))
	mux.Handle("POST /transfers/{providerPID}/suspension", WrapHandlerWithError(ch.providerTransferSuspensionHandler))
	// Transfer process consumer callbacks
	mux.Handle("POST /callback/transfers/{consumerPID}/start", WrapHandlerWithError(ch.consumerTransferStartHandler))
	mux.Handle("POST /callback/transfers/{consumerPID}/completion",
		WrapHandlerWithError(ch.consumerTransferCompletionHandler))
	mux.Handle("POST /callback/transfers/{consumerPID}/termination",
		WrapHandlerWithError(ch.consumerTransferTerminationHandler))
	mux.Handle("POST /callback/transfers/{consumerPID}/suspension",
		WrapHandlerWithError(ch.consumerTransferSuspensionHandler))

	return mux
}
