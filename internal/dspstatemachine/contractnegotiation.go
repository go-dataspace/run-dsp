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
	CheckContractOffer(ctx context.Context, args ContractArgs) error
	SendContractAccepted(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractAgreement(ctx context.Context, args ContractArgs) error
	SendContractAgreementVerification(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error,
	)
	SendTerminationMessage(ctx context.Context) (ContractNegotiationMessageType, ContractNegotiationState, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

type providerContractTasksService interface {
	CheckContractRequest(ctx context.Context, args ContractArgs) error
	SendContractOffer(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractAccepted(ctx context.Context, args ContractArgs) error
	SendContractAgreement(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	CheckContractAgreementVerification(ctx context.Context, args ContractArgs)
	SendNegotiationFinalized(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendTerminationMessage(ctx context.Context) (ContractNegotiationMessageType, ContractNegotiationState, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}

func sendContractErrorMessage(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	err := args.consumerService.SendErrorMessage(ctx, args)
	if err != nil {
		return ContractArgs{}, nil, err
	}
	return ContractArgs{}, nil, nil
}

func terminateContractNegotiation(ctx context.Context, args ContractArgs) (
	ContractArgs, DSPState[ContractArgs], error,
) {
	logger := getLogger(ctx, args.BaseArgs)

	negotiationID := args.ProviderProcessId
	if args.ParticipantRole == Consumer {
		negotiationID = args.ConsumerProcessId
	}

	err := args.StateStorage.StoreState(ctx, negotiationID, Terminated)
	if err != nil {
		logger.Error("Failed to store state TERMINATED")
		return ContractArgs{}, nil, err
	}

	messageType, negotiationState, err := args.consumerService.SendTerminationMessage(ctx)
	if err != nil {
		logger.Error("Error sending termination message")
		return ContractArgs{}, nil, err
	}

	if messageType != ContractNegotiationMessage || negotiationState != Terminated {
		logger.Error("Unexpected response. Expected ContractNegotiationMessage and state TERMINATED")
		return ContractArgs{}, nil, newDSPContractError(42, "Unexpected response when trying to terminate negotiation.")
	}

	if args.StatusCode > 0 {
		logger.Info("Error message triggered by actual internal problem, returning exception")
		return ContractArgs{}, nil, newDSPContractError(args.StatusCode, args.ErrorMessage)
	}

	logger.Info("Terminated contract negotiation in state TERMINATED")

	return ContractArgs{}, nil, nil
}

func sendContractRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	negotiationState, err := args.StateStorage.FindState(ctx, args.ConsumerProcessId)
	if err != nil {
		logger.Error("Can't find initial state for contract negotiation")
		return ContractArgs{}, nil, err
	}
	if negotiationState != UndefinedState {
		return ContractArgs{}, nil, &DSPContractNegotiationError{
			42, errors.New("Initial contract negotiation state invalid, should be UndefinedState"),
		}
	}
	messageType, err := args.consumerService.SendContractRequest(ctx, args)
	if err != nil {
		return ContractArgs{}, nil, err
	}
	if messageType == ContractNegotiationError {
		logger.Error("Got error response when sending contract request")
		return ContractArgs{}, nil, newDSPContractError(42, "Remote returned error. Should set negotiation to failed?")
	}

	if !slices.Contains([]ContractNegotiationMessageType{ContractNegotiationMessage, ContractOfferMessage}, messageType) {
		logger.Error("Unexpected message type. Expected ContractOfferMessage or ContractNegotiationError.",
			"message_type", messageType)
		return ContractArgs{}, nil, newDSPContractError(42, "Unexpected message type received after sending contract request")
	}

	if messageType == ContractNegotiationMessage {
		logger.Info("Got contract negotiation message, assuming this is an asynchronous negotiation")
		err := args.StateStorage.StoreState(ctx, args.ConsumerProcessId, Requested)
		if err != nil {
			logger.Error("Failed to store state REQUESTED, this is bad and will probably leak transactions remotely")
			// TODO: Should we send a termination message here instead?
			args.StatusCode = 42
			args.ErrorMessage = "Failed to store REQUESTED state"
			args.Error = err
			//lint:ignore nilerr Error can be returned in sendContractErrorMessage if necessary
			return args, sendContractErrorMessage, nil
		}

		return ContractArgs{}, nil, nil
	}

	if messageType == ContractOfferMessage {
		logger.Info("Got contract offer message, this is a synchronous negotiation")
		err := args.StateStorage.StoreState(ctx, args.ConsumerProcessId, Offered)
		if err != nil {
			logger.Error("Failed to store state OFFERED, send ERROR response")
			// TODO: Should we send a termination message here instead?
			args.StatusCode = 42
			args.ErrorMessage = "Failed to store OFFERED state"
			args.Error = err
			//lint:ignore nilerr Error can be returned in sendContractErrorMessage if necessary
			return args, sendContractErrorMessage, nil
		}

		args.NegotiationState = Offered
		return args, sendContractAcceptedRequest, nil
	}

	return ContractArgs{}, nil, newDSPContractError(42, "Transaction failure. This point should not be reached")
}

func sendContractAcceptedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractAcceptedRequest")

	return ContractArgs{}, nil, fmt.Errorf("Transaction failure. This point should not be reached")
}

func sendContractAgreedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractAgreedRequest")
	return ContractArgs{}, nil, fmt.Errorf("Transaction failure. This point should not be reached")
}

func sendContractVerifiedRequest(ctx context.Context, args ContractArgs) (ContractArgs, DSPState[ContractArgs], error) {
	logger := getLogger(ctx, args.BaseArgs)
	logger.Debug("in sendContractVerifiedRequest")
	return ContractArgs{}, nil, fmt.Errorf("Transaction failure. This point should not be reached")
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
