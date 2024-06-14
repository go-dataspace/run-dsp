// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
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

//nolint:dupl
type providerContractTasksService interface {
	SendContractOffer(ctx context.Context) error
	SendContractAgreement(ctx context.Context) error
	SendNegotiationFinalized(ctx context.Context) error
	SendContractNegotiationMessage(ctx context.Context) error
	SendTerminationMessage(ctx context.Context) error
	SendErrorMessage(ctx context.Context) error
}

// //nolint:unused
// func sendContractOfferRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
// 	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{UndefinedState})
// 	if err != nil {
// 		return ContractArgs{}, nil, err
// 	}

// 	messageType, err := args.providerService.SendContractOffer(ctx, args)
// 	return checkMessageTypeAndStoreState(
// 		ctx,
// 		args,
// 		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractOfferMessage},
// 		messageType,
// 		err,
// 		Offered,
// 		Offered,
// 		sendContractAcceptedRequest)
// }

func ProviderCheckContractRequestMessage(ctx context.Context, args ContractArgs) (shared.ContractNegotiation, error) {
	// NOTE: Going directly from REQUESTED to AGREED
	newState := DSPContractStateStorage{
		StateID:                 args.ProviderProcessId,
		ProviderPID:             args.ProviderProcessId,
		ConsumerPID:             args.ConsumerProcessId,
		State:                   Requested,
		ConsumerCallbackAddress: args.ConsumerCallbackAddress,
		ProviderCallbackAddress: "http://localhost:8080",
		ParticipantRole:         Provider,
	}
	err := args.StateStorage.StoreContractNegotiationState(ctx, newState.StateID, newState)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Requested)
	}

	return shared.ContractNegotiation{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: newState.ProviderPID.String(),
		ConsumerPID: newState.ConsumerPID.String(),
		State:       "dspace:REQUESTED",
	}, err
}

// //nolint:unused
func ProviderCheckContractAcceptedMessage(
	ctx context.Context, args ContractArgs,
) (shared.ContractNegotiation, error) {
	state, err := checkFindNegotiationState(ctx, args, args.ProviderProcessId, []ContractNegotiationState{Offered})
	if err != nil {
		return shared.ContractNegotiation{}, err
	}

	state.ConsumerPID = args.ConsumerProcessId
	state.ProviderPID = state.StateID
	state.State = Accepted
	err = args.StateStorage.StoreContractNegotiationState(ctx, args.ProviderProcessId, state)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Accepted)
	}

	return shared.ContractNegotiation{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: state.ProviderPID.String(),
		ConsumerPID: state.ConsumerPID.String(),
		State:       "dspace:ACCEPTED",
	}, err
}

// //nolint:unused
// func sendContractAgreedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
// 	logger := getLogger(ctx, args.BaseArgs)
// 	logger.Debug("in sendContractAgreedRequest")
// 	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Requested})
// 	if err != nil {
// 		return ContractArgs{}, nil, err
// 	}

// 	messageType, err := args.providerService.SendContractAgreement(ctx, args)
// 	return checkMessageTypeAndStoreState(
// 		ctx,
// 		args,
// 		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractAgreementMessage},
// 		messageType,
// 		err,
// 		Agreed,
// 		Agreed,
// 		sendContractAcceptedRequest)
// }

// //nolint:unused
// func sendContractFinalizedRequest(
// 	ctx context.Context, args ContractArgs,
// ) (ContractArgs, DSPState[ContractArgs], error) {
// 	logger := getLogger(ctx, args.BaseArgs)
// 	logger.Debug("in sendContractFinalizedRequest")
// 	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Verified})
// 	if err != nil {
// 		return ContractArgs{}, nil, err
// 	}

// 	messageType, err := args.providerService.SendNegotiationFinalized(ctx, args)
// 	return checkMessageTypeAndStoreState(
// 		ctx,
// 		args,
// 		[]ContractNegotiationMessageType{ContractNegotiationMessage},
// 		messageType,
// 		err,
// 		Finalized,
// 		Finalized,
// 		nil)
// }

func ProviderCheckContractAgreementVerificationMessage(
	ctx context.Context, args ContractArgs,
) (shared.ContractNegotiation, error) {
	state, err := checkFindNegotiationState(ctx, args, args.ProviderProcessId, []ContractNegotiationState{Agreed})
	if err != nil {
		return shared.ContractNegotiation{}, err
	}

	state.State = Verified
	err = args.StateStorage.StoreContractNegotiationState(ctx, args.ProviderProcessId, state)
	if err != nil {
		return shared.ContractNegotiation{}, fmt.Errorf("Failed to store %s state", Verified)
	}

	return shared.ContractNegotiation{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: state.ProviderPID.String(),
		ConsumerPID: state.ConsumerPID.String(),
		State:       "dspace:VERIFIED",
	}, err
}
