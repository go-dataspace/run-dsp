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

// Package dspstatemachine provides the state management for dataspace negotiations.
package dspstatemachine

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/google/uuid"
)

func ConsumerCheckContractOfferRequest(ctx context.Context, args ContractArgs) (shared.ContractNegotiation, error) {
	state, err := checkFindContractNegotiationState(
		ctx, args, args.BaseArgs.ConsumerProcessId, []ContractNegotiationState{UndefinedState})
	if err == nil {
		return shared.ContractNegotiation{}, errors.New("Found negotation state on contract offer received")
	}

	state.ParticipantRole = args.ParticipantRole
	// Might we already have a consumerPID here if it went the request/offer loop in the state diagram?
	state.ConsumerPID = state.StateID
	state.ProviderPID = args.ProviderProcessId
	state.ProviderCallbackAddress = args.BaseArgs.ProviderCallbackAddress
	state.State = Offered
	err = args.StateStorage.StoreContractNegotiationState(ctx, args.ConsumerProcessId, state)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Offered)
	}

	return shared.ContractNegotiation{
		Type:        "dspace:ContractNegotiation",
		ProviderPID: state.ProviderPID.String(),
		ConsumerPID: state.ConsumerPID.String(),
		State:       "dspace:REQUESTED",
	}, err
}

func ConsumerCheckContractAgreedRequest(
	ctx context.Context, args ContractArgs,
) (shared.ContractNegotiation, error) {
	state, err := checkFindContractNegotiationState(
		ctx, args, args.BaseArgs.ConsumerProcessId, []ContractNegotiationState{Requested})
	if err != nil {
		return shared.ContractNegotiation{}, err
	}

	if state.ProviderPID == uuid.Nil {
		state.ProviderPID = args.ProviderProcessId
	}
	state.State = Agreed
	state.Agreement = args.Agreement
	err = args.StateStorage.StoreContractNegotiationState(ctx, state.StateID, state)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Agreed)
	}
	return shared.ContractNegotiation{
		Type:        "dspace:ContractNegotiation",
		ProviderPID: state.ProviderPID.String(),
		ConsumerPID: state.ConsumerPID.String(),
		State:       "dspace:AGREED",
	}, err
}

func ConsumerCheckContractFinalizedRequest(
	ctx context.Context, args ContractArgs,
) (shared.ContractNegotiation, error) {
	state, err := checkFindContractNegotiationState(
		ctx, args, args.BaseArgs.ProviderProcessId, []ContractNegotiationState{Verified})
	if err != nil {
		return shared.ContractNegotiation{}, err
	}

	state.State = Finalized
	err = args.StateStorage.StoreContractNegotiationState(ctx, state.StateID, state)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Finalized)
	}

	err = args.StateStorage.StoreAgreement(ctx, state.Agreement.ID, state.Agreement)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Finalized)
	}
	return shared.ContractNegotiation{
		Type:        "dspace:ContractNegotiation",
		ProviderPID: state.ProviderPID.String(),
		ConsumerPID: state.ConsumerPID.String(),
		State:       "dspace:FINALIZED",
	}, err
}
