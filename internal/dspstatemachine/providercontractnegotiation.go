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
)

//nolint:dupl
type providerContractTasksService interface {
	SendContractOffer(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractRequest(ctx context.Context, args ContractArgs) (bool, error)
	CheckContractAcceptedMessage(ctx context.Context, args ContractArgs) (bool, error)
	SendContractAgreement(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractAgreementVerification(ctx context.Context, args ContractArgs) (bool, error)
	SendNegotiationFinalized(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendContractNegotiationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendTerminationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

//nolint:unused
func sendContractOfferRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{UndefinedState})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	messageType, err := args.providerService.SendContractOffer(ctx, args)
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractOfferMessage},
		messageType,
		err,
		Offered,
		Offered,
		sendContractAcceptedRequest)
}

//nolint:unused
func checkContractRequestMessage(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	// NOTE: Going directly from REQUESTED to AGREED
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractRequestMessage,
		[]ContractNegotiationState{Requested},
		Agreed, false, sendContractAgreedRequest,
	)
}

//nolint:unused
func checkContractAcceptedMessage(
	ctx context.Context, args ContractArgs,
) (ContractArgs, DSPState[ContractArgs], error) {
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractNegotiationEventMessage,
		[]ContractNegotiationState{Offered},
		Agreed, false, sendContractAgreedRequest,
	)
}

//nolint:unused
func sendContractAgreedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractAgreedRequest")
	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Requested})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	messageType, err := args.providerService.SendContractAgreement(ctx, args)
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage, ContractAgreementMessage},
		messageType,
		err,
		Agreed,
		Agreed,
		sendContractAcceptedRequest)
}

//nolint:unused
func sendContractFinalizedRequest(
	ctx context.Context, args ContractArgs,
) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractFinalizedRequest")
	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{Verified})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	messageType, err := args.providerService.SendNegotiationFinalized(ctx, args)
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage},
		messageType,
		err,
		Finalized,
		Finalized,
		nil)
}

//nolint:unused
func checkContractAgreeementVerificationMessage(
	ctx context.Context, args ContractArgs,
) (ContractArgs, DSPState[ContractArgs], error) {
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractAgreementVerificationMessage,
		[]ContractNegotiationState{Agreed},
		Verified, false, sendContractFinalizedRequest,
	)
}
