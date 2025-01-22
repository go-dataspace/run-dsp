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

	"github.com/dgraph-io/badger/v4"
)

func mkTokenKey(id string) []byte {
	return []byte("token-" + id)
}

// GetToken gets a token by key.
func (sp *StorageProvider) GetToken(ctx context.Context, key string) (string, error) {
	bkey := mkTokenKey(key)
	b, err := get(sp.db, bkey)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Deltoken deletes a token by key.
func (sp *StorageProvider) DelToken(ctx context.Context, key string) error {
	bkey := mkTokenKey(key)
	return sp.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(bkey)
	})
}

// PutToken stores a key/token combination, but should return an error if the key already
// exists.
func (sp *StorageProvider) PutToken(ctx context.Context, key, token string) error {
	bkey := mkTokenKey(key)
	return put(sp.db, bkey, []byte(token))
}
