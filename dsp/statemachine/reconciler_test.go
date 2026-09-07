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

package statemachine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/logging"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

// newReconcilerTestStore returns a fresh store wired to a reconciler with a
// short lock timeout, so a test that expects a stuck lock fails fast instead
// of hanging for the production 60s default.
func newReconcilerTestStore(
	t *testing.T, requester *MockRequester,
) (context.Context, *sqlite.Provider, statemachine.Reconciler) {
	t.Helper()
	logger := logging.New("error", true)
	ctx := ctxslog.Inject(t.Context(), logger)

	store, err := sqlite.New(ctx, true, false, "")
	require.Nil(t, err)
	err = store.Migrate(ctx)
	require.Nil(t, err)
	store.SetLockTimeout(2)

	reconciler := statemachine.NewReconciler(ctx, requester, store)
	reconciler.Run()

	return ctx, store, reconciler
}

func testOffer() odrl.Offer {
	return odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 uuid.New().URN(),
			},
			Type:   "odrl:Offer",
			Target: uuid.New().URN(),
		},
	}
}

//nolint:funlen // Table of six scenarios sharing one runCase helper; splitting it up would obscure the table.
func TestReconcilerLearnsProviderPIDFromAck(t *testing.T) {
	runCase := func(t *testing.T, buildBody func(consumerPID, ackProviderPID uuid.UUID) []byte, wantBackfill bool) {
		t.Helper()
		ackProviderPID := uuid.New()
		cPID := uuid.New()
		requester := &MockRequester{Response: buildBody(cPID, ackProviderPID)}
		ctx, store, reconciler := newReconcilerTestStore(t, requester)

		negotiation := contract.New(
			ctx, uuid.UUID{}, cPID, contract.States.INITIAL, testOffer(),
			providerCallback, consumerCallback, constants.DataspaceConsumer, false,
			&dsrpc.RequesterInfo{AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN},
		)
		require.Nil(t, store.PutContract(ctx, negotiation))

		reconciler.Add(statemachine.ReconciliationEntry{
			EntityID:    cPID,
			Type:        statemachine.ReconciliationContract,
			Role:        constants.DataspaceConsumer,
			TargetState: contract.States.REQUESTED.String(),
			Method:      http.MethodPost,
			URL:         urlMustParse("https://provider.dsp/negotiations/request"),
			Body:        []byte(`{}`),
			Context:     ctx,
		})

		require.Eventually(t, func() bool {
			neg, err := store.GetContract(ctx, contractopts.WithRolePID(cPID, constants.DataspaceConsumer))
			return err == nil && neg.GetState() == contract.States.REQUESTED
		}, 5*time.Second, 50*time.Millisecond)

		neg, err := store.GetContract(ctx, contractopts.WithRolePID(cPID, constants.DataspaceConsumer))
		require.Nil(t, err)
		if wantBackfill {
			require.Equal(t, ackProviderPID, neg.GetProviderPID())
		} else {
			require.Equal(t, uuid.UUID{}, neg.GetProviderPID())
		}
	}

	t.Run("valid ack backfills provider PID", func(t *testing.T) {
		runCase(t, func(cPID, ackProviderPID uuid.UUID) []byte {
			b, err := json.Marshal(shared.ContractNegotiation{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:ContractNegotiation",
				ProviderPID: ackProviderPID.URN(),
				ConsumerPID: cPID.URN(),
				State:       contract.States.REQUESTED.String(),
			})
			require.Nil(t, err)
			return b
		}, true)
	})

	t.Run("nil body is ignored", func(t *testing.T) {
		runCase(t, func(uuid.UUID, uuid.UUID) []byte { return nil }, false)
	})

	t.Run("non-JSON body is ignored", func(t *testing.T) {
		runCase(t, func(uuid.UUID, uuid.UUID) []byte {
			return []byte("<html>502 Bad Gateway</html>")
		}, false)
	})

	t.Run("wrong message type is ignored", func(t *testing.T) {
		runCase(t, func(cPID, ackProviderPID uuid.UUID) []byte {
			b, err := json.Marshal(shared.ContractRequestMessage{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:ContractRequestMessage",
				ConsumerPID: cPID.URN(),
			})
			require.Nil(t, err)
			return b
		}, false)
	})

	t.Run("mismatched consumer PID is rejected", func(t *testing.T) {
		runCase(t, func(cPID, ackProviderPID uuid.UUID) []byte {
			b, err := json.Marshal(shared.ContractNegotiation{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:ContractNegotiation",
				ProviderPID: ackProviderPID.URN(),
				ConsumerPID: uuid.New().URN(), // unrelated to the negotiation under test
				State:       contract.States.REQUESTED.String(),
			})
			require.Nil(t, err)
			return b
		}, false)
	})

	t.Run("zero provider PID in ack is ignored", func(t *testing.T) {
		runCase(t, func(cPID, _ uuid.UUID) []byte {
			b, err := json.Marshal(shared.ContractNegotiation{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:ContractNegotiation",
				ProviderPID: uuid.UUID{}.URN(),
				ConsumerPID: cPID.URN(),
				State:       contract.States.REQUESTED.String(),
			})
			require.Nil(t, err)
			return b
		}, false)
	})
}

