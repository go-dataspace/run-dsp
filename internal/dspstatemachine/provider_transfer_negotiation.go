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

package dspstatemachine

import (
	"context"
	"fmt"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
)

func ProviderCheckTransferRequestMessage(ctx context.Context, args TransferArgs) (shared.TransferProcess, error) {
	newState := DSPTransferStateStorage{
		StateID:                 args.ProviderProcessId,
		ProviderPID:             args.ProviderProcessId,
		ConsumerPID:             args.ConsumerProcessId,
		State:                   TransferRequested,
		ConsumerCallbackAddress: args.ConsumerCallbackAddress,
		ProviderCallbackAddress: "http://localhost:8080/run-dsp/v2024-1",
		ParticipantRole:         Provider,
		AgreementID:             args.AgreementID,
		PublishInfo:             args.PublishInfo,
	}
	err := args.StateStorage.StoreTransferNegotiationState(ctx, newState.StateID, newState)
	if err != nil {
		return shared.TransferProcess{}, fmt.Errorf("Failed to store %s state", TransferRequested)
	}

	return shared.TransferProcess{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:TransferProcess",
		ProviderPID: newState.ProviderPID.String(),
		ConsumerPID: newState.ConsumerPID.String(),
		State:       "dspace:REQUESTED",
	}, err
}

func ProviderCheckTransferCompletionMessage(ctx context.Context, args TransferArgs) (shared.TransferProcess, error) {
	state, err := checkFindTransferNegotiationState(
		ctx, args, args.BaseArgs.ProviderProcessId, []TransferNegotiationState{TransferStarted})
	if err != nil {
		return shared.TransferProcess{}, err
	}

	state.State = TransferCompleted
	err = args.StateStorage.StoreTransferNegotiationState(ctx, state.StateID, state)
	if err != nil {
		return shared.TransferProcess{}, fmt.Errorf("Failed to store %s state", TransferCompleted)
	}

	return shared.TransferProcess{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:TransferProcess",
		ProviderPID: state.ProviderPID.String(),
		ConsumerPID: state.ConsumerPID.String(),
		State:       "dspace:COMPLETED",
	}, err
}
