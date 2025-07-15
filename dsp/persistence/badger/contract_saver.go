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

package badger

import (
	"context"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
)

// GetContractR gets a contract and sets the read-only property.
// It does not check any locks, as the database transaction already freezes the view.
func (sp *StorageProvider) GetContractR(
	ctx context.Context,
	pid uuid.UUID,
	role constants.DataspaceRole,
) (*contract.Negotiation, error) {
	key := contract.GenerateStorageKey(pid, role)
	ctx = ctxslog.With(ctx, "pid", pid, "role", role, "key", string(key))
	b, err := get(sp.db, key)
	if err != nil {
		return nil, ctxslog.ReturnError(ctx, "could not get contract", err)
	}
	negotiation, err := contract.FromBytes(b)
	if err != nil {
		return nil, err
	}

	negotiation.SetReadOnly()
	return negotiation, nil
}

// GetContractRW gets a contract but does NOT set the read-only property, allowing changes to be saved.
// It will try to acquire a lock, and if it can't it will panic. The panic will be replaced once
// RUN-DSP reaches beta, but right now we want contract problems to be extremely visible.
func (sp *StorageProvider) GetContractRW(
	ctx context.Context,
	pid uuid.UUID,
	role constants.DataspaceRole,
) (*contract.Negotiation, error) {
	key := contract.GenerateStorageKey(pid, role)
	ctx = ctxslog.With(ctx, "type", "contract", "pid", pid, "role", role, "key", string(key))
	b, err := getLocked(ctx, sp, key)
	if err != nil {
		return nil, err
	}

	negotiation, err := contract.FromBytes(b)
	if err != nil {
		_ = sp.ReleaseLock(ctx, newLockKey(key))
		return nil, err
	}

	return negotiation, nil
}

// PutContract saves a contract to the database.
// If the contract is set to read-only, it will panic as this is a bug in the code.
// It will release the lock after it has saved.
func (sp *StorageProvider) PutContract(ctx context.Context, negotiation *contract.Negotiation) error {
	return putUnlock(ctx, sp, negotiation)
}

func (sp *StorageProvider) ReleaseContract(
	ctx context.Context,
	negotiation *contract.Negotiation,
) error {
	key := contract.GenerateStorageKey(negotiation.GetLocalPID(), negotiation.GetRole())

	negotiation.SetReadOnly()
	return sp.ReleaseLock(ctx, newLockKey(key))
}
