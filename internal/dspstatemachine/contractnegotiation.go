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

	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
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

	// Backend service for storing / retrieving transaction state
	StateStorage DSPStateStorageService

	Offer     odrl.MessageOffer
	Agreement odrl.Agreement
}

type DSPContractNegotiationError struct {
	StatusCode int
	Err        error
}

func (err *DSPContractNegotiationError) Error() string {
	return fmt.Sprintf("status %d: err %v", err.StatusCode, err.Err)
}

func newDSPContractError(status int, message string) error {
	return &DSPContractNegotiationError{
		status, errors.New(message),
	}
}

func checkFindContractNegotiationState(
	ctx context.Context, args ContractArgs, processID uuid.UUID, expectedStates []ContractNegotiationState,
) (DSPContractStateStorage, error) {
	logger := getLogger(ctx, args.BaseArgs)

	negotiationState, err := args.StateStorage.FindContractNegotiationState(ctx, processID)
	if err != nil {
		logger.Error("Could not find state for contract negotiation", "uuid", processID)
		return DSPContractStateStorage{}, err
	}
	if !slices.Contains(expectedStates, negotiationState.State) {
		return DSPContractStateStorage{}, newDSPContractError(
			42,
			fmt.Sprintf("Contract negotiation state invalid. Got %s, expected %s", negotiationState.State, expectedStates),
		)
	}

	return negotiationState, nil
}
