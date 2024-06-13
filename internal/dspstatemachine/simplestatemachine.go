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
	"sync"
	"time"

	"github.com/go-dataspace/run-dsp/internal/auth"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

var (
	contractStates = make(map[uuid.UUID]DSPContractStateStorage)
	mutex          = &sync.RWMutex{}
)

type DSPContractStateStorage struct {
	StateID                 uuid.UUID
	ProviderPID             uuid.UUID
	ConsumerPID             uuid.UUID
	State                   ContractNegotiationState
	ConsumerCallbackAddress string
	ProviderCallbackAddress string
	ParticipantRole         DSPParticipantRole
}

type SimpleStateStorage struct {
	ctx context.Context
}

func GetStateStorage(ctx context.Context) *SimpleStateStorage {
	return &SimpleStateStorage{
		ctx: ctx,
	}
}

func (s *SimpleStateStorage) FindContractNegotiationState(
	ctx context.Context, id uuid.UUID,
) (DSPContractStateStorage, error) {
	mutex.Lock()
	state, ok := contractStates[id]
	mutex.Unlock()
	if !ok {
		return DSPContractStateStorage{
			StateID: uuid.New(), State: UndefinedState, ProviderCallbackAddress: "http://localhost:8080",
		}, nil
	}
	return state, nil
}

func (s *SimpleStateStorage) StoreContractNegotiationState(
	ctx context.Context, id uuid.UUID, negotationState DSPContractStateStorage,
) error {
	mutex.RLock()
	contractStates[id] = negotationState
	mutex.RUnlock()
	return nil
}

func (s *SimpleStateStorage) AllowStateToProgress(idChan chan StateStorageChannelMessage) {
	// get state of thing
	// check that it seems ok
	// trigger callback function to send out request
	logger := logging.Extract(s.ctx)
	logger.Info("Started go routine for state storage")

	for {
		select {
		case message := <-idChan:
			logger = logging.Extract(message.Context)
			logger.Info("Got UUID of state to release...",
				"uuid", message.ProcessID, "auth", auth.ExtractUserInfo(message.Context))
			mutex.Lock()
			state, ok := contractStates[message.ProcessID]
			mutex.Unlock()
			if !ok {
				logger.Error("Could not find state for processId", "process_id", message.ProcessID)
				continue
			}

			logger.Info("Delaying triggering next state for one second")
			time.Sleep(time.Second)

			var err error
			if state.ParticipantRole == Consumer {
				err = s.triggerNextConsumerState(message.Context, state)
			} else {
				err = s.triggerNextProviderState(message.Context, state)
			}
			if err != nil {
				logger.Error("Failed to trigger next state for processId", "process_id", message.ProcessID)
			}
		}
	}
}

func (s *SimpleStateStorage) triggerNextConsumerState(ctx context.Context, contractState DSPContractStateStorage) error {
	logger := logging.Extract(ctx)
	logger.Info("In SimpleStorage.triggerNextConsumerState")
	var err error
	switch contractState.State {
	case Requested:
	case UndefinedState:
	case Offered:
	case Accepted:
	case Agreed:
	case Verified:
	case Finalized:
	case Terminated:
	}
	return err
}

func (s *SimpleStateStorage) triggerNextProviderState(ctx context.Context, contractState DSPContractStateStorage) error {
	httpService := getHttpContractService(ctx, contractState)
	var err error
	switch contractState.State {
	case Requested:
		contractState.State = Agreed
		err = httpService.SendContractAgreement(ctx)
	case UndefinedState:
	case Offered:
	case Accepted:
	case Agreed:
	case Verified:
	case Finalized:
	case Terminated:
	}

	if err == nil {
		err = s.StoreContractNegotiationState(ctx, contractState.StateID, contractState)
	}

	return err
}
