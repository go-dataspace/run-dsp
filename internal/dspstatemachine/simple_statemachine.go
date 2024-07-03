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

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

var (
	contractStates  = make(map[uuid.UUID]DSPContractStateStorage)
	contractsMutex  = &sync.RWMutex{}
	transferStates  = make(map[uuid.UUID]DSPTransferStateStorage)
	transfersMutex  = &sync.RWMutex{}
	agreements      = make(map[string]odrl.Agreement)
	agreementsMutex = &sync.RWMutex{}
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
	Offer                   odrl.MessageOffer
	Agreement               odrl.Agreement
	DatasetID               uuid.UUID
}

type DSPTransferStateStorage struct {
	StateID                 uuid.UUID
	ProviderPID             uuid.UUID
	ConsumerPID             uuid.UUID
	State                   TransferNegotiationState
	ConsumerCallbackAddress string
	ProviderCallbackAddress string
	ParticipantRole         DSPParticipantRole
	AgreementID             string
	PublishInfo             shared.PublishInfo
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
	contractsMutex.Lock()
	state, ok := contractStates[id]
	contractsMutex.Unlock()
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
	contractsMutex.RLock()
	contractStates[id] = negotationState
	contractsMutex.RUnlock()
	return nil
}

func (s *SimpleStateStorage) FindTransferNegotiationState(
	ctx context.Context, id uuid.UUID,
) (DSPTransferStateStorage, error) {
	logger := logging.Extract(ctx)
	logger.Debug("TRANSFERSTATESTORAGE: Trying to get transfer state", "state_id", id)
	transfersMutex.Lock()
	state, ok := transferStates[id]
	transfersMutex.Unlock()
	if !ok {
		logger.Debug("TRANSFERSTATESTORAGE: Could not find state, returning error")
		return DSPTransferStateStorage{}, errors.New("Could not find negotiation state")
	}
	return state, nil
}

func (s *SimpleStateStorage) StoreTransferNegotiationState(
	ctx context.Context, id uuid.UUID, negotationState DSPTransferStateStorage,
) error {
	logger := logging.Extract(ctx)
	logger.Debug("TRANSFERSTATESTORAGE: Storing transfer state", "state_id", id, "state_storage", negotationState)
	transfersMutex.RLock()
	transferStates[id] = negotationState
	transfersMutex.RUnlock()
	return nil
}

func (s *SimpleStateStorage) FindAgreement(
	ctx context.Context, id string,
) (odrl.Agreement, error) {
	logger := logging.Extract(ctx)
	logger.Debug("TRANSFERSTATESTORAGE: Trying to get transfer state", "state_id", id)
	agreementsMutex.Lock()
	agreement, ok := agreements[id]
	agreementsMutex.Unlock()
	if !ok {
		logger.Debug("TRANSFERSTATESTORAGE: Could not find state, returning error")
		return odrl.Agreement{}, errors.New("Could not find negotiation state")
	}
	return agreement, nil
}

func (s *SimpleStateStorage) StoreAgreement(
	ctx context.Context, id string, agreement odrl.Agreement,
) error {
	logger := logging.Extract(ctx)
	logger.Debug("TRANSFERSTATESTORAGE: Storing transfer state", "state_id", id, "state_storage", agreement)
	agreementsMutex.RLock()
	agreements[id] = agreement
	agreementsMutex.RUnlock()
	return nil
}

func (s *SimpleStateStorage) AllowStateToProgress(idChan chan StateStorageChannelMessage) {
	// get state of thing
	// check that it seems ok
	// trigger callback function to send out request
	logger := logging.Extract(s.ctx)
	logger.Info("Started go routine for state storage")

	//nolint:gosimple
	for {
		select {
		case message := <-idChan:
			logger = logging.Extract(message.Context)
			logger.Info("Got UUID of state to release...",
				"uuid", message.ProcessID)
			if message.TransactionType == Contract {
				s.triggerContractStates(message)
			} else {
				s.triggerTransferStates(message)
			}
		}
	}
}

func (s *SimpleStateStorage) triggerContractStates(message StateStorageChannelMessage) {
	logger := logging.Extract(message.Context)

	contractsMutex.Lock()
	state, ok := contractStates[message.ProcessID]
	contractsMutex.Unlock()
	if !ok {
		logger.Error("Could not find state for processId", "process_id", message.ProcessID)
	}

	if state.ParticipantRole == Consumer {
		go s.triggerNextConsumerContractState(message.Context, state)
	} else {
		go s.triggerNextProviderContractState(message.Context, state)
	}
}

