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

package dspstatemachine

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

type TransferNegotiationState int64

const (
	UndefinedTransferState TransferNegotiationState = iota
	TransferRequested
	TransferStarted
	TransferSuspended
	TransferCompleted
	TransferTerminated
)

func (s TransferNegotiationState) String() string {
	switch s {
	case UndefinedTransferState:
		return "UndefinedTransferState"
	case TransferRequested:
		return "REQUESTED"
	case TransferStarted:
		return "STARTED"
	case TransferSuspended:
		return "SUSPENDED"
	case TransferCompleted:
		return "COMPLETED"
	case TransferTerminated:
		return "TERMINATED"
	}
	return "unknown"
}

type TransferArgs struct {
	BaseArgs
	TransferState TransferNegotiationState
	StateStorage  DSPStateStorageService
	AgreementID   string
}

type DSPTransferNegotiationError struct {
	StatusCode int
	Err        error
}

func (err *DSPTransferNegotiationError) Error() string {
	return fmt.Sprintf("status %d: err %v", err.StatusCode, err.Err)
}

func checkFindTransferNegotiationState(
	ctx context.Context, args TransferArgs, processID uuid.UUID, expectedStates []TransferNegotiationState,
) (DSPTransferStateStorage, error) {
	logger := getLogger(ctx, args.BaseArgs)

	negotiationState, err := args.StateStorage.FindTransferNegotiationState(ctx, processID)
	if err != nil {
		logger.Error("Could not find state for contract negotiation", "uuid", processID)
		return DSPTransferStateStorage{}, err
	}
	if !slices.Contains(expectedStates, negotiationState.State) {
		return DSPTransferStateStorage{}, &DSPTransferNegotiationError{
			42, fmt.Errorf("Contract negotiation state invalid. Got %s, expected %s", negotiationState.State, expectedStates),
		}
	}

	return negotiationState, nil
}
