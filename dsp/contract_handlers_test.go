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

package dsp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go-dataspace.eu/run-dsp/dsp"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/persistence/badger"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	mockdsrpc "go-dataspace.eu/run-dsp/mocks/go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

const (
	dataserviceID  = "testID"
	dataserviceURL = "http://example.com"
)

var (
	staticProviderPID = uuid.MustParse("42e3656b-751c-40e1-a59c-3a07ec047c01")
	staticConsumerPID = uuid.MustParse("435b1eb7-824a-4a88-8dd3-9034b65db45c")
	targetID          = uuid.MustParse("271d90b7-80ed-4f02-856d-5a881efba4ec")
	agreementID       = uuid.MustParse("e76c567b-963a-40f4-ad16-e7d88884d880")
	odrlOffer         = odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			Type: "odrl:Offer",
			PolicyClass: odrl.PolicyClass{
				ID: uuid.MustParse("4e3770fd-63d5-4cd7-bb82-bca2ce0cf563").URN(),
				AbstractPolicyRule: odrl.AbstractPolicyRule{
					Assigner: "urn:blablabla",
				},
				Permission: []odrl.Permission{
					{
						Action: "odrl:use",
					},
				},
			},
			Target: targetID.URN(),
		},
	}
	odrlAgreement = odrl.Agreement{
		Type:        "odrl:Agreement",
		ID:          agreementID.URN(),
		Target:      targetID.URN(),
		Timestamp:   time.Date(1974, time.September, 9, 13, 14, 15, 0, time.UTC),
		PolicyClass: odrl.PolicyClass{},
	}
	callBack = shared.MustParseURL("http://example.com")
	selfURL  = shared.MustParseURL("http://example.org")
	pidMap   = map[constants.DataspaceRole]uuid.UUID{
		constants.DataspaceProvider: staticProviderPID,
		constants.DataspaceConsumer: staticConsumerPID,
	}
)

type mockReconciler struct {
	e statemachine.ReconciliationEntry
}

func (mr *mockReconciler) Add(e statemachine.ReconciliationEntry) {
	mr.e = e
}

type environment struct {
	server          *httptest.Server
	provider        *mockdsrpc.MockProviderServiceClient
	contractService *mockdsrpc.MockContractServiceClient
	store           *badger.StorageProvider
	reconciler      *mockReconciler
}

func mockAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requesterInfo := &dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_UNAUTHENTICATED,
		}

		req = req.WithContext(context.WithValue(req.Context(), authforwarder.RequesterInfoContextKey, requesterInfo))
		next.ServeHTTP(w, req)
	})
}

func setupEnvironment(t *testing.T, autoAccept bool) (
	context.Context,
	context.CancelFunc,
	*environment,
) {
	ctx, cancel := context.WithCancel(t.Context())
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	slog.SetDefault(logger)
	authn := mockdsrpc.NewMockAuthNServiceClient(t)
	prov := mockdsrpc.NewMockProviderServiceClient(t)
	var cService *mockdsrpc.MockContractServiceClient
	if !autoAccept {
		// Only used in the initial requests as we don't set the negotiation there.
		cService = mockdsrpc.NewMockContractServiceClient(t)
	}
	store, err := badger.New(ctx, true, "")
	reconciler := &mockReconciler{}
	assert.Nil(t, err)
	pingResponse := &dsrpc.PingResponse{
		ProviderName:        "bla",
		ProviderDescription: "bla",
		Authenticated:       false,
		DataserviceId:       dataserviceID,
		DataserviceUrl:      dataserviceURL,
	}
	ts := httptest.NewServer(mockAuthMiddleware(
		dsp.GetDSPRoutes(authn, prov, cService, store, reconciler, selfURL, pingResponse)))
	e := environment{
		server:          ts,
		provider:        prov,
		contractService: cService,
		store:           store,
		reconciler:      reconciler,
	}
	return ctx, cancel, &e
}

func fetchAndDecode[T any](ctx context.Context, t *testing.T, method, url string, body io.Reader) T {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	assert.Nil(t, err)
	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { _ = resp.Body.Close() }()
	return decode[T](t, resp.Body)
}

func decode[T any](t *testing.T, body io.Reader) T {
	var thing T
	err := json.NewDecoder(body).Decode(&thing)
	assert.Nil(t, err)
	return thing
}

func encode[T any](t *testing.T, thing T) io.Reader {
	t.Helper()
	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(thing)
	assert.Nil(t, err)
	return b
}

