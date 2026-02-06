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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go-dataspace.eu/run-dsp/dsp"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
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
	store           *sqlite.Provider
	reconciler      *mockReconciler
}

func (e *environment) Close() {
	if err := e.store.Close(); err != nil {
		panic(err)
	}
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

//nolint:unparam // This is sometimes set to true when debugging tests.
func setupEnvironment(t *testing.T, autoAccept, debugDB bool) (
	context.Context,
	context.CancelFunc,
	*environment,
) {
	t.Helper()
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
	store, err := sqlite.New(ctx, true, debugDB, "")
	require.Nil(t, err)
	err = store.Migrate(ctx)
	require.Nil(t, err)
	reconciler := &mockReconciler{}
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
	require.Nil(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.Nilf(t, err, "got error: %s", err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { _ = resp.Body.Close() }()
	return decode[T](t, resp.Body)
}

func decode[T any](t *testing.T, body io.Reader) T {
	var thing T
	err := json.NewDecoder(body).Decode(&thing)
	require.Nil(t, err)
	return thing
}

func encode[T any](t *testing.T, thing T) io.Reader {
	t.Helper()
	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(thing)
	require.Nil(t, err)
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
	require.Nil(t, err)
}

func TestNegotiationStatus(t *testing.T) {
	ctx, cancel, env := setupEnvironment(t, false, false)
	defer cancel()
	defer env.Close()
	createNegotiation(ctx, t, env.store, contract.States.OFFERED, constants.DataspaceProvider, false, &dsrpc.RequesterInfo{
		AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
	})
	u := env.server.URL + fmt.Sprintf("/negotiations/%s", staticProviderPID.String())
	status := fetchAndDecode[shared.ContractNegotiation](ctx, t, http.MethodGet, u, nil)
	require.Equal(t, "dspace:ContractNegotiation", status.Type)
	require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
	require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
	require.Equal(t, contract.States.OFFERED.String(), status.State)
}

func mkRequestUrl(u *url.URL, parts ...string) string {
	cu := shared.MustParseURL(u.String())
	parts = append([]string{cu.Path}, parts...)
	cu.Path = path.Join(parts...)
	return cu.String()
}

//nolint:funlen
func TestNegotiationProviderInitialRequest(t *testing.T) {
	for _, autoAccept := range []bool{true, true} {
		ctx, cancel, env := setupEnvironment(t, autoAccept, false)

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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		require.NotEqual(t, uuid.UUID{}, status.ProviderPID)
		require.Equal(t, contract.States.REQUESTED.String(), status.State)

		providerPID := uuid.MustParse(status.ProviderPID)
		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(providerPID, constants.DataspaceProvider),
		)
		require.Nil(t, err)
		require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		require.Equal(t, providerPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.REQUESTED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())

		if autoAccept {
			require.NotNil(t, env.reconciler.e)
			pid := env.reconciler.e.EntityID
			require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			require.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			require.Equal(t, contract.States.OFFERED.String(), env.reconciler.e.TargetState)
			require.Equal(t, http.MethodPost, env.reconciler.e.Method)
			require.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "offers"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractOfferMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			require.Nil(t, err)
			require.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
			require.Equal(t, pid.URN(), reqPayload.ProviderPID)
			require.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			require.Equal(t, mkRequestUrl(selfURL), reqPayload.CallbackAddress)
		}
		cancel()
		env.Close()
	}
}

//nolint:dupl
func TestNegotiationProviderRequest(t *testing.T) {
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false, false)

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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		require.Equal(t, contract.States.REQUESTED.String(), status.State)

		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(staticProviderPID, constants.DataspaceProvider),
		)
		require.Nil(t, err)
		require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.REQUESTED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())
		if autoAccept {
			require.NotNil(t, env.reconciler.e)
			require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			require.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			require.Equal(t, contract.States.AGREED.String(), env.reconciler.e.TargetState)
			require.Equal(t, http.MethodPost, env.reconciler.e.Method)
			require.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "agreement"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractAgreementMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			require.Nil(t, err)
			require.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			require.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
		}
		cancel()
		env.Close()
	}
}

