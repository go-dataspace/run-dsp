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

package server

import (
	"context"
	"fmt"

	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite"
)

func (c *command) getStorageProvider(ctx context.Context) (persistence.StorageProvider, error) {
	var provider persistence.StorageProvider
	var err error
	switch c.PersistenceBackend {
	case "sqlite":
		// TODO: make debug configurable.
		provider, err = sqlite.New(ctx, c.SQLiteMemoryDB, false, c.SQLiteDBPath)
	default:
		return nil, fmt.Errorf("invalid backend: %s", c.PersistenceBackend)
	}
	if err != nil {
		return nil, err
	}
	if err := provider.Migrate(ctx); err != nil {
		return nil, err
	}
	return provider, nil
}