func createNegotiation(
	ctx context.Context,
	t *testing.T,
	store persistence.StorageProvider,
	state contract.State,
	role constants.DataspaceRole,
	autoAccept bool,
	requesterInfo *dsrpc.RequesterInfo,
) {
	t.Helper()
	providerPID := staticProviderPID
	consumerPID := staticConsumerPID
	neg := contract.New(
		ctx,
		providerPID,
		consumerPID,
		state,
		odrlOffer,
		callBack,
		selfURL,
		role,
		autoAccept,
		requesterInfo,
	)
	err := store.PutContract(ctx, neg)
	assert.Nil(t, err)
}

func TestNegotiationStatus(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t, false)
	defer cancel()
	createNegotiation(ctx, t, env.store, contract.States.OFFERED, constants.DataspaceProvider, false, &dsrpc.RequesterInfo{
		AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
	})
	u := env.server.URL + fmt.Sprintf("/negotiations/%s", staticProviderPID.String())
	status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodGet, u, nil)
	assert.Equal(t, "dspace:ContractNegotiation", status.Type)
	assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
	assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
	assert.Equal(t, contract.States.OFFERED.String(), status.State)
}

func mkRequestUrl(u *url.URL, parts ...string) string {
	cu := shared.MustParseURL(u.String())
	parts = append([]string{cu.Path}, parts...)
	cu.Path = path.Join(parts...)
	return cu.String()
}

//nolint:funlen
func TestNegotiationProviderInitialRequest(t *testing.T) {
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, autoAccept)
		defer cancel()

		env.provider.On("GetDataset", mock.Anything, &dsrpc.GetDatasetRequest{
			DatasetId: targetID.String(),
			RequesterInfo: &dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_UNAUTHENTICATED,
			},
		}).Return(&dsrpc.GetDatasetResponse{
			Dataset: &dsrpc.Dataset{},
		}, nil)

		if !autoAccept {
			env.contractService.On(
				"RequestReceived", mock.Anything, mock.Anything,
			).Return(&dsrpc.ContractServiceRequestReceivedResponse{}, nil)
		}
		u := env.server.URL + "/negotiations/request"

		body := encode(t, shared.ContractRequestMessage{
			Context:         shared.GetDSPContext(),
			Type:            "dspace:ContractRequestMessage",
			ConsumerPID:     staticConsumerPID.URN(),
			Offer:           odrlOffer.MessageOffer,
			CallbackAddress: callBack.String(),
		})
		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		assert.NotEqual(t, uuid.UUID{}, status.ProviderPID)
		assert.Equal(t, contract.States.REQUESTED.String(), status.State)

		providerPID := uuid.MustParse(status.ProviderPID)
		negotiation, err := env.store.GetContractR(ctx, providerPID, constants.DataspaceProvider)
		assert.Nil(t, err)
		assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, providerPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.REQUESTED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())

		if autoAccept {
			assert.NotNil(t, env.reconciler.e)
			pid := env.reconciler.e.EntityID
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			assert.Equal(t, contract.States.OFFERED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "offers"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractOfferMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
			assert.Equal(t, pid.URN(), reqPayload.ProviderPID)
			assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			assert.Equal(t, mkRequestUrl(selfURL), reqPayload.CallbackAddress)
		}
	}
}

//nolint:dupl
func TestNegotiationProviderRequest(t *testing.T) {
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false)
		defer cancel()

		createNegotiation(ctx, t, env.store, contract.States.OFFERED, constants.DataspaceProvider, autoAccept,
			&dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			})

		u := env.server.URL + "/negotiations/" + staticProviderPID.String() + "/request"

		if !autoAccept {
			env.contractService.On(
				"RequestReceived", mock.Anything, mock.Anything,
			).Return(&dsrpc.ContractServiceRequestReceivedResponse{}, nil)
		}
		body := encode(t, shared.ContractRequestMessage{
			Context:         shared.GetDSPContext(),
			Type:            "dspace:ContractRequestMessage",
			ConsumerPID:     staticConsumerPID.URN(),
			ProviderPID:     staticProviderPID.URN(),
			Offer:           odrlOffer.MessageOffer,
			CallbackAddress: callBack.String(),
		})
		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		assert.Equal(t, contract.States.REQUESTED.String(), status.State)

		negotiation, err := env.store.GetContractR(ctx, staticProviderPID, constants.DataspaceProvider)
		assert.Nil(t, err)
		assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.REQUESTED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())
		if autoAccept {
			assert.NotNil(t, env.reconciler.e)
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			assert.Equal(t, contract.States.AGREED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "agreement"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractAgreementMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
		}
	}
}