func TestNegotiationProviderEventAccepted(t *testing.T) { //nolint:funlen
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false, false)

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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		require.Equal(t, contract.States.ACCEPTED.String(), status.State)

		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(staticProviderPID, constants.DataspaceProvider),
		)
		require.Nil(t, err)
		require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.ACCEPTED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())

		//nolint:dupl
		if autoAccept {
			require.NotNil(t, env.reconciler.e)
			require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			require.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			require.Equal(t, contract.States.AGREED.String(), env.reconciler.e.TargetState)
			require.Equal(t, http.MethodPost, env.reconciler.e.Method)
			require.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "agreement"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractAgreementMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			require.Nil(t, err)
			require.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			require.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
		}
		cancel()
		env.Close()
	}
}

func TestNegotiationProviderAgreementVerification(t *testing.T) { //nolint:funlen
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false, false)

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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		require.Equal(t, contract.States.VERIFIED.String(), status.State)

		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(staticProviderPID, constants.DataspaceProvider),
		)
		require.Nil(t, err)
		require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.VERIFIED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())

		//nolint:dupl
		if autoAccept {
			require.NotNil(t, env.reconciler.e)
			require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			require.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
			require.Equal(t, contract.States.FINALIZED.String(), env.reconciler.e.TargetState)
			require.Equal(t, http.MethodPost, env.reconciler.e.Method)
			require.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", staticConsumerPID.String(), "events"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractNegotiationEventMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			require.Nil(t, err)
			require.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			require.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			require.Equal(t, contract.States.FINALIZED.String(), reqPayload.EventType)
		}
		cancel()
		env.Close()
	}
}

func TestNegotiationConsumerInitialOffer(t *testing.T) {
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, autoAccept, false)

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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		require.NotEqual(t, uuid.UUID{}, status.ConsumerPID)
		require.Equal(t, contract.States.OFFERED.String(), status.State)

		consumerPID := uuid.MustParse(status.ConsumerPID)
		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(consumerPID, constants.DataspaceConsumer),
		)
		require.Nil(t, err)
		require.Equal(t, consumerPID, negotiation.GetConsumerPID())
		require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.OFFERED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())

		if autoAccept {
			require.NotNil(t, env.reconciler.e)
			pid := env.reconciler.e.EntityID
			require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			require.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
			require.Equal(t, contract.States.REQUESTED.String(), env.reconciler.e.TargetState)
			require.Equal(t, http.MethodPost, env.reconciler.e.Method)
			require.Equal(t, mkRequestUrl(
				callBack,
				"negotiations",
				staticProviderPID.String(),
				"request",
			), env.reconciler.e.URL.String())

			var reqPayload shared.ContractRequestMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			require.Nil(t, err)
			require.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
			require.Equal(t, pid.URN(), reqPayload.ConsumerPID)
			require.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			require.Equal(t, mkRequestUrl(selfURL, "callback"), reqPayload.CallbackAddress)
		}
		cancel()
		env.Close()
	}
}

//nolint:dupl
func TestNegotiationConsumerOffer(t *testing.T) { //nolint:funlen
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false, false)

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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		require.Equal(t, contract.States.OFFERED.String(), status.State)

		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(staticConsumerPID, constants.DataspaceConsumer),
		)
		require.Nil(t, err)
		require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.OFFERED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())

		if autoAccept {
			require.NotNil(t, env.reconciler.e)
			require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			require.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
			require.Equal(t, contract.States.ACCEPTED.String(), env.reconciler.e.TargetState)
			require.Equal(t, http.MethodPost, env.reconciler.e.Method)
			require.Equal(t, mkRequestUrl(
				callBack,
				"negotiations",
				staticProviderPID.String(),
				"events",
			), env.reconciler.e.URL.String())

			var reqPayload shared.ContractNegotiationEventMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			require.Nil(t, err)
			require.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			require.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			require.Equal(t, contract.States.ACCEPTED.String(), reqPayload.EventType)
		}
		cancel()
		env.Close()
	}
}

