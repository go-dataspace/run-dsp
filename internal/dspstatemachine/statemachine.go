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
	"log/slog"

	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

type DSPParticipantRole int64

const (
	Consumer DSPParticipantRole = iota
	Provider
)

type DSPTransactionType int64

const (
	Contract DSPTransactionType = iota
	Transfer
)

type BaseArgs struct {
	// ConsumerProcessId
	ConsumerProcessId uuid.UUID

	// ProviderProcessIdContractNegotiationState
	ProviderProcessId uuid.UUID

	// Role in DSP interaction
	ParticipantRole DSPParticipantRole

	// Type of DSP transaction
	TransactionType DSPTransactionType

	// Callback urls for request
	ConsumerCallbackAddress string
	ProviderCallbackAddress string

	// Error information
	StatusCode   int
	ErrorMessage string
	Error        error
}

type (
	DSPState[T any]        func(ctx context.Context, args T) (T, DSPState[T], error)
	DSPStateStorageService interface {
		FindContractNegotiationState(ctx context.Context, id uuid.UUID) (DSPContractStateStorage, error)
		StoreContractNegotiationState(ctx context.Context, id uuid.UUID, negotationState DSPContractStateStorage) error
		FindTransferNegotiationState(ctx context.Context, id uuid.UUID) (DSPTransferStateStorage, error)
		StoreTransferNegotiationState(ctx context.Context, id uuid.UUID, negotationState DSPTransferStateStorage) error
		FindAgreement(ctx context.Context, id string) (odrl.Agreement, error)
		StoreAgreement(ctx context.Context, id string, agrement odrl.Agreement) error
	}
)

func getLogger(ctx context.Context, args BaseArgs) *slog.Logger {
	return logging.Extract(ctx).With(
		"consumer_pid", args.ConsumerProcessId,
	).With("provider_pid", args.ProviderProcessId)
}

func Run[T any](ctx context.Context, args T, start DSPState[T]) (T, error) {
	var err error
	current := start
	for {
		if ctx.Err() != nil {
			return args, ctx.Err()
		}
		args, current, err = current(ctx, args)
		if err != nil {
			return args, err
		}
		if current == nil {
			return args, nil
		}
	}
}
