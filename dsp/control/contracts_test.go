// Copyright 2025 go-dataspace
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

package control_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/control"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/persistence/badger"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	mockdsrpc "go-dataspace.eu/run-dsp/mocks/go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

var (
	staticProviderPID = uuid.MustParse("42e3656b-751c-40e1-a59c-3a07ec047c01")
	staticConsumerPID = uuid.MustParse("435b1eb7-824a-4a88-8dd3-9034b65db45c")
	targetID          = uuid.MustParse("271d90b7-80ed-4f02-856d-5a881efba4ec")
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
	callBack        = shared.MustParseURL("http://example.com")
	selfURL         = shared.MustParseURL("http://example.org")
	versionResponse = `{
	"@context": "https://w3id.org/dspace/2024/1/context.json",
	"protocolVersions": [{"version": "2024-1", "path": "/p"}]}`
)

type mockReconciler struct {
	e statemachine.ReconciliationEntry
}

func (mr *mockReconciler) Add(e statemachine.ReconciliationEntry) {
	mr.e = e
}

type mockRequester struct {
	method  string
	u       *url.URL
	reqBody []byte
}

func (mr *mockRequester) SendHTTPRequest(
	_ context.Context,
	method string,
	url *url.URL,
	reqBody []byte,
) ([]byte, error) {
	mr.method = method
	mr.u = url
	mr.reqBody = reqBody
	return []byte(versionResponse), nil
}

type environment struct {
	server          *control.Server
	provider        *mockdsrpc.MockProviderServiceClient
	contractService *mockdsrpc.MockContractServiceClient
	store           *badger.StorageProvider
	reconciler      *mockReconciler
	requester       *mockRequester
}

func setupEnvironment(t *testing.T) (
	context.Context,
	context.CancelFunc,
	*environment,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	slog.SetDefault(logger)
	prov := mockdsrpc.NewMockProviderServiceClient(t)
	cService := mockdsrpc.NewMockContractServiceClient(t)
	store, err := badger.New(ctx, true, "")
	reconciler := &mockReconciler{}
	requester := &mockRequester{}
	assert.Nil(t, err)
	ts := control.New(requester, store, reconciler, prov, cService, selfURL)
	e := environment{
		server:          ts,
		provider:        prov,
		contractService: cService,
		store:           store,
		reconciler:      reconciler,
		requester:       requester,
	}
	return ctx, cancel, &e
}

func createNegotiation(
	ctx context.Context,
	t *testing.T,
	store persistence.StorageProvider,
	state contract.State,
	role constants.DataspaceRole,
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
		false,
		&dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
			Identifier:           staticProviderPID.String(),
		},
	)
	err := store.PutContract(ctx, neg)
	assert.Nil(t, err)
}

func TestVerifyConnection(t *testing.T) {
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()
	err := env.store.PutToken(ctx, "contract-token", "test")
	assert.Nil(t, err)
	_, err = env.server.VerifyConnection(ctx, &dsrpc.VerifyConnectionRequest{
		VerificationToken: "test",
	})
	assert.Nil(t, err)
	_, err = env.server.VerifyConnection(ctx, &dsrpc.VerifyConnectionRequest{
		VerificationToken: "nothere",
	})
	assert.NotNil(t, err)
}

func mkRequestUrl(u *url.URL, parts ...string) string {
	cu := shared.MustParseURL(u.String())
	parts = append([]string{cu.Path}, parts...)
	cu.Path = path.Join(parts...)
	return cu.String()
}

func TestContractRequest_Initial(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	offer, err := json.Marshal(odrlOffer)
	assert.Nil(t, err)
	pa := callBack.String()
	_, err = env.server.ContractRequest(ctx, &dsrpc.ContractRequestRequest{
		Offer:              string(offer),
		ParticipantAddress: &pa,
	})

	assert.Nil(t, err)
	assert.Equal(t, http.MethodGet, env.requester.method)
	assert.Equal(t, mkRequestUrl(callBack, ".well-known", "dspace-version"), env.requester.u.String())
	assert.Nil(t, env.requester.reqBody)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
	assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
	assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
	assert.Equal(t, contract.States.REQUESTED.String(), env.reconciler.e.TargetState)
	assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
	assert.Equal(t, mkRequestUrl(callBack, "p", "negotiations", "request"), env.reconciler.e.URL.String())

	var reqPayload shared.ContractRequestMessage
	err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
	assert.Nil(t, err)
	assert.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
	assert.Equal(t, pid.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, "", reqPayload.ProviderPID)
	assert.Equal(t, mkRequestUrl(selfURL), reqPayload.CallbackAddress)
}

