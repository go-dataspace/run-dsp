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

// Package persistence contains the storage interfaces for the dataspace code. It also contains
// constants and other shared code for the implementation packages.
package persistance

import (
	"context"

	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

// StorageProvider is an interface that combines the *Saver interfaces.
type StorageProvider interface {
	ContractSaver
	AgreementSaver
	TransferSaver
}

// ContractSaver is an interface for storing/retrieving dataspace contracts.
// It supports both read-only and read/write versions.
// Do note that in this implementation that read-only is enforced at save time, as all contract
// fields are public (for encoding purposes).
// It is up to the implementer to handle locking of contracts for the read/write instances,
// and to error if a read-only contract is being saved.
type ContractSaver interface {
	// GetContractR gets a read-only version of a contract.
	GetContractR(
		ctx context.Context,
		pid uuid.UUID,
		role statemachine.DataspaceRole,
	) (*statemachine.Contract, error)
	// GetContractRW gets a read/write version of a contract. This should set a contract specific
	// lock for the requested contract.
	GetContractRW(
		ctx context.Context,
		pid uuid.UUID,
		role statemachine.DataspaceRole,
	) (*statemachine.Contract, error)
	// PutContract saves a contract, and releases the contract specific lock. If the contract
	// is read-only, it will return an error.
	PutContract(ctx context.Context, contract *statemachine.Contract) error
}

// AgreementSaver is an interface for storing/retrieving dataspace agreements.
// This does not have any locking involved as agreements are immutable.
type AgreementSaver interface {
	// GetAgreement gets an agreement by ID.
	GetAgreement(ctx context.Context, id uuid.UUID) (*odrl.Agreement, error)
	// PutAgreement stores an agreement, but should return an error if the agreement ID already
	// exists.
	PutAgreement(ctx context.Context, agreement *odrl.Agreement) error
}

// TransferSaver is an interface for storing dataspace transfer request.
// The read/write semantics are the same as those for contracts.
type TransferSaver interface {
	// GetTransferR gets a read-only version of a transfer request.
	GetTransferR(
		ctx context.Context,
		pid uuid.UUID,
		role statemachine.DataspaceRole,
	) (*statemachine.TransferRequest, error)
	// GetTransferRW gets a read/write version of a transfer request.
	GetTransferRW(
		ctx context.Context,
		pid uuid.UUID,
		role statemachine.DataspaceRole,
	) (*statemachine.TransferRequest, error)
	// PutTransfer saves a transfer.
	PutTransfer(ctx context.Context, transfer *statemachine.TransferRequest) error
}
