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
)

// GetRoutes gets all the dataspace routes.
func GetWellKnownRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /dspace-version", WrapHandlerWithMetrics("dspace-version", WrapHandlerWithError(dspaceVersionHandler)))
	// This is an optional proof endpoint for protected datasets.
	mux.HandleFunc("GET /dspace-trust", routeNotImplemented)
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
	mux.Handle("POST /catalog/request", WrapHandlerWithMetrics(
		"catalog_request", WrapHandlerWithError(ch.catalogRequestHandler)))
	mux.Handle("GET /catalog/datasets/{id}", WrapHandlerWithMetrics(
		"catalog_datasets", WrapHandlerWithError(ch.datasetRequestHandler)))

	// Contract negotiation endpoints
	mux.Handle("GET /negotiations/{providerPID}", WrapHandlerWithMetrics(
		"negotiations", WrapHandlerWithError(ch.providerContractStateHandler)))
	mux.Handle("POST /negotiations/request", WrapHandlerWithMetrics(
		"negotiations_request", WrapHandlerWithError(ch.providerContractRequestHandler)))
	mux.Handle("POST /negotiations/{providerPID}/request", WrapHandlerWithMetrics(
		"negotiations_ongoing_request", WrapHandlerWithError(ch.providerContractSpecificRequestHandler)))
	mux.Handle("POST /negotiations/{providerPID}/events", WrapHandlerWithMetrics(
		"negotiations_ongoing_events", WrapHandlerWithError(ch.providerContractEventHandler)))
	mux.Handle("POST /negotiations/{providerPID}/agreement/verification", WrapHandlerWithMetrics(
		"negotiations_ongoing_agreement_verification", WrapHandlerWithError(ch.providerContractVerificationHandler)))
	mux.Handle("POST /negotiations/{PID}/termination", WrapHandlerWithMetrics(
		"negotiations_ongoing_termination", WrapHandlerWithError(ch.contractTerminationHandler)))

	// Contract negotiation consumer callbacks)
	mux.Handle("POST /negotiations/offers", WrapHandlerWithMetrics(
		"negotiation_offer", WrapHandlerWithError(ch.consumerContractOfferHandler)))
	mux.Handle("POST /callback/negotiations/{consumerPID}/offers", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_offers", WrapHandlerWithError(ch.consumerContractSpecificOfferHandler)))
	mux.Handle("POST /callback/negotiations/{consumerPID}/agreement", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_agreement", WrapHandlerWithError(ch.consumerContractAgreementHandler)))
	mux.Handle("POST /callback/negotiations/{consumerPID}/events", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_events", WrapHandlerWithError(ch.consumerContractEventHandler)))
	mux.Handle("POST /callback/negotiations/{PID}/termination", WrapHandlerWithMetrics(
		"callback_negotiations_ongoing_termination", WrapHandlerWithError(ch.contractTerminationHandler)))

	// Transfer process endpoints
	mux.Handle("GET /transfers/{providerPID}", WrapHandlerWithMetrics(
		"transfers_ongoing", WrapHandlerWithError(ch.providerTransferProcessHandler)))
	mux.Handle("POST /transfers/request", WrapHandlerWithMetrics(
		"transfers_request", WrapHandlerWithError(ch.providerTransferRequestHandler)))
	mux.Handle("POST /transfers/{providerPID}/start", WrapHandlerWithMetrics(
		"transfers_ongoing_start", WrapHandlerWithError(ch.providerTransferStartHandler)))
	mux.Handle("POST /transfers/{providerPID}/completion", WrapHandlerWithMetrics(
		"transfers_ongoing_completion", WrapHandlerWithError(ch.providerTransferCompletionHandler)))
	mux.Handle("POST /transfers/{providerPID}/termination", WrapHandlerWithMetrics(
		"transfers_ongoing_termination", WrapHandlerWithError(ch.providerTransferTerminationHandler)))
	mux.Handle("POST /transfers/{providerPID}/suspension", WrapHandlerWithMetrics(
		"transfers_ongoing_suspension", WrapHandlerWithError(ch.providerTransferSuspensionHandler)))
	// Transfer process consumer callbacks
	mux.Handle("POST /callback/transfers/{consumerPID}/start", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_start", WrapHandlerWithError(ch.consumerTransferStartHandler)))
	mux.Handle("POST /callback/transfers/{consumerPID}/completion", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_completion", WrapHandlerWithError(ch.consumerTransferCompletionHandler)))
	mux.Handle("POST /callback/transfers/{consumerPID}/termination", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_termination", WrapHandlerWithError(ch.consumerTransferTerminationHandler)))
	mux.Handle("POST /callback/transfers/{consumerPID}/suspension", WrapHandlerWithMetrics(
		"callback_transfers_ongoing_suspension", WrapHandlerWithError(ch.consumerTransferSuspensionHandler)))

	return mux
}
