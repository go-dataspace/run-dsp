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

//nolint:unused
type consumerContractTasksService interface {
	SendContractRequest(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendContractAccepted(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendContractAgreementVerification(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error,
	)
	SendContractNegotiationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendTerminationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

// func sendContractRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
// 	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{UndefinedState})
// 	if err != nil {
// 		return ContractArgs{}, nil, err
// 	}

// 	messageType, err := args.consumerService.SendContractRequest(ctx, args)
// 	return checkMessageTypeAndStoreState(
// 		ctx,
// 		args,
// 		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractOfferMessage},
// 		messageType,
// 		err,
// 		Requested,
// 		Offered,
// 		sendContractAcceptedRequest)
// }

func ConsumerCheckContractOfferRequest(ctx context.Context, args ContractArgs) (shared.ContractNegotiation, error) {
	// check contract offer request
	// if asynchronous -> send ack
	// if valid and synchronous -> return send contract accepted
	// if rejected -> return send contract termination
	// logger := getLogger(ctx, args.BaseArgs)
	state, err := checkFindNegotiationState(
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

// func sendContractAcceptedRequest(
// ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
// 	logger := getLogger(ctx, args.BaseArgs)
// 	logger.Debug("in sendContractAcceptedRequest")

// 	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Offered})
// 	if err != nil {
// 		return ContractArgs{}, nil, err
// 	}

// 	messageType, err := args.consumerService.SendContractAccepted(ctx, args)
// 	return checkMessageTypeAndStoreState(
// 		ctx,
// 		args,
// 		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractAgreementMessage},
// 		messageType,
// 		err,
// 		Accepted,
// 		Agreed,
// 		sendContractVerifiedRequest)
// }

func ConsumerCheckContractAgreedRequest(
	ctx context.Context, args ContractArgs,
) (shared.ContractNegotiation, error) {
	// check contract agreed request
	// if asynchronous -> send ack
	// if valid and synchronous -> return send contract agreement verification
	// if rejected -> return send contract termination
	// return checkContractNegotiationRequest(
	// 	ctx,
	// 	args,
	// 	ContractAgreementMessage,
	// 	[]ContractNegotiationState{Requested, Accepted},
	// 	Agreed, false, sendContractVerifiedRequest,
	// )
	state, err := checkFindNegotiationState(
		ctx, args, args.BaseArgs.ConsumerProcessId, []ContractNegotiationState{Requested})
	if err != nil {
		return shared.ContractNegotiation{}, err
	}

	if state.ProviderPID == uuid.Nil {
		state.ProviderPID = args.ProviderProcessId
	}
	state.State = Agreed
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
	// check contract finalized request
	// if valid, always send ack
	// if error, send error

	state, err := checkFindNegotiationState(
		ctx, args, args.BaseArgs.ProviderProcessId, []ContractNegotiationState{Verified})
	if err != nil {
		return shared.ContractNegotiation{}, err
	}

	state.State = Finalized
	err = args.StateStorage.StoreContractNegotiationState(ctx, state.StateID, state)
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

// func sendContractVerifiedRequest(
// ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
// 	logger := getLogger(ctx, args.BaseArgs)
// 	logger.Debug("in sendContractVerifiedRequest")

// 	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Agreed})
// 	if err != nil {
// 		return ContractArgs{}, nil, err
// 	}

// 	messageType, err := args.consumerService.SendContractAgreementVerification(ctx, args)
// 	return checkMessageTypeAndStoreState(
// 		ctx,
// 		args,
// 		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractNegotiationEventMessage},
// 		messageType,
// 		err,
// 		Verified,
// 		Finalized,
// 		nil)
// }
