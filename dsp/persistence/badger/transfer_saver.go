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
	"go-dataspace.eu/run-dsp/dsp/transfer"
)

// GetTransfers returns all transfers.
func (sp *StorageProvider) GetTransfers(ctx context.Context) ([]*transfer.Request, error) {
	transfers := []*transfer.Request{}

	rawTransfers, err := getAll(sp.db, []byte(transfer.TransferPrefix))
	if err != nil {
		return nil, err
	}

	for _, t := range rawTransfers {
		request, err := transfer.FromBytes(t)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, request)
	}

	return transfers, nil
}

// GetTransferR gets a transfer and sets the read-only property.
// It does not check any locks, as the database transaction already freezes the view.
func (sp *StorageProvider) GetTransferR(
	ctx context.Context,
	pid uuid.UUID,
	role constants.DataspaceRole,
) (*transfer.Request, error) {
	key := transfer.GenerateKey(pid, role)
	ctx = ctxslog.With(ctx, "pid", pid, "role", role, "key", string(key))
	b, err := get(sp.db, key)
	if err != nil {
		return nil, ctxslog.ReturnError(ctx, "Failed to get transfer", err)
	}
	request, err := transfer.FromBytes(b)
	if err != nil {
		return nil, err
	}

	request.SetReadOnly()
	return request, nil
}

// GetTransferRW gets a transfer but does NOT set the read-only property, allowing changes to be saved.
// It will try to acquire a lock, and if it can't it will panic. The panic will be replaced once
// RUN-DSP reaches beta, but right now we want transfer problems to be extremely visible.
func (sp *StorageProvider) GetTransferRW(
	ctx context.Context,
	pid uuid.UUID,
	role constants.DataspaceRole,
) (*transfer.Request, error) {
	key := transfer.GenerateKey(pid, role)
	ctx = ctxslog.With(ctx, "type", "transfer", "pid", pid, "role", role, "key", string(key))
	ctxslog.Debug(ctx, "Attempting to read transfer from store")
	b, err := getLocked(ctx, sp, key)
	if err != nil {
		return nil, err
	}
	request, err := transfer.FromBytes(b)
	if err != nil {
		_ = sp.ReleaseLock(ctx, newLockKey(key))
		return nil, err
	}

	return request, nil
}

// PutTransfer saves a transfer to the database.
// If the transfer is set to read-only, it will panic as this is a bug in the code.
// It will release the lock after it has saved.
func (sp *StorageProvider) PutTransfer(ctx context.Context, transfer *transfer.Request) error {
	return putUnlock(ctx, sp, transfer)
}

func (sp *StorageProvider) ReleaseTransfer(
	ctx context.Context,
	t *transfer.Request,
) error {
	key := transfer.GenerateKey(t.GetLocalPID(), t.GetRole())
	t.SetReadOnly()
	return sp.ReleaseLock(ctx, newLockKey(key))
}
