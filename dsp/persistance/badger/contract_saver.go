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

//nolint:dupl // Bare minimum of duplicated code
package badger

import (
	"context"
	"fmt"

	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

// GetContractR gets a contract and sets the read-only property.
// It does not check any locks, as the database transaction already freezes the view.
func (sp *StorageProvider) GetContractR(
	ctx context.Context,
	pid uuid.UUID,
	role statemachine.DataspaceRole,
) (*statemachine.Contract, error) {
	key := statemachine.MkContractKey(pid, role)
	logger := logging.Extract(ctx).With("pid", pid, "role", role, "key", string(key))
	contract, err := get[*statemachine.Contract](sp.db, key)
	if err != nil {
		logger.Error("Failed to get contract", "err", err)
		return nil, fmt.Errorf("could not get contract: %w", err)
	}
	contract.SetReadOnly()
	return contract, nil
}

// GetContractRW gets a contract but does NOT set the read-only property, allowing changes to be saved.
// It will try to acquire a lock, and if it can't it will panic. The panic will be replaced once
// RUN-DSP reaches beta, but right now we want contract problems to be extremely visible.
func (sp *StorageProvider) GetContractRW(
	ctx context.Context,
	pid uuid.UUID,
	role statemachine.DataspaceRole,
) (*statemachine.Contract, error) {
	key := statemachine.MkContractKey(pid, role)
	ctx, _ = logging.InjectLabels(ctx, "type", "contract", "pid", pid, "role", role, "key", string(key))
	return getLocked[*statemachine.Contract](ctx, sp, key)
}

// PutContract saves a contract to the database.
// If the contract is set to read-only, it will panic as this is a bug in the code.
// It will release the lock after it has saved.
func (sp *StorageProvider) PutContract(ctx context.Context, contract *statemachine.Contract) error {
	ctx, _ = logging.InjectLabels(
		ctx,
		"consumer_pid", contract.ConsumerPID,
		"provider_pid", contract.ProviderPID,
		"role", contract.Role,
	)
	return putUnlock(ctx, sp, contract)
}