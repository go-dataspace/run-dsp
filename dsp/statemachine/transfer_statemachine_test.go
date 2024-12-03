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
	"testing"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/constants"
	"github.com/go-dataspace/run-dsp/dsp/persistence/badger"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/dsp/transfer"
	"github.com/go-dataspace/run-dsp/logging"
	mockprovider "github.com/go-dataspace/run-dsp/mocks/github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var agreementID = uuid.MustParse("e1c68180-de68-428d-9853-7d4dd3c66904")

func TestTransferTermination(t *testing.T) {
	t.Parallel()

	agreement := odrl.Agreement{
		PolicyClass: odrl.PolicyClass{},
		ID:          agreementID.URN(),
		Timestamp:   time.Time{},
	}
	logger := logging.NewJSON("error", true)
	ctx := logging.Inject(context.Background(), logger)
	ctx, done := context.WithCancel(ctx)
	defer done()

	store, err := badger.New(ctx, true, "")
	assert.Nil(t, err)
	requester := &MockRequester{}

	mockProvider := mockprovider.NewMockProviderServiceClient(t)
	err = store.PutAgreement(ctx, &agreement)
	assert.Nil(t, err)

	reconciler := statemachine.NewReconciler(ctx, requester, store)
	reconciler.Run()

	for _, role := range []constants.DataspaceRole{
		constants.DataspaceConsumer,
		constants.DataspaceProvider,
	} {
		for _, state := range []transfer.State{
			transfer.States.REQUESTED,
			transfer.States.STARTED,
		} {
			transReq := transfer.New(
				consumerPID, &agreement,
				"HTTP_PULL",
				providerCallback, consumerCallback,
				role,
				state,
				nil,
			)
			pState := statemachine.GetTransferRequestNegotiation(transReq, mockProvider, reconciler)
			pState.GetTransferRequest().SetProviderPID(providerPID)

			transferMsg := shared.TransferTerminationMessage{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:TransferTerminationMessage",
				ProviderPID: providerPID.URN(),
				ConsumerPID: consumerPID.URN(),
				Code:        "meh",
				Reason:      []map[string]any{},
			}

			next, err := pState.Recv(ctx, transferMsg)
			assert.IsType(t, &statemachine.TransferRequestNegotiationTerminated{}, next)
			assert.Nil(t, err)
			_, err = next.Send(ctx)
			assert.Nil(t, err)
			assert.Equal(t, transfer.States.TERMINATED, next.GetTransferRequest().GetState())
		}
	}
}
