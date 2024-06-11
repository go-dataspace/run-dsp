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
		logger.Info("Got contract negotiation message, assuming this is an asynchronous negotiation or a response")
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
	var err error

	if args.BaseArgs.ParticipantRole == Consumer {
		err = args.consumerService.SendErrorMessage(ctx, args)
	} else {
		err = args.providerService.SendErrorMessage(ctx, args)
	}
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

//nolint:cyclop,funlen
func checkContractNegotiationRequest(
	ctx context.Context,
	args ContractArgs,
	messageType ContractNegotiationMessageType,
	expectedNegotiationStates []ContractNegotiationState,
	futureNegotationState ContractNegotiationState,
	finalizedRequest bool,
	nextState DSPState[ContractArgs],
) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	err := checkFindNegotiationState(ctx, args, expectedNegotiationStates)
	if err != nil {
		return ContractArgs{}, nil, err
	}

	var messageAccepted bool
	switch messageType {
	case ContractOfferMessage:
		messageAccepted, err = args.consumerService.CheckContractOffer(ctx, args)
	case ContractAgreementMessage:
		messageAccepted, err = args.consumerService.CheckContractAgreed(ctx, args)
	case ContractNegotiationEventMessage:
		if args.BaseArgs.ParticipantRole == Consumer {
			messageAccepted, err = args.consumerService.CheckContractFinalized(ctx, args)
		} else {
			return ContractArgs{}, nil, newDSPContractError(42, "Unexpected message type received when trying to respond")
		}
	case UndefinedMessage:
	case ContractRequestMessage:
	case ContractAgreementVerificationMessage:
	case ContractNegotiationTerminationMessage:
	case ContractNegotiationMessage:
	case ContractNegotiationError:
	default:
		return ContractArgs{}, nil, newDSPContractError(42, "Unexpected message type received when trying to respond")
	}
	if err != nil {
		return ContractArgs{}, nil, err
	}

	err = args.StateStorage.StoreState(ctx, args.ConsumerProcessId, futureNegotationState)
	if err != nil {
		msg := fmt.Sprintf(
			"Failed to store state %s, this is bad and will probably leak transactions remotely",
			futureNegotationState,
		)
		logger.Error(msg)
		args.StatusCode = 42
		args.ErrorMessage = fmt.Sprintf("Failed to store %s state", futureNegotationState)
		args.Error = err

		//lint:ignore nilerr Error can be returned in sendContractErrorMessage if necessary
		return args, sendContractErrorMessage, nil
	}
	if !messageAccepted {
		nextState = sendTerminateContractNegotiation
	}
	if args.AsynchronousCommunication || finalizedRequest {
		msgType, err := args.consumerService.SendContractNegotiationMessage(ctx, args)
		_, _, err = checkMessageTypeAndStoreState(ctx, args, []ContractNegotiationMessageType{ContractNegotiationMessage},
			msgType, err, Agreed, Agreed, nil)
		if err != nil {
			return ContractArgs{}, nil, err
		}
	}

	return args, nextState, nil
}