//nolint:funlen
func TestNegotiationConsumerAgreement(t *testing.T) {
	for _, autoAccept := range []bool{true, false} {
		for _, s := range []contract.State{contract.States.REQUESTED, contract.States.ACCEPTED} {
			ctx, cancel, env := setupEnvironment(t, false, false)
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
			require.Equal(t, "dspace:ContractNegotiation", status.Type)
			require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
			require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
			require.Equal(t, contract.States.AGREED.String(), status.State)

			negotiation, err := env.store.GetContract(
				ctx,
				contractopts.WithRolePID(staticConsumerPID, constants.DataspaceConsumer),
			)
			require.Nil(t, err)
			require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
			require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
			require.Equal(t, contract.States.AGREED, negotiation.GetState())
			require.Equal(t, odrlOffer, negotiation.GetOffer())
			require.Equal(t, &odrlAgreement, negotiation.GetAgreement())
			require.Equal(t, callBack.String(), negotiation.GetCallback().String())

			if autoAccept {
				require.NotNil(t, env.reconciler.e)
				require.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
				require.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
				require.Equal(t, contract.States.VERIFIED.String(), env.reconciler.e.TargetState)
				require.Equal(t, http.MethodPost, env.reconciler.e.Method)
				require.Equal(t, mkRequestUrl(
					callBack,
					"negotiations",
					staticProviderPID.String(),
					"agreement",
					"verification",
				), env.reconciler.e.URL.String())

				var reqPayload shared.ContractAgreementVerificationMessage
				err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
				require.Nil(t, err)
				require.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
				require.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			}
			cancel()
			env.Close()
		}
	}
}

func TestNegotiationConsumerEventFinalized(t *testing.T) {
	for _, autoAccept := range []bool{true, false} {
		ctx, cancel, env := setupEnvironment(t, false, false)

		if !autoAccept {
			odrl, err := json.Marshal(odrlOffer)
			require.Nil(t, err)
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
		require.Equal(t, "dspace:ContractNegotiation", status.Type)
		require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
		require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
		require.Equal(t, contract.States.FINALIZED.String(), status.State)

		negotiation, err := env.store.GetContract(
			ctx,
			contractopts.WithRolePID(staticConsumerPID, constants.DataspaceConsumer),
		)
		require.Nil(t, err)
		require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
		require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
		require.Equal(t, contract.States.FINALIZED, negotiation.GetState())
		require.Equal(t, odrlOffer, negotiation.GetOffer())
		require.Equal(t, callBack.String(), negotiation.GetCallback().String())
		cancel()
		env.Close()
	}
}

func TestNegotiationTermination(t *testing.T) {
	for _, autoAccept := range []bool{true, false} {
		for _, r := range []constants.DataspaceRole{constants.DataspaceConsumer, constants.DataspaceProvider} {
			for _, s := range []contract.State{
				contract.States.REQUESTED,
				contract.States.OFFERED,
				contract.States.ACCEPTED,
				contract.States.AGREED,
				contract.States.VERIFIED,
			} {
				ctx, cancel, env := setupEnvironment(t, false, false)
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
				require.Equal(t, "dspace:ContractNegotiation", status.Type)
				require.Equal(t, staticConsumerPID.URN(), status.ConsumerPID)
				require.Equal(t, staticProviderPID.URN(), status.ProviderPID)
				require.Equal(t, contract.States.TERMINATED.String(), status.State)

				negotiation, err := env.store.GetContract(
					ctx,
					contractopts.WithRolePID(pidMap[r], r),
				)
				require.Nil(t, err)
				require.Equal(t, staticConsumerPID, negotiation.GetConsumerPID())
				require.Equal(t, staticProviderPID, negotiation.GetProviderPID())
				require.Equal(t, contract.States.TERMINATED, negotiation.GetState())
				require.Equal(t, odrlOffer, negotiation.GetOffer())
				require.Equal(t, callBack.String(), negotiation.GetCallback().String())
				cancel()
				env.Close()
			}
		}
	}
}