func TestReconcilerDoesNotOverwriteExistingProviderPID(t *testing.T) {
	trustedProviderPID := uuid.New()
	cPID := uuid.New()

	ackBody, err := json.Marshal(shared.ContractNegotiation{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: uuid.New().URN(), // a different PID than the one already stored
		ConsumerPID: cPID.URN(),
		State:       contract.States.AGREED.String(),
	})
	require.Nil(t, err)

	requester := &MockRequester{Response: ackBody}
	ctx, store, reconciler := newReconcilerTestStore(t, requester)

	negotiation := contract.New(
		ctx, trustedProviderPID, cPID, contract.States.REQUESTED, testOffer(),
		providerCallback, consumerCallback, constants.DataspaceConsumer, false,
		&dsrpc.RequesterInfo{AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN},
	)
	require.Nil(t, store.PutContract(ctx, negotiation))

	reconciler.Add(statemachine.ReconciliationEntry{
		EntityID:    cPID,
		Type:        statemachine.ReconciliationContract,
		Role:        constants.DataspaceConsumer,
		TargetState: contract.States.AGREED.String(),
		Method:      http.MethodPost,
		URL:         urlMustParse("https://provider.dsp/negotiations/" + trustedProviderPID.String() + "/agreement"),
		Body:        []byte(`{}`),
		Context:     ctx,
	})

	require.Eventually(t, func() bool {
		neg, err := store.GetContract(ctx, contractopts.WithRolePID(cPID, constants.DataspaceConsumer))
		return err == nil && neg.GetState() == contract.States.AGREED
	}, 5*time.Second, 50*time.Millisecond)

	neg, err := store.GetContract(ctx, contractopts.WithRolePID(cPID, constants.DataspaceConsumer))
	require.Nil(t, err)
	require.Equal(t, trustedProviderPID, neg.GetProviderPID())
}

func TestReconcilerLearnsConsumerPIDFromAck(t *testing.T) {
	pPID := uuid.New()
	ackConsumerPID := uuid.New()

	ackBody, err := json.Marshal(shared.ContractNegotiation{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: pPID.URN(),
		ConsumerPID: ackConsumerPID.URN(),
		State:       contract.States.OFFERED.String(),
	})
	require.Nil(t, err)

	requester := &MockRequester{Response: ackBody}
	ctx, store, reconciler := newReconcilerTestStore(t, requester)

	negotiation := contract.New(
		ctx, pPID, uuid.UUID{}, contract.States.INITIAL, testOffer(),
		consumerCallback, providerCallback, constants.DataspaceProvider, false,
		&dsrpc.RequesterInfo{AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN},
	)
	require.Nil(t, store.PutContract(ctx, negotiation))

	reconciler.Add(statemachine.ReconciliationEntry{
		EntityID:    pPID,
		Type:        statemachine.ReconciliationContract,
		Role:        constants.DataspaceProvider,
		TargetState: contract.States.OFFERED.String(),
		Method:      http.MethodPost,
		URL:         urlMustParse("https://consumer.dsp/negotiations/offers"),
		Body:        []byte(`{}`),
		Context:     ctx,
	})

	require.Eventually(t, func() bool {
		neg, err := store.GetContract(ctx, contractopts.WithRolePID(pPID, constants.DataspaceProvider))
		return err == nil && neg.GetState() == contract.States.OFFERED
	}, 5*time.Second, 50*time.Millisecond)

	neg, err := store.GetContract(ctx, contractopts.WithRolePID(pPID, constants.DataspaceProvider))
	require.Nil(t, err)
	require.Equal(t, ackConsumerPID, neg.GetConsumerPID())
}

func TestReconcilerReleasesLockOnInvalidTransition(t *testing.T) {
	requester := &MockRequester{}
	ctx, store, reconciler := newReconcilerTestStore(t, requester)

	cPID := uuid.New()
	negotiation := contract.New(
		ctx, uuid.New(), cPID, contract.States.AGREED, testOffer(),
		providerCallback, consumerCallback, constants.DataspaceConsumer, false,
		&dsrpc.RequesterInfo{AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN},
	)
	require.Nil(t, store.PutContract(ctx, negotiation))

	// AGREED -> REQUESTED is not a valid transition (see validTransitions in
	// dsp/contract/negotiation.go), so getStateUpdater must fail here before
	// ever calling SendHTTPRequest.
	reconciler.Add(statemachine.ReconciliationEntry{
		EntityID:    cPID,
		Type:        statemachine.ReconciliationContract,
		Role:        constants.DataspaceConsumer,
		TargetState: contract.States.REQUESTED.String(),
		Method:      http.MethodPost,
		URL:         urlMustParse("https://provider.dsp/negotiations/request"),
		Body:        []byte(`{}`),
		Context:     ctx,
	})

	time.Sleep(500 * time.Millisecond)

	neg, err := store.GetContract(ctx, contractopts.WithRW(), contractopts.WithRolePID(cPID, constants.DataspaceConsumer))
	require.Nilf(t, err, "row lock was not released after the invalid transition (getStateUpdater leaked it): %v", err)
	require.Nil(t, store.ReleaseContract(ctx, neg))

	require.Nil(t, requester.ReceivedURL, "the entry must fail before any HTTP send is attempted")
}
