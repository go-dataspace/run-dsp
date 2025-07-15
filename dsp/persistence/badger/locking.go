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
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
	"go-dataspace.eu/ctxslog"
)

const (
	lockTTL       = 15 * time.Minute
	maxWaitTime   = 10 * time.Minute
	lockCheckTime = 10 * time.Millisecond

	logKey = "lock_key"
)

type lockKey struct {
	k []byte
}

func newLockKey(key []byte) lockKey {
	return lockKey{
		k: append([]byte("lock-"), key...),
	}
}

func (l lockKey) key() []byte {
	return l.k
}

func (l lockKey) String() string {
	return string(l.k)
}

func (sp *StorageProvider) AcquireLock(ctx context.Context, k lockKey) error {
	err := sp.waitLock(ctx, k)
	if err != nil {
		return err
	}
	return sp.setLock(ctx, k)
}

func (sp *StorageProvider) ReleaseLock(ctx context.Context, k lockKey) error {
	ctx = ctxslog.With(ctx, logKey, k.String())
	return sp.db.Update(func(txn *badger.Txn) error {
		ctxslog.Debug(ctx, "Attempting to release lock")
		err := txn.Delete(k.key())
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				// No lock found is essentially released, this will most likely only happen on
				// first time saves.
				return nil
			}
			ctxslog.Err(ctx, "Failed to unlock, will have to depend on TTL", err)
		}
		return err
	})
}

func (sp *StorageProvider) isLocked(ctx context.Context, k lockKey) bool {
	err := sp.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(k.key())
		return err
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return false
		}
		ctxslog.Error(ctx, "Got an error, reporting locked", err)
		return true
	}
	return true
}

func (sp *StorageProvider) setLock(ctx context.Context, k lockKey) error {
	err := sp.db.Update(func(txn *badger.Txn) error {
		ctxslog.Debug(ctx, "Setting lock")
		entry := badger.NewEntry(k.key(), []byte{1}).WithTTL(lockTTL)
		return txn.SetEntry(entry)
	})
	if err != nil {
		return ctxslog.ReturnError(ctx, "Couldn't set lock", err)
	}
	ctxslog.Debug(ctx, "Lock set")
	return nil
}

func (sp *StorageProvider) waitLock(ctx context.Context, k lockKey) error {
	ticker := time.NewTicker(lockCheckTime)
	defer ticker.Stop()
	timer := time.NewTicker(maxWaitTime)
	defer timer.Stop()
	ctxslog.Debug(ctx, "Starting to wait for lock")
	for {
		select {
		case <-ticker.C:
			if sp.isLocked(ctx, k) {
				continue
			}
			return nil
		case <-timer.C:
			return ctxslog.ReturnError(ctx, "Lock timeout reached", errors.New("timed out waiting for lock"))
		case <-ctx.Done():
			return ctxslog.ReturnError(ctx, "Shutting down waiting for lock", errors.New("context cancelled"))
		}
	}
}
