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
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"go-dataspace.eu/ctxslog"
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
	Modified() bool
	ToBytes() ([]byte, error)
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
	opt.WithLogger(logAdaptor{ctxslog.Extract(ctx)})

	ctx = ctxslog.With(ctx,
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
	ctxslog.Info(sp.ctx, "Starting database maintenance loop")
	ticker := time.NewTicker(gcInterval)
	for {
		select {
		case <-ticker.C:
			ctxslog.Debug(sp.ctx, "Garbage collection starting")
			err := sp.db.RunValueLogGC(0.7)
			if err != nil && !errors.Is(err, badger.ErrGCInMemoryMode) {
				ctxslog.Error(sp.ctx, "GC not completed cleanly", "err", err)
			}
		case <-sp.ctx.Done():
			ticker.Stop()
			sp.db.Close()
			return
		}
	}
}

// getAll returns all values that match a prefix.
func getAll(db *badger.DB, prefix []byte) ([][]byte, error) {
	var values [][]byte
	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				values = append(values, v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return values, nil
}

// get is a generic function that gets the bytes from the database, decodes and returns it.
func get(db *badger.DB, key []byte) ([]byte, error) {
	var b []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			b = append([]byte{}, val...)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return b, err
}

// getLocked is a generic function that wraps get in a lock/unlock.
func getLocked(
	ctx context.Context,
	sp *StorageProvider,
	key []byte,
) ([]byte, error) {
	ctxslog.Debug(ctx, "Acquiring lock")
	if err := sp.AcquireLock(ctx, newLockKey(key)); err != nil {
		ctxslog.Err(ctx, "Could not acquire lock, panicking", err)
		panic("Failed to acquire lock")
	}
	ctxslog.Debug(ctx, "Lock acquired, fetching")
	b, err := get(sp.db, key)
	if err != nil {
		if lockErr := sp.ReleaseLock(ctx, newLockKey(key)); lockErr != nil {
			ctxslog.Err(ctx, "Failed to unlock, will have to depend on TTL", lockErr)
		}
		return nil, ctxslog.ReturnError(ctx, "Couldn't fetch from db", err)
	}
	return b, nil
}

func put(db *badger.DB, key []byte, value []byte) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

func putUnlock[T writeController](ctx context.Context, sp *StorageProvider, thing T) error {
	tType := fmt.Sprintf("%T", thing)
	key := thing.StorageKey()
	ctx = ctxslog.With(ctx, "type", tType, "key", string(key))
	if thing.ReadOnly() {
		ctxslog.Error(ctx, "Trying to write a read only entry")
		panic("Trying to write a read only entry")
	}
	if thing.Modified() {
		b, err := thing.ToBytes()
		if err != nil {
			return err
		}
		ctxslog.Debug(ctx, "Writing to store")
		if err := put(sp.db, key, b); err != nil {
			return ctxslog.ReturnError(ctx, "Could not save entry, not releasing lock", err)
		}
	}
	return sp.ReleaseLock(ctx, newLockKey(key))
}
