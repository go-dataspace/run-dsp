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

// GetTransferR gets a transfer and sets the read-only property.
// It does not check any locks, as the database transaction already freezes the view.
func (sp *StorageProvider) GetTransferR(
	ctx context.Context,
	pid uuid.UUID,
	role statemachine.DataspaceRole,
) (*statemachine.TransferRequest, error) {
	key := statemachine.MkTransferKey(pid, role)
	logger := logging.Extract(ctx).With("pid", pid, "role", role, "key", string(key))
	transfer, err := get[*statemachine.TransferRequest](sp.db, key)
	if err != nil {
		logger.Error("Failed to get transfer", "err", err)
		return nil, fmt.Errorf("could not get transfer %w", err)
	}
	transfer.SetReadOnly()
	return transfer, nil
}

// GetTransferRW gets a transfer but does NOT set the read-only property, allowing changes to be saved.
// It will try to acquire a lock, and if it can't it will panic. The panic will be replaced once
// RUN-DSP reaches beta, but right now we want transfer problems to be extremely visible.
func (sp *StorageProvider) GetTransferRW(
	ctx context.Context,
	pid uuid.UUID,
	role statemachine.DataspaceRole,
) (*statemachine.TransferRequest, error) {
	key := statemachine.MkTransferKey(pid, role)
	ctx, _ = logging.InjectLabels(ctx, "type", "transfer", "pid", pid, "role", role, "key", string(key))
	return getLocked[*statemachine.TransferRequest](ctx, sp, key)
}

// PutTransfer saves a transfer to the database.
// If the transfer is set to read-only, it will panic as this is a bug in the code.
// It will release the lock after it has saved.
func (sp *StorageProvider) PutTransfer(ctx context.Context, transfer *statemachine.TransferRequest) error {
	ctx, _ = logging.InjectLabels(
		ctx,
		"consumer_pid", transfer.ConsumerPID,
		"provider_pid", transfer.ProviderPID,
		"role", transfer.Role,
	)
	return putUnlock(ctx, sp, transfer)
}
