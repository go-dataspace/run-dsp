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
)

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

func checkContractRequestMessage(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	// NOTE: Going directly from
	return checkContractNegotiationRequest(
		ctx,
		args,
		ContractRequestMessage,
		[]ContractNegotiationState{UndefinedState},
		Agreed, false, sendContractAgreedRequest,
	)
}

func sendContractAgreedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractAgreedRequest")
	return ContractArgs{}, nil, fmt.Errorf("Transaction failure. This point should not be reached")
}