func (s *SimpleStateStorage) triggerTransferStates(message StateStorageChannelMessage) {
	logger := logging.Extract(message.Context)
	logger.Debug("In trigger transfer states")
	transfersMutex.Lock()
	state, ok := transferStates[message.ProcessID]
	transfersMutex.Unlock()
	if !ok {
		logger.Error("Could not find state for processId", "process_id", message.ProcessID)
	}

	if state.ParticipantRole == Consumer {
		go s.triggerNextConsumerTransferState(message.Context, state)
	} else {
		go s.triggerNextProviderTransferState(message.Context, state)
	}
}

func (s *SimpleStateStorage) checkErrorAndStoreContractState(
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

	if contractState.State == Finalized {
		err = s.StoreAgreement(ctx, contractState.Agreement.ID, contractState.Agreement)
		if err != nil {
			msg = fmt.Sprintf("Failed to store agreement for processId. Error: %s", err.Error())
			logger.Error(msg,
				"process_id", contractState.StateID,
				"agreement", contractState.Agreement,
				"role", contractState.ParticipantRole,
				"error", err)
		}
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

func (s *SimpleStateStorage) checkErrorAndStoreTransferState(
	ctx context.Context, transferState DSPTransferStateStorage, err error,
) {
	logger := logging.Extract(ctx)
	var msg string
	if err != nil {
		msg = fmt.Sprintf("Failed to trigger next state for processId. Error: %s", err.Error())
		logger.Error(msg,
			"process_id", transferState.StateID,
			"next_state", transferState.State,
			"role", transferState.ParticipantRole,
			"error", err)
		return
	}

	err = s.StoreTransferNegotiationState(ctx, transferState.StateID, transferState)
	if err != nil {
		msg = fmt.Sprintf("Failed to store state for processId. Error: %s", err.Error())
		logger.Error(msg,
			"process_id", transferState.StateID,
			"next_state", transferState.State,
			"role", transferState.ParticipantRole,
			"error", err)
	}
}

func (s *SimpleStateStorage) triggerNextConsumerContractState(
	ctx context.Context, contractState DSPContractStateStorage,
) {
	logger := logging.Extract(ctx)
	logger.Info("Delaying triggering next consumer contract state for one second")
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

	s.checkErrorAndStoreContractState(ctx, contractState, err)
}

func (s *SimpleStateStorage) triggerNextProviderContractState(
	ctx context.Context, contractState DSPContractStateStorage,
) {
	logger := logging.Extract(ctx)
	logger.Info("Delaying triggering next provider contract state for one second")
	httpService := getHttpContractService(ctx, contractState)
	time.Sleep(time.Second)
	var err error
	switch contractState.State {
	case Requested:
		contractState.State = Agreed
		contractState.Agreement, err = httpService.ProviderSendContractAgreement(ctx)
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

	s.checkErrorAndStoreContractState(ctx, contractState, err)
}

func (s *SimpleStateStorage) triggerNextConsumerTransferState(
	ctx context.Context, transferState DSPTransferStateStorage,
) {
	logger := logging.Extract(ctx)
	logger.Info("Delaying triggering next consumer transfer state for one second")
	httpService := getHttpTransferService(ctx, transferState)
	var err error
	switch transferState.State {
	case UndefinedTransferState:
		transferState.State = TransferRequested
		err = httpService.ConsumerSendTransferRequest(ctx)
	case TransferRequested:
	case TransferStarted:
		transferState.State = TransferCompleted
		err = httpService.ConsumerSendTransferCompleted(ctx)
	case TransferSuspended:
	case TransferCompleted:
	case TransferTerminated:
	}

	s.checkErrorAndStoreTransferState(ctx, transferState, err)
}

func (s *SimpleStateStorage) triggerNextProviderTransferState(
	ctx context.Context, transferState DSPTransferStateStorage,
) {
	logger := logging.Extract(ctx)
	logger.Info("Delaying triggering next provider transfer state for one second")
	httpService := getHttpTransferService(ctx, transferState)
	var err error
	switch transferState.State {
	case UndefinedTransferState:
	case TransferRequested:
		transferState.State = TransferStarted
		err = httpService.ProviderSendTransferStartRequest(ctx)
	case TransferStarted:
	case TransferSuspended:
	case TransferCompleted:
	case TransferTerminated:
	}

	s.checkErrorAndStoreTransferState(ctx, transferState, err)
}