//nolint:dupl
func TestContractRequest(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	createNegotiation(ctx, t, env.store, contract.States.OFFERED, constants.DataspaceConsumer)

	strPID := staticConsumerPID.String()
	_, err := env.server.ContractRequest(ctx, &dsrpc.ContractRequestRequest{
		Pid: &strPID,
	})

	assert.Nil(t, err)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
	assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
	assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
	assert.Equal(t, contract.States.REQUESTED.String(), env.reconciler.e.TargetState)
	assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
	assert.Equal(
		t,
		mkRequestUrl(callBack, "negotiations", staticProviderPID.String(), "request"),
		env.reconciler.e.URL.String(),
	)

	var reqPayload shared.ContractRequestMessage
	err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
	assert.Nil(t, err)
	assert.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
	assert.Equal(t, pid.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
	assert.Equal(t, mkRequestUrl(selfURL), reqPayload.CallbackAddress)
}

func TestContractOffer_Initial(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	env.provider.On("GetDataset", mock.Anything, &dsrpc.GetDatasetRequest{
		DatasetId: targetID.String(),
		RequesterInfo: &dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
		},
	}).Return(&dsrpc.GetDatasetResponse{
		Dataset: &dsrpc.Dataset{},
	}, nil)

	offer, err := json.Marshal(odrlOffer)
	assert.Nil(t, err)
	pa := callBack.String()
	_, err = env.server.ContractOffer(ctx, &dsrpc.ContractOfferRequest{
		Offer:              string(offer),
		ParticipantAddress: &pa,
	})

	assert.Nil(t, err)
	assert.Equal(t, http.MethodGet, env.requester.method)
	assert.Equal(t, mkRequestUrl(callBack, ".well-known", "dspace-version"), env.requester.u.String())
	assert.Nil(t, env.requester.reqBody)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
	assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
	assert.Equal(t, constants.DataspaceProvider, env.reconciler.e.Role)
	assert.Equal(t, contract.States.OFFERED.String(), env.reconciler.e.TargetState)
	assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
	assert.Equal(t, mkRequestUrl(callBack, "p", "negotiations", "offers"), env.reconciler.e.URL.String())

	var reqPayload shared.ContractOfferMessage
	err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
	assert.Nil(t, err)
	assert.Equal(t, odrlOffer.MessageOffer, reqPayload.Offer)
	assert.Equal(t, pid.URN(), reqPayload.ProviderPID)
	assert.Equal(t, "", reqPayload.ConsumerPID)
	assert.Equal(t, mkRequestUrl(selfURL), reqPayload.CallbackAddress)
}