func TestNegotiationProviderEventAccepted(t *testing.T) { //nolint:funlen
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false)
		defer cancel()

		createNegotiation(ctx, t, env.store, contract.States.OFFERED, constants.DataspaceProvider, autoAccept,
			&dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			})

		u := env.server.URL + "/negotiations/" + staticProviderPID.String() + "/events"

		if !autoAccept {
			env.contractService.EXPECT().AcceptedReceived(
				mock.Anything, &dsrpc.ContractServiceAcceptedReceivedRequest{
					Pid: staticProviderPID.String(),
					RequesterInfo: &dsrpc.RequesterInfo{
						AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_CONTINUATION,
					},
				},
			).Return(&dsrpc.ContractServiceAcceptedReceivedResponse{}, nil)
		}
		body := encode(t, shared.ContractNegotiationEventMessage{
			Context:     shared.GetDSPContext(),
			Type:        "dspace:ContractNegotiationEventMessage",
			ConsumerPID: staticConsumerPID.URN(),
			ProviderPID: staticProviderPID.URN(),
			EventType:   contract.States.ACCEPTED.String(),
		})
		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		assert.Equal(t, contract.States.ACCEPTED.String(), status.State)

		negotiation, err := env.store.GetContractR(ctx, staticProviderPID, constants.DataspaceProvider)
		assert.Nil(t, err)
		assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.ACCEPTED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())

		//nolint:dupl
		if autoAccept {
			assert.NotNil(t, env.reconciler.e)
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			assert.Equal(t, contract.States.AGREED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "agreement"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractAgreementMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
		}
	}
}

func TestNegotiationProviderAgreementVerification(t *testing.T) { //nolint:funlen
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false)
		defer cancel()

		createNegotiation(ctx, t, env.store, contract.States.AGREED, constants.DataspaceProvider, autoAccept,
			&dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			})

		u := env.server.URL + "/negotiations/" + staticProviderPID.String() + "/agreement/verification"

		if !autoAccept {
			env.contractService.EXPECT().VerificationReceived(
				mock.Anything, &dsrpc.ContractServiceVerificationReceivedRequest{
					Pid: staticProviderPID.String(),
					RequesterInfo: &dsrpc.RequesterInfo{
						AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_CONTINUATION,
					},
				},
			).Return(&dsrpc.ContractServiceVerificationReceivedResponse{}, nil)
		}

		body := encode(t, shared.ContractAgreementVerificationMessage{
			Context:     shared.GetDSPContext(),
			Type:        "dspace:ContractAgreementVerificationMessage",
			ConsumerPID: staticConsumerPID.URN(),
			ProviderPID: staticProviderPID.URN(),
		})
		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		assert.Equal(t, contract.States.VERIFIED.String(), status.State)

		negotiation, err := env.store.GetContractR(ctx, staticProviderPID, constants.DataspaceProvider)
		assert.Nil(t, err)
		assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.VERIFIED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())

		//nolint:dupl
		if autoAccept {
			assert.NotNil(t, env.reconciler.e)
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			assert.Equal(t, contract.States.FINALIZED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "events"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractNegotiationEventMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			assert.Equal(t, contract.States.FINALIZED.String(), reqPayload.EventType)
		}
	}
}

func TestNegotiationConsumerInitialOffer(t *testing.T) {
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, autoAccept)
		defer cancel()

		u := env.server.URL + "/negotiations/offers"

		if !autoAccept {
			env.contractService.On(
				"OfferReceived", mock.Anything, mock.Anything,
			).Return(&dsrpc.ContractServiceOfferReceivedResponse{}, nil)
		}

		body := encode(t, shared.ContractOfferMessage{
			Context:         shared.GetDSPContext(),
			Type:            "dspace:ContractOfferMessage",
			ProviderPID:     staticProviderPID.URN(),
			Offer:           odrlOffer.MessageOffer,
			CallbackAddress: callBack.String(),
		})
		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		assert.NotEqual(t, uuid.UUID{}, status.ConsumerPID)
		assert.Equal(t, contract.States.OFFERED.String(), status.State)

		consumerPID := uuid.MustParse(status.ConsumerPID)
		negotiation, err := env.store.GetContractR(ctx, consumerPID, constants.DataspaceConsumer)
		assert.Nil(t, err)
		assert.Equal(t, consumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.OFFERED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())

		if autoAccept {
			assert.NotNil(t, env.reconciler.e)
			pid := env.reconciler.e.EntityID
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
			assert.Equal(t, contract.States.REQUESTED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(t, mkRequestUrl(
				callBack,
				"negotiations",
				staticProviderPID.String(),
				"request",
			), env.reconciler.e.URL.String())

			var reqPayload shared.ContractRequestMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
			assert.Equal(t, pid.URN(), reqPayload.ConsumerPID)
			assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			assert.Equal(t, mkRequestUrl(selfURL, "callback"), reqPayload.CallbackAddress)
		}
	}
}

