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

package statemachine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Archiver interface {
	GetProviderContract(ctx context.Context, providerPID uuid.UUID) (*Contract, error)
	PutProviderContract(ctx context.Context, contract *Contract) error
	GetConsumerContract(ctx context.Context, consumerPID uuid.UUID) (*Contract, error)
	PutConsumerContract(ctx context.Context, contract *Contract) error

	GetAgreement(ctx context.Context, agreementID uuid.UUID) (*odrl.Agreement, error)
	DelAgreement(ctx context.Context, agreementID uuid.UUID) error
	PutAgreement(ctx context.Context, agreement *odrl.Agreement) error

	GetProviderTransfer(ctx context.Context, providerPID uuid.UUID) (*TransferRequest, error)
	PutProviderTransfer(ctx context.Context, contract *TransferRequest) error
	GetConsumerTransfer(ctx context.Context, consumerPID uuid.UUID) (*TransferRequest, error)
	PutConsumerTransfer(ctx context.Context, contract *TransferRequest) error
}

type MemoryArchiver struct {
	providerContracts map[uuid.UUID]*Contract
	consumerContracts map[uuid.UUID]*Contract
	agreements        map[uuid.UUID]*odrl.Agreement
	providerTransfers map[uuid.UUID]*TransferRequest
	consumerTransfers map[uuid.UUID]*TransferRequest
	sync.RWMutex
}

func NewMemoryArchiver() *MemoryArchiver {
	return &MemoryArchiver{
		providerContracts: make(map[uuid.UUID]*Contract),
		consumerContracts: make(map[uuid.UUID]*Contract),
		agreements:        make(map[uuid.UUID]*odrl.Agreement),
		providerTransfers: make(map[uuid.UUID]*TransferRequest),
		consumerTransfers: make(map[uuid.UUID]*TransferRequest),
	}
}

func (ma *MemoryArchiver) PutProviderContract(ctx context.Context, contract *Contract) error {
	return ma.putContract(contract.GetProviderPID(), contract, ma.providerContracts)
}

func (ma *MemoryArchiver) PutConsumerContract(ctx context.Context, contract *Contract) error {
	return ma.putContract(contract.GetConsumerPID(), contract, ma.consumerContracts)
}

func (ma *MemoryArchiver) putContract(pid uuid.UUID, contract *Contract, contracts map[uuid.UUID]*Contract) error {
	defer ma.Unlock()
	ma.Lock()
	contracts[pid] = contract
	return nil
}

func (ma *MemoryArchiver) GetProviderContract(ctx context.Context, pid uuid.UUID) (*Contract, error) {
	return ma.getContract(pid, ma.providerContracts)
}

func (ma *MemoryArchiver) GetConsumerContract(ctx context.Context, pid uuid.UUID) (*Contract, error) {
	return ma.getContract(pid, ma.consumerContracts)
}

func (ma *MemoryArchiver) getContract(pid uuid.UUID, contracts map[uuid.UUID]*Contract) (*Contract, error) {
	defer ma.RUnlock()
	ma.RLock()
	c, ok := contracts[pid]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (ma *MemoryArchiver) PutProviderTransfer(ctx context.Context, transfer *TransferRequest) error {
	return ma.putTransfer(transfer.GetProviderPID(), transfer, ma.providerTransfers)
}

func (ma *MemoryArchiver) PutConsumerTransfer(ctx context.Context, transfer *TransferRequest) error {
	return ma.putTransfer(transfer.GetConsumerPID(), transfer, ma.consumerTransfers)
}

func (ma *MemoryArchiver) putTransfer(
	pid uuid.UUID, transfer *TransferRequest, transfers map[uuid.UUID]*TransferRequest,
) error {
	defer ma.Unlock()
	ma.Lock()
	transfers[pid] = transfer
	return nil
}

func (ma *MemoryArchiver) GetProviderTransfer(ctx context.Context, pid uuid.UUID) (*TransferRequest, error) {
	return ma.getTransfer(pid, ma.providerTransfers)
}

func (ma *MemoryArchiver) GetConsumerTransfer(ctx context.Context, pid uuid.UUID) (*TransferRequest, error) {
	return ma.getTransfer(pid, ma.consumerTransfers)
}

func (ma *MemoryArchiver) getTransfer(
	pid uuid.UUID,
	transfers map[uuid.UUID]*TransferRequest,
) (*TransferRequest, error) {
	defer ma.RUnlock()
	ma.RLock()
	c, ok := transfers[pid]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (ma *MemoryArchiver) GetAgreement(ctx context.Context, agreementID uuid.UUID) (*odrl.Agreement, error) {
	defer ma.RUnlock()
	ma.RLock()
	a, ok := ma.agreements[agreementID]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}

func (ma *MemoryArchiver) DelAgreement(ctx context.Context, agreementID uuid.UUID) error {
	delete(ma.agreements, agreementID)
	return nil
}

func (ma *MemoryArchiver) PutAgreement(ctx context.Context, agreement *odrl.Agreement) error {
	u, err := uuid.Parse(agreement.ID)
	if err != nil {
		return fmt.Errorf("not a valid agreement ID: %w", err)
	}
	ma.agreements[u] = agreement
	return nil
}
