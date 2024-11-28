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

	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

func mkAgreementKey(id string) []byte {
	return []byte("odrl-agreement-" + id)
}

// GetAgreement gets an agreement by ID.
func (sp *StorageProvider) GetAgreement(
	ctx context.Context,
	id uuid.UUID,
) (*odrl.Agreement, error) {
	key := mkAgreementKey(id.String())
	return get[*odrl.Agreement](sp.db, key)
}

// PutAgreement stores an agreement, but should return an error if the agreement ID already
// exists.
func (sp *StorageProvider) PutAgreement(ctx context.Context, agreement *odrl.Agreement) error {
	id, err := uuid.Parse(agreement.ID)
	if err != nil {
		return err
	}
	key := mkAgreementKey(id.String())
	return put(sp.db, key, agreement)
}