//nolint:dupl
func TestNegotiationConsumerOffer(t *testing.T) { //nolint:funlen
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false)
		defer cancel()

		createNegotiation(ctx, t, env.store, contract.States.REQUESTED, constants.DataspaceConsumer, autoAccept,
			&dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			})

		u := env.server.URL + "/callback/negotiations/" + staticConsumerPID.String() + "/offers"

		if !autoAccept {
			env.contractService.On(
				"OfferReceived", mock.Anything, mock.Anything,
			).Return(&dsrpc.ContractServiceOfferReceivedResponse{}, nil)
		}

		body := encode(t, shared.ContractOfferMessage{
			Context:         shared.GetDSPContext(),
			Type:            "dspace:ContractOfferMessage",
			ConsumerPID:     staticConsumerPID.URN(),
			ProviderPID:     staticProviderPID.URN(),
			Offer:           odrlOffer.MessageOffer,
			CallbackAddress: callBack.String(),
		})
		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		assert.Equal(t, contract.States.OFFERED.String(), status.State)

		negotiation, err := env.store.GetContractR(ctx, staticConsumerPID, constants.DataspaceConsumer)
		assert.Nil(t, err)
		assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.OFFERED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())

		if autoAccept {
			assert.NotNil(t, env.reconciler.e)
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
			assert.Equal(t, contract.States.ACCEPTED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(t, mkRequestUrl(
				callBack,
				"negotiations",
				staticProviderPID.String(),
				"events",
			), env.reconciler.e.URL.String())

			var reqPayload shared.ContractNegotiationEventMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			assert.Equal(t, contract.States.ACCEPTED.String(), reqPayload.EventType)
		}
	}
}

//nolint:funlen
func TestNegotiationConsumerAgreement(t *testing.T) {
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false)
		defer cancel()

		for _, s := range []contract.State{contract.States.REQUESTED, contract.States.ACCEPTED} {
			createNegotiation(ctx, t, env.store, s, constants.DataspaceConsumer, autoAccept, &dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			})

			if !autoAccept {
				env.contractService.EXPECT().AgreementReceived(
					mock.Anything, &dsrpc.ContractServiceAgreementReceivedRequest{
						Pid: staticConsumerPID.String(),
						RequesterInfo: &dsrpc.RequesterInfo{
							AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_CONTINUATION,
						},
					},
				).Return(&dsrpc.ContractServiceAgreementReceivedResponse{}, nil)
			}

			u := env.server.URL + "/callback/negotiations/" + staticConsumerPID.String() + "/agreement"

			body := encode(t, shared.ContractAgreementMessage{
				Context:         shared.GetDSPContext(),
				Type:            "dspace:ContractAgreementMessage",
				ConsumerPID:     staticConsumerPID.URN(),
				ProviderPID:     staticProviderPID.URN(),
				Agreement:       odrlAgreement,
				CallbackAddress: callBack.String(),
			})

			status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
			assert.Equal(t, "dspace:ContractNegotiation", status.Type)
			assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
			assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
			assert.Equal(t, contract.States.AGREED.String(), status.State)

			negotiation, err := env.store.GetContractR(ctx, staticConsumerPID, constants.DataspaceConsumer)
			assert.Nil(t, err)
			assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
			assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
			assert.Equal(t, contract.States.AGREED, negotiation.GetState())
			assert.Equal(t, odrlOffer, negotiation.GetOffer())
			assert.Equal(t, &odrlAgreement, negotiation.GetAgreement())
			assert.Equal(t, callBack.String(), negotiation.GetCallback().String())

			if autoAccept {
				assert.NotNil(t, env.reconciler.e)
				assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
				assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
				assert.Equal(t, contract.States.VERIFIED.String(), env.reconciler.e.TargetState)
				assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
				assert.Equal(t, mkRequestUrl(
					callBack,
					"negotiations",
					staticProviderPID.String(),
					"agreement",
					"verification",
				), env.reconciler.e.URL.String())

				var reqPayload shared.ContractAgreementVerificationMessage
				err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
				assert.Nil(t, err)
				assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
				assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			}
		}
	}
}

