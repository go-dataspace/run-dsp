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
package persistence

import (
	"context"

	"go-dataspace.eu/run-dsp/dsp/contract"
	agreementopts "go-dataspace.eu/run-dsp/dsp/persistence/options/agreement"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
	transferopts "go-dataspace.eu/run-dsp/dsp/persistence/options/transfer"
	"go-dataspace.eu/run-dsp/dsp/transfer"
	"go-dataspace.eu/run-dsp/odrl"
)

// StorageProvider is an interface that combines the *Saver interfaces.
type StorageProvider interface {
	// Migrate runs any pending migrations..
	Migrate() error
	// Close runs clean up
	Close() error
	ContractSaver
	AgreementSaver
	TransferSaver
	TokenSaver
}

// ContractSaver is an interface for storing/retrieving dataspace contracts.
// By default returned contract negotiations are readonly, they are set to read/write if the option is passed.
// Do note that in this implementation that read-only is enforced at save time, as all contract negotiation
// fields are public (for encoding purposes).
// It is up to the implementer to handle locking of contracts for the read/write instances,
// and to error if a read-only contract is being saved.
type ContractSaver interface {
	// GetContract gets a single contract negotiation, returning an error if not exactly one negotiation matches.
	GetContract(context.Context, ...contractopts.NegotiationOption) (*contract.Negotiation, error)
	// GetContract gets all matching contract negotiations, returns an empty slice if no negotiations matched.
	GetContracts(context.Context, ...contractopts.NegotiationOption) ([]*contract.Negotiation, error)
	// PutContract saves a contract, and releases the contract specific lock. If the contract
	// is read-only, it will return an error.
	PutContract(context.Context, *contract.Negotiation) error
	// ReleaseContract will release any lock the negotiation has
	ReleaseContract(context.Context, *contract.Negotiation) error
}

// AgreementSaver is an interface for storing/retrieving dataspace agreements.
// This does not have any locking involved as agreements are immutable.
type AgreementSaver interface {
	// GetAgreement gets an agreement based on the given options, returns an error if not exactly one found.
	GetAgreement(context.Context, ...agreementopts.Option) (*odrl.Agreement, error)
	// PutAgreement stores an agreement, but should return an error if the agreement ID already
	// exists.
	PutAgreement(context.Context, *odrl.Agreement) error
}

// TransferSaver is an interface for storing dataspace transfer request.
// The read/write semantics are the same as those for contracts.
type TransferSaver interface {
	// GetTransferR gets a read-only version of a transfer request.
	GetTransfer(context.Context, ...transferopts.RequestOption) (*transfer.Request, error)

	GetTransfers(context.Context, ...transferopts.RequestOption) ([]*transfer.Request, error)
	// PutTransfer saves a transfer.
	PutTransfer(context.Context, *transfer.Request) error
	// ReleaseTransfer will release any lock the transferhas
	ReleaseTransfer(context.Context, *transfer.Request) error
}

// TokenSaver saves a token to a key, no locking necessary as a token is immutable.
type TokenSaver interface {
	// GetToken retrieves a token by key.
	GetToken(context.Context, string) (string, error)
	// DelToken deletes a token by key.
	DelToken(context.Context, string) error
	// PutToken stores a key/token combination.
	PutToken(context.Context, string, string) error
}
