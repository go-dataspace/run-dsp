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
	"slices"
)

type ContractNegotiationMessageType int64

const (
	UndefinedMessage ContractNegotiationMessageType = iota
	ContractRequestMessage
	ContractOfferMessage
	ContractAgreementMessage
	ContractAgreementVerificationMessage
	ContractNegotiationEventMessage
	ContractNegotiationTerminationMessage
	ContractNegotiationMessage
	ContractNegotiationError
)

func (s ContractNegotiationMessageType) String() string {
	switch s {
	case UndefinedMessage:
		return "UndefinedMessage"
	case ContractRequestMessage:
		return "ContractRequestMessage"
	case ContractOfferMessage:
		return "ContractOfferMessage"
	case ContractAgreementMessage:
		return "ContractAgreementMessage"
	case ContractAgreementVerificationMessage:
		return "ContractAgreementVerificationMessage"
	case ContractNegotiationEventMessage:
		return "ContractNegotiationEventMessage"
	case ContractNegotiationTerminationMessage:
		return "ContractNegotiationTerminationMessage"
	case ContractNegotiationMessage:
		return "ContractNegotiationMessage"
	case ContractNegotiationError:
		return "ContractNegotiationError"
	}
	return "unknown"
}

type ContractNegotiationState int64

const (
	UndefinedState ContractNegotiationState = iota
	Requested
	Offered
	Accepted
	Agreed
	Verified
	Finalized
	Terminated
)

func (s ContractNegotiationState) String() string {
	switch s {
	case UndefinedState:
		return "UndefinedState"
	case Requested:
		return "REQUESTED"
	case Offered:
		return "OFFERED"
	case Accepted:
		return "ACCEPTED"
	case Agreed:
		return "AGREED"
	case Verified:
		return "VERIFIED"
	case Finalized:
		return "FINALIZED"
	case Terminated:
		return "TERMINATED"
	}
	return "unknown"
}

type ContractArgs struct {
	BaseArgs

	// ContractNegotiationstate
	NegotiationState ContractNegotiationState

	// ContractMessageType
	MessageType ContractNegotiationMessageType

	// Backend service for storing / retrieving transaction state
	StateStorage DSPStateStorageService

	// Consumer contract managing service
	consumerService consumerContractTasksService

	// Provider contract managing service
	providerService providerContractTasksService
}

type DSPContractNegotiationError struct {
	StatusCode int
	Err        error
}

func newDSPContractError(status int, message string) error {
	return &DSPContractNegotiationError{
		status, errors.New(message),
	}
}

func (err *DSPContractNegotiationError) Error() string {
	return fmt.Sprintf("status %d: err %v", err.StatusCode, err.Err)
}

