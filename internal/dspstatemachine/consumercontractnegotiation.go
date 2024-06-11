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
	"fmt"
)

type consumerContractTasksService interface {
	SendContractRequest(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractOffer(ctx context.Context, args ContractArgs) (bool, error)
	SendContractAccepted(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractAgreed(ctx context.Context, args ContractArgs) (bool, error)
	SendContractAgreementVerification(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error,
	)
	CheckContractFinalized(ctx context.Context, args ContractArgs) (bool, error)
	SendContractNegotiationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendTerminationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

func sendContractRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{UndefinedState})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	messageType, err := args.consumerService.SendContractRequest(ctx, args)
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractOfferMessage},
		messageType,
		err,
		Requested,
		Offered,
		sendContractAcceptedRequest)
}

func checkContractOfferRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	// check contract offer request
	// if asynchronous -> send ack
	// if valid and synchronous -> return send contract accepted
	// if rejected -> return send contract termination
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractOfferMessage,
		[]ContractNegotiationState{Requested},
		Offered, false, sendContractAcceptedRequest,
	)
}

func sendContractAcceptedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractAcceptedRequest")

	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Offered})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	messageType, err := args.consumerService.SendContractAccepted(ctx, args)
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractAgreementMessage},
		messageType,
		err,
		Accepted,
		Agreed,
		sendContractAgreedRequest)
}

func checkContractAgreedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	// check contract agreed request
	// if asynchronous -> send ack
	// if valid and synchronous -> return send contract agreement verification
	// if rejected -> return send contract termination
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractAgreementMessage,
		[]ContractNegotiationState{Accepted},
		Agreed, false, sendContractAgreedRequest,
	)
}

func checkContractFinalizedRequest(
	ctx context.Context, args ContractArgs,
) (ContractArgs, DSPState[ContractArgs], error) {
	// check contract finalized request
	// if valid, always send ack
	// if error, send error
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractNegotiationEventMessage,
		[]ContractNegotiationState{Verified},
		Finalized, true, nil,
	)
}

func sendContractAgreedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractAgreedRequest")
	return ContractArgs{}, nil, fmt.Errorf("Transaction failure. This point should not be reached")
}

func sendContractVerifiedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractVerifiedRequest")

	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Agreed})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	messageType, err := args.consumerService.SendContractAgreementVerification(ctx, args)
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractNegotiationEventMessage},
		messageType,
		err,
		Verified,
		Finalized,
		nil)
}
