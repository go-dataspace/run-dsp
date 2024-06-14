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
	"errors"
	"fmt"
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

// type contractStateStore struct {
// 	contractState map[uuid.UUIDs]DSPContractStateStorage
// 	sync.RWMutex
// }

// func (c contractStateStore) Get(u uuid.UUID) DSPContractStateStorage {
// 	defer c.Unlock()
// 	c.Lock()

// 	return c.contractState[u]
// }

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
	logger := logging.Extract(ctx)
	logger.Debug("STATESTORAGE: Trying to get contract state", "state_id", id)
	mutex.Lock()
	state, ok := contractStates[id]
	mutex.Unlock()
	if !ok {
		logger.Debug("STATESTORAGE: Could not find state, returning error")
		return DSPContractStateStorage{}, errors.New("Could not find negotiation state")
	}
	return state, nil
}

func (s *SimpleStateStorage) StoreContractNegotiationState(
	ctx context.Context, id uuid.UUID, negotationState DSPContractStateStorage,
) error {
	logger := logging.Extract(ctx)
	logger.Debug("STATESTORAGE: Storing contract state", "state_id", id, "state_storage", negotationState)
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

			if state.ParticipantRole == Consumer {
				go s.triggerNextConsumerState(message.Context, state)
			} else {
				go s.triggerNextProviderState(message.Context, state)
			}
		}
	}
}

func (s *SimpleStateStorage) checkErrorAndStoreState(
	ctx context.Context, contractState DSPContractStateStorage, err error,
) {
	logger := logging.Extract(ctx)

	var msg string
	if err != nil {
		msg = fmt.Sprintf("Failed to trigger next state for processId. Error: %s", err.Error())
		logger.Error(msg,
			"process_id", contractState.StateID,
			"next_state", contractState.State,
			"role", contractState.ParticipantRole,
			"error", err)
		return
	}

	err = s.StoreContractNegotiationState(ctx, contractState.StateID, contractState)
	if err != nil {
		msg = fmt.Sprintf("Failed to store state for processId. Error: %s", err.Error())
		logger.Error(msg,
			"process_id", contractState.StateID,
			"next_state", contractState.State,
			"role", contractState.ParticipantRole,
			"error", err)
	}
}

func (s *SimpleStateStorage) triggerNextConsumerState(
	ctx context.Context, contractState DSPContractStateStorage,
) {
	logger := logging.Extract(ctx)
	logger.Info("Delaying triggering next state for one second")
	httpService := getHttpContractService(ctx, contractState)
	var err error
	switch contractState.State {
	case UndefinedState:
		contractState.State = Requested
		err = httpService.ConsumerSendContractRequest(ctx)
	case Agreed:
		contractState.State = Verified
		err = httpService.ConsumerSendAgreementVerificationRequest(ctx)
	case Requested:
	case Offered:
	case Accepted:
	case Verified:
	case Finalized:
	case Terminated:
	}

	s.checkErrorAndStoreState(ctx, contractState, err)
}

func (s *SimpleStateStorage) triggerNextProviderState(ctx context.Context, contractState DSPContractStateStorage) {
	logger := logging.Extract(ctx)
	logger.Info("Delaying triggering next state for one second")
	httpService := getHttpContractService(ctx, contractState)
	time.Sleep(time.Second)
	var err error
	switch contractState.State {
	case Requested:
		contractState.State = Agreed
		err = httpService.ProviderSendContractAgreement(ctx)
	case Agreed:
		contractState.State = Finalized
		err = httpService.ProviderSendContractFinalized(ctx)
	case Verified:
		contractState.State = Finalized
		err = httpService.ProviderSendContractFinalized(ctx)
	case UndefinedState:
	case Offered:
	case Accepted:
	case Finalized:
	case Terminated:
	}

	s.checkErrorAndStoreState(ctx, contractState, err)
}