type consumerContractTasksService interface {
	SendContractRequest(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendContractAccepted(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendContractAgreementVerification(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error,
	)
	SendTerminationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

type providerContractTasksService interface {
	SendContractOffer(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendContractAgreement(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendNegotiationFinalized(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendTerminationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

func checkFindNegotiationState(
	ctx context.Context, args ContractArgs, expectedStates []ContractNegotiationState,
) error {
	logger := getLogger(ctx, args.BaseArgs)

	processId := args.ConsumerProcessId
	if args.ParticipantRole == Provider {
		processId = args.ProviderProcessId
	}
	negotiationState, err := args.StateStorage.FindState(ctx, processId)
	if err != nil {
		logger.Error("Could not find state for contract negotiation", "uuid", processId)
		return err
	}
	if !slices.Contains(expectedStates, negotiationState) {
		return &DSPContractNegotiationError{
			42, fmt.Errorf("Contract negotiation state invalid. Got %s, expected %s", negotiationState, expectedStates),
		}
	}

	return nil
}

func checkMessageTypeAndStoreState(
	ctx context.Context,
	args ContractArgs,
	expectedMessageTypes []ContractNegotiationMessageType,
	messageType ContractNegotiationMessageType,
	err error,
	asyncState ContractNegotiationState,
	syncState ContractNegotiationState,
	nextState DSPState[ContractArgs],
) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	if err != nil {
		return ContractArgs{}, nil, err
	}
	if messageType == ContractNegotiationError {
		logger.Error("Got error response when sending contract request")
		return ContractArgs{}, nil, newDSPContractError(42, "Remote returned error. Should set negotiation to failed?")
	}
	if !slices.Contains(expectedMessageTypes, messageType) {
		// FIXME: Replace error message
		logger.Error("Unexpected message type. Expected ContractOfferMessage or ContractNegotiationError.",
			"message_type", messageType)
		return ContractArgs{}, nil, newDSPContractError(42, "Unexpected message type received after sending contract request")
	}

	processId := args.ConsumerProcessId
	if args.ParticipantRole == Provider {
		processId = args.ProviderProcessId
	}

	if messageType == ContractNegotiationMessage {
		logger.Info("Got contract negotiation message, assuming this is an asynchronous negotiation")
		err := args.StateStorage.StoreState(ctx, args.ConsumerProcessId, asyncState)
		if err != nil {
			msg := fmt.Sprintf("Failed to store state %s, this is bad and will probably leak transactions remotely", asyncState)
			logger.Error(msg)
			args.StatusCode = 42
			args.ErrorMessage = fmt.Sprintf("Failed to store %s state", asyncState)
			args.Error = err
			//lint:ignore nilerr Error can be returned in sendContractErrorMessage if necessary
			return args, sendContractErrorMessage, nil
		}

		return ContractArgs{}, nil, nil
	}

	logger.Info("Got other message than ACK, this is a synchronous negotiation")
	err = args.StateStorage.StoreState(ctx, processId, syncState)
	if err != nil {
		msg := fmt.Sprintf("Failed to store state %s, send ERROR response", syncState)
		logger.Error(msg)
		args.StatusCode = 42
		args.ErrorMessage = fmt.Sprintf("Failed to store %s state", syncState)
		args.Error = err
		// lint:ignore nilerr Error can be returned in sendContractErrorMessage if necessary
		return args, sendContractErrorMessage, nil
	}

	return args, nextState, nil
}

func sendContractErrorMessage(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	err := args.consumerService.SendErrorMessage(ctx, args)
	if err != nil {
		return ContractArgs{}, nil, err
	}
	return ContractArgs{}, nil, nil
}

func sendTerminateContractNegotiation(ctx context.Context, args ContractArgs) (
	ContractArgs, DSPState[ContractArgs], error,
) {
	err := checkFindNegotiationState(ctx, args, []ContractNegotiationState{
		Requested,
		Offered,
		Accepted,
		Agreed,
		Verified,
	})
	if err != nil {
		return ContractArgs{}, nil, err
	}

	var messageType ContractNegotiationMessageType
	if args.BaseArgs.ParticipantRole == Consumer {
		messageType, err = args.consumerService.SendTerminationMessage(ctx, args)
	} else {
		messageType, err = args.providerService.SendTerminationMessage(ctx, args)
	}
	return checkMessageTypeAndStoreState(
		ctx,
		args,
		[]ContractNegotiationMessageType{ContractNegotiationMessage},
		messageType,
		err,
		Terminated,
		Terminated,
		nil)
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

func sendContractFinalizedRequest(ctx context.Context, args ContractArgs) (
	ContractArgs, DSPState[ContractArgs], error,
) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractFinalizedRequest")
	return ContractArgs{}, nil, fmt.Errorf("Transaction failure. This point should not be reached")
}

func (a ContractArgs) validate() error {
	if !slices.Contains(participantRoles, a.ParticipantRole) {
		return fmt.Errorf("Invalid partipant role %d", a.ParticipantRole)
	}

	if !slices.Contains(transactionTypes, a.TransactionType) {
		return fmt.Errorf("Invalid transaction role %d", a.TransactionType)
	}

	if a.StateStorage == nil {
		return fmt.Errorf("State storage cannot be nil")
	}

	return nil
}