func TestNegotiationConsumerEventFinalized(t *testing.T) {
	t.Parallel()
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false)
		defer cancel()

		if !autoAccept {
			odrl, err := json.Marshal(odrlOffer)
			assert.Nil(t, err)
			env.contractService.EXPECT().FinalizationReceived(
				mock.Anything, &dsrpc.ContractServiceFinalizationReceivedRequest{
					Pid: staticConsumerPID.String(),
					RequesterInfo: &dsrpc.RequesterInfo{
						AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_CONTINUATION,
					},
					Offer: string(odrl),
				},
			).Return(&dsrpc.ContractServiceFinalizationReceivedResponse{}, nil)
		}
		createNegotiation(ctx, t, env.store, contract.States.VERIFIED, constants.DataspaceConsumer, autoAccept,
			&dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			})

		u := env.server.URL + "/callback/negotiations/" + staticConsumerPID.String() + "/events"

		body := encode(t, shared.ContractNegotiationEventMessage{
			Context:     shared.GetDSPContext(),
			Type:        "dspace:ContractNegotiationEventMessage",
			ConsumerPID: staticConsumerPID.URN(),
			ProviderPID: staticProviderPID.URN(),
			EventType:   contract.States.FINALIZED.String(),
		})

		status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
		assert.Equal(t, "dspace:ContractNegotiation", status.Type)
		assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		assert.Equal(t, contract.States.FINALIZED.String(), status.State)

		negotiation, err := env.store.GetContractR(ctx, staticConsumerPID, constants.DataspaceConsumer)
		assert.Nil(t, err)
		assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		assert.Equal(t, contract.States.FINALIZED, negotiation.GetState())
		assert.Equal(t, odrlOffer, negotiation.GetOffer())
		assert.Equal(t, callBack.String(), negotiation.GetCallback().String())
	}
}

func TestNegotiationTermination(t *testing.T) {
	t.Parallel()

	for _, autoAccept := range []bool{true, false} {
		for _, r := range []constants.DataspaceRole{constants.DataspaceConsumer, constants.DataspaceProvider} {
			for _, s := range []contract.State{
				contract.States.REQUESTED,
				contract.States.OFFERED,
				contract.States.ACCEPTED,
				contract.States.AGREED,
				contract.States.VERIFIED,
			} {
				ctx, cancel, env := setupEnvironment(t, false)
				defer cancel()
				u := env.server.URL + "/negotiations/" + pidMap[r].String() + "/termination"
				createNegotiation(ctx, t, env.store, s, r, autoAccept, &dsrpc.RequesterInfo{
					AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
				})
				pid := staticConsumerPID.String()
				if r == constants.DataspaceProvider {
					pid = staticProviderPID.String()
				}

				if !autoAccept {
					env.contractService.EXPECT().TerminationReceived(
						mock.Anything, &dsrpc.ContractServiceTerminationReceivedRequest{
							Pid:    pid,
							Code:   "some code",
							Reason: []string{"test"},
						},
					).Return(&dsrpc.ContractServiceTerminationReceivedResponse{}, nil)
				}

				body := encode(t, shared.ContractNegotiationTerminationMessage{
					Context:     shared.GetDSPContext(),
					Type:        "dspace:ContractNegotiationTerminationMessage",
					ConsumerPID: staticConsumerPID.URN(),
					ProviderPID: staticProviderPID.URN(),
					Code:        "some code",
					Reason: []shared.Multilanguage{{
						Value:    "test",
						Language: "en",
					}},
				})
				status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodPost, u, body)
				assert.Equal(t, "dspace:ContractNegotiation", status.Type)
				assert.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
				assert.Equal(t, staticProviderPID.URN(), status.ProviderPID)
				assert.Equal(t, contract.States.TERMINATED.String(), status.State)

				negotiation, err := env.store.GetContractR(ctx, pidMap[r], r)
				assert.Nil(t, err)
				assert.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
				assert.Equal(t, staticProviderPID, negotiation.GetProviderPID())
				assert.Equal(t, contract.States.TERMINATED, negotiation.GetState())
				assert.Equal(t, odrlOffer, negotiation.GetOffer())
				assert.Equal(t, callBack.String(), negotiation.GetCallback().String())
			}
		}
	}
}
