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
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-dataspace/run-dsp/logging"
)

const (
	gcInterval = 5 * time.Minute
)

type StorageProvider struct {
	ctx context.Context
	db  *badger.DB
}

type storageKeyGenerator interface {
	StorageKey() []byte
}

type writeController interface {
	SetReadOnly()
	ReadOnly() bool
	storageKeyGenerator
}

// New returns a new badger storage provider, using an inMemory setup if the boolean is set,
// or it will create/reuse the badger database located in dbPath.
func New(ctx context.Context, inMemory bool, dbPath string) (*StorageProvider, error) {
	var opt badger.Options
	var dbType string
	if inMemory {
		opt = badger.DefaultOptions("").WithInMemory(inMemory)
		dbType = "memory"
	} else {
		opt = badger.DefaultOptions(dbPath)
		dbType = "disk"
	}
	ctx, _ = logging.InjectLabels(ctx,
		"module", "badger",
		"db_type", dbType,
		"db_path", dbPath,
	)
	db, err := badger.Open(opt)
	if err != nil {
		return nil, err
	}
	sp := &StorageProvider{
		ctx: ctx,
		db:  db,
	}
	go sp.maintenance()
	return sp, nil
}

// maintenance is a goroutine that runs the badger garbage collection every gcInterval.
func (sp StorageProvider) maintenance() {
	logger := logging.Extract(sp.ctx)
	logger.Info("Starting database maintenance loop")
	ticker := time.NewTicker(gcInterval)
	for {
		select {
		case <-ticker.C:
			logger.Info("Garbage collection starting")
			err := sp.db.RunValueLogGC(0.7)
			if err != nil {
				logger.Error("GC not completed cleanly", "err", err)
			}
		case <-sp.ctx.Done():
			ticker.Stop()
			sp.db.Close()
			return
		}
	}
}

// get is a generic function that gets the bytes from the database, decodes and returns it.
func get[T any](db *badger.DB, key []byte) (T, error) {
	var thing T
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			dec := gob.NewDecoder(bytes.NewReader(val))
			return dec.Decode(thing)
		})
	})
	return thing, err
}

// getLocked is a generic function that wraps get in a lock/unlock.
func getLocked[T writeController](
	ctx context.Context,
	sp *StorageProvider,
	key []byte,
) (T, error) {
	logger := logging.Extract(ctx)
	logger.Info("Acquiring lock")
	if err := sp.AcquireLock(ctx, newLockKey(key)); err != nil {
		logger.Error("Could not acquire lock, panicking", "err", err)
		panic("Failed to acquire lock")
	}
	logger.Info("Lock acquired, fetching")
	thing, err := get[T](sp.db, key)
	if err != nil {
		logger.Error("Couldn't fetch from db, unlocking", "err", err)
		if lockErr := sp.ReleaseLock(ctx, newLockKey(key)); lockErr != nil {
			logger.Error("Failed to unlock, will have to depend on TTL", "err", lockErr)
		}
		var n T
		return n, fmt.Errorf("failed to fetch from db")
	}
	return thing, nil
}

func put[T any](db *badger.DB, key []byte, thing T) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(thing)
	if err != nil {
		return fmt.Errorf("could not encode in gob: %w", err)
	}
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, buf.Bytes())
	})
}

func putUnlock[T writeController](ctx context.Context, sp *StorageProvider, thing T) error {
	tType := fmt.Sprintf("%T", thing)
	logger := logging.Extract(ctx).With("type", tType)
	if thing.ReadOnly() {
		logger.Error("Trying to write a read only entry", "type", tType)
		panic("Trying to write a read only entry")
	}
	key := thing.StorageKey()
	if err := put(sp.db, key, thing); err != nil {
		logger.Error("Could not save entry, not releasing lock", "err", err)
		return err
	}

	return sp.ReleaseLock(ctx, newLockKey(key))
}
