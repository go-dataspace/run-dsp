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

package dspstatemachine

import (
	"context"

	"github.com/google/uuid"
)

var contractStates map[uuid.UUID]DSPContractStateStorage

type DSPContractStateStorage struct {
	StateID         uuid.UUID
	State           ContractNegotiationState
	CallbackAddress string
	ParticipantRole DSPParticipantRole
}

type SimpleStateStorage struct{}

func (s *SimpleStateStorage) FindContractNegotiationState(
	ctx context.Context, id uuid.UUID,
) (DSPContractStateStorage, error) {
	state, ok := contractStates[id]
	if !ok {
		return DSPContractStateStorage{StateID: uuid.New(), State: UndefinedState}, nil
	}
	return state, nil
}

func (s *SimpleStateStorage) StoreContractNegotiationState(
	ctx context.Context, id uuid.UUID, negotationState DSPContractStateStorage,
) error {
	contractStates[id] = negotationState
	return nil
}
