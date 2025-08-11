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

	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// GetRoutes gets all the dataspace routes.
func GetWellKnownRoutes() http.Handler {
	mux := http.NewServeMux()

	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	handleFunc("GET /dspace-version", WrapHandlerWithMetrics("dspace-version", WrapHandlerWithError(dspaceVersionHandler)))
	// This is an optional proof endpoint for protected datasets.
	handleFunc("GET /dspace-trust", routeNotImplemented)
	return mux
}

func GetDSPRoutes(
	authNService dsrpc.AuthNServiceClient,
	provider dsrpc.ProviderServiceClient,
	contractService dsrpc.ContractServiceClient,
	store persistence.StorageProvider,
	reconciler statemachine.Reconciler,
	selfURL *url.URL,
	pingResponse *dsrpc.PingResponse,
) http.Handler {
	mux := http.NewServeMux()
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	ch := dspHandlers{
		authNService:        authNService,
		provider:            provider,
		contractService:     contractService,
		store:               store,
		reconciler:          reconciler,
		selfURL:             selfURL,
		dataserviceID:       pingResponse.GetDataserviceId(),
		dataserviceEndpoint: pingResponse.GetDataserviceUrl(),
	}
	// Catalog endpoints
	setupCatalogEndpoints(handleFunc, ch)

	// Contract endpoints
	setupContractEndpoints(handleFunc, ch)

	// Transfer endpoints
	setupTransferEndpoints(handleFunc, ch)

	return mux
}

func setupTransferEndpoints(handleFunc func(
	pattern string, handlerFunc func(http.ResponseWriter, *http.Request)), ch dspHandlers,
) {
	// Transfer process endpoints
	handleFunc("GET /transfers/{providerPID}", WrapHandlerWithMetrics(
		"transfers_ongoing", WrapHandlerWithError(ch.providerTransferProcessHandler)))
	handleFunc("POST /transfers/request", WrapHandlerWithMetrics(
		"transfers_request", WrapHandlerWithError(ch.providerTransferRequestHandler)))
	handleFunc("POST /transfers/{providerPID}/start", WrapHandlerWithMetrics(
		"transfers_ongoing_start", WrapHandlerWithError(ch.providerTransferStartHandler)))
	handleFunc("POST /transfers/{providerPID}/completion", WrapHandlerWithMetrics(
		"transfers_ongoing_completion", WrapHandlerWithError(ch.providerTransferCompletionHandler)))
	handleFunc("POST /transfers/{providerPID}/termination", WrapHandlerWithMetrics(
		"transfers_ongoing_termination", WrapHandlerWithError(ch.providerTransferTerminationHandler)))
	handleFunc("POST /transfers/{providerPID}/suspension", WrapHandlerWithMetrics(
		"transfers_ongoing_suspension", WrapHandlerWithError(ch.providerTransferSuspensionHandler)))
	// Transfer process consumer callbacks
	handleFunc("POST /callback/transfers/{consumerPID}/start", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_start", WrapHandlerWithError(ch.consumerTransferStartHandler)))
	handleFunc("POST /callback/transfers/{consumerPID}/completion", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_completion", WrapHandlerWithError(ch.consumerTransferCompletionHandler)))
	handleFunc("POST /callback/transfers/{consumerPID}/termination", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_termination", WrapHandlerWithError(ch.consumerTransferTerminationHandler)))
	handleFunc("POST /callback/transfers/{consumerPID}/suspension", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_suspension", WrapHandlerWithError(ch.consumerTransferSuspensionHandler)))
}

func setupContractEndpoints(handleFunc func(
	pattern string, handlerFunc func(http.ResponseWriter, *http.Request)), ch dspHandlers,
) {
	// Contract negotiation endpoints
	handleFunc("GET /negotiations/{providerPID}", WrapHandlerWithMetrics(
		"negotiations", WrapHandlerWithError(ch.providerContractStateHandler)))
	handleFunc("POST /negotiations/request", WrapHandlerWithMetrics(
		"negotiations_request", WrapHandlerWithError(ch.providerContractRequestHandler)))
	handleFunc("POST /negotiations/{providerPID}/request", WrapHandlerWithMetrics(
		"negotiations_ongoing_request", WrapHandlerWithError(ch.providerContractSpecificRequestHandler)))
	handleFunc("POST /negotiations/{providerPID}/events", WrapHandlerWithMetrics(
		"negotiations_ongoing_events", WrapHandlerWithError(ch.providerContractEventHandler)))
	handleFunc("POST /negotiations/{providerPID}/agreement/verification", WrapHandlerWithMetrics(
		"negotiations_ongoing_agreement_verification", WrapHandlerWithError(ch.providerContractVerificationHandler)))
	handleFunc("POST /negotiations/{PID}/termination", WrapHandlerWithMetrics(
		"negotiations_ongoing_termination", WrapHandlerWithError(ch.contractTerminationHandler)))

	// Contract negotiation consumer callbacks)
	handleFunc("POST /negotiations/offers", WrapHandlerWithMetrics(
		"negotiation_offer", WrapHandlerWithError(ch.consumerContractOfferHandler)))
	handleFunc("POST /callback/negotiations/{consumerPID}/offers", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_offers", WrapHandlerWithError(ch.consumerContractSpecificOfferHandler)))
	handleFunc("POST /callback/negotiations/{consumerPID}/agreement", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_agreement", WrapHandlerWithError(ch.consumerContractAgreementHandler)))
	handleFunc("POST /callback/negotiations/{consumerPID}/events", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_events", WrapHandlerWithError(ch.consumerContractEventHandler)))
	handleFunc("POST /callback/negotiations/{PID}/termination", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_termination", WrapHandlerWithError(ch.contractTerminationHandler)))
}

func setupCatalogEndpoints(handleFunc func(
	pattern string, handlerFunc func(http.ResponseWriter, *http.Request)), ch dspHandlers,
) {
	handleFunc("POST /catalog/request", WrapHandlerWithMetrics(
		"catalog_request", WrapHandlerWithError(ch.catalogRequestHandler)))
	handleFunc("GET /catalog/datasets/{id}", WrapHandlerWithMetrics(
		"catalog_datasets", WrapHandlerWithError(ch.datasetRequestHandler)))
}