//nolint:dupl
func TestContractOffer(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	createNegotiation(ctx, t, env.store, contract.States.REQUESTED, constants.DataspaceProvider)

	strPID := staticProviderPID.String()
	_, err := env.server.ContractOffer(ctx, &dsrpc.ContractOfferRequest{
		Pid: &strPID,
	})

	assert.Nil(t, err)

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

//nolint:dupl
func TestContractAccept(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	createNegotiation(ctx, t, env.store, contract.States.OFFERED, constants.DataspaceConsumer)

	strPID := staticConsumerPID.String()
	_, err := env.server.ContractAccept(ctx, &dsrpc.ContractAcceptRequest{
		Pid: strPID,
	})

	assert.Nil(t, err)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
	assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
	assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
	assert.Equal(t, contract.States.ACCEPTED.String(), env.reconciler.e.TargetState)
	assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
	assert.Equal(
		t,
		mkRequestUrl(callBack, "negotiations", staticProviderPID.String(), "events"),
		env.reconciler.e.URL.String(),
	)

	var reqPayload shared.ContractNegotiationEventMessage
	err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
	assert.Nil(t, err)
	assert.Equal(t, pid.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
	assert.Equal(t, contract.States.ACCEPTED.String(), reqPayload.EventType)
}

func TestContractAgree(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	createNegotiation(ctx, t, env.store, contract.States.ACCEPTED, constants.DataspaceProvider)

	strPID := staticProviderPID.String()
	_, err := env.server.ContractAgree(ctx, &dsrpc.ContractAgreeRequest{
		Pid: strPID,
	})

	assert.Nil(t, err)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
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
	assert.Equal(t, pid.URN(), reqPayload.ProviderPID)
	assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
}

func TestContractVerify(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	createNegotiation(ctx, t, env.store, contract.States.AGREED, constants.DataspaceConsumer)

	strPID := staticConsumerPID.String()
	_, err := env.server.ContractVerify(ctx, &dsrpc.ContractVerifyRequest{
		Pid: strPID,
	})

	assert.Nil(t, err)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
	assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
	assert.Equal(t, constants.DataspaceConsumer, env.reconciler.e.Role)
	assert.Equal(t, contract.States.VERIFIED.String(), env.reconciler.e.TargetState)
	assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
	assert.Equal(
		t,
		mkRequestUrl(callBack, "negotiations", staticProviderPID.String(), "agreement", "verification"),
		env.reconciler.e.URL.String(),
	)

	var reqPayload shared.ContractAgreementMessage
	err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
	assert.Nil(t, err)
	assert.Equal(t, pid.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
}

//nolint:dupl
func TestContractFinalize(t *testing.T) {
	t.Parallel()
	ctx, cancel, env := setupEnvironment(t)
	defer cancel()

	createNegotiation(ctx, t, env.store, contract.States.VERIFIED, constants.DataspaceProvider)

	strPID := staticProviderPID.String()
	_, err := env.server.ContractFinalize(ctx, &dsrpc.ContractFinalizeRequest{
		Pid: strPID,
	})

	assert.Nil(t, err)

	assert.NotNil(t, env.reconciler.e)
	pid := env.reconciler.e.EntityID
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
	assert.Equal(t, pid.URN(), reqPayload.ProviderPID)
	assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
	assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
	assert.Equal(t, contract.States.FINALIZED.String(), reqPayload.EventType)
}

func TestContractTerminate(t *testing.T) {
	t.Parallel()

	for _, role := range []constants.DataspaceRole{constants.DataspaceProvider, constants.DataspaceConsumer} {
		for _, state := range []contract.State{
			contract.States.REQUESTED,
			contract.States.OFFERED,
			contract.States.ACCEPTED,
			contract.States.AGREED,
			contract.States.VERIFIED,
		} {
			ctx, cancel, env := setupEnvironment(t)
			createNegotiation(ctx, t, env.store, state, role)

			curPID := staticProviderPID
			remPID := staticConsumerPID
			if role == constants.DataspaceConsumer {
				curPID = staticConsumerPID
				remPID = staticProviderPID
			}
			_, err := env.server.ContractTerminate(ctx, &dsrpc.ContractTerminateRequest{
				Pid:    curPID.String(),
				Code:   "test",
				Reason: []string{"test"},
			})

			assert.Nil(t, err)

			assert.NotNil(t, env.reconciler.e)
			assert.Equal(t, statemachine.ReconciliationContract, env.reconciler.e.Type)
			assert.Equal(t, role, env.reconciler.e.Role)
			assert.Equal(t, contract.States.TERMINATED.String(), env.reconciler.e.TargetState)
			assert.Equal(t, http.MethodPost, env.reconciler.e.Method)
			assert.Equal(
				t,
				mkRequestUrl(callBack, "negotiations", remPID.String(), "termination"),
				env.reconciler.e.URL.String(),
			)

			var reqPayload shared.ContractNegotiationTerminationMessage
			err = json.Unmarshal(env.reconciler.e.Body, &reqPayload)
			assert.Nil(t, err)
			assert.Equal(t, staticConsumerPID.URN(), reqPayload.ConsumerPID)
			assert.Equal(t, staticProviderPID.URN(), reqPayload.ProviderPID)
			assert.Equal(t, "test", reqPayload.Code)
			assert.Equal(t, "test", reqPayload.Reason[0].Value)
			cancel()
		}
	}
}
