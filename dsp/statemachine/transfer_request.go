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

package statemachine

import (
	"fmt"
	"net/url"
	"slices"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
	"github.com/google/uuid"
)

var validTransferTransitions = map[TransferRequestState][]TransferRequestState{
	TransferRequestStates.TRANSFERINITIAL: {
		TransferRequestStates.TRANSFERREQUESTED,
		TransferRequestStates.TRANSFERTERMINATED,
	},
	TransferRequestStates.TRANSFERREQUESTED: {
		TransferRequestStates.STARTED,
		TransferRequestStates.TRANSFERTERMINATED,
	},
	TransferRequestStates.STARTED: {
		TransferRequestStates.SUSPENDED,
		TransferRequestStates.COMPLETED,
		TransferRequestStates.TRANSFERTERMINATED,
	},
	TransferRequestStates.SUSPENDED: {
		TransferRequestStates.STARTED,
		TransferRequestStates.TRANSFERTERMINATED,
	},
	TransferRequestStates.COMPLETED: {},
}

type TransferDirection uint8

const (
	DirectionUnknown TransferDirection = iota
	DirectionPull
	DirectionPush
)

// TransferRequest represents a transfer request and its state.
type TransferRequest struct {
	state             TransferRequestState
	providerPID       uuid.UUID
	consumerPID       uuid.UUID
	agreementID       uuid.UUID
	target            uuid.UUID
	format            string
	callback          *url.URL
	self              *url.URL
	role              DataspaceRole
	publishInfo       *providerv1.PublishInfo
	transferDirection TransferDirection
}

func (tr *TransferRequest) GetProviderPID() uuid.UUID               { return tr.providerPID }
func (tr *TransferRequest) GetConsumerPID() uuid.UUID               { return tr.consumerPID }
func (tr *TransferRequest) GetAgreementID() uuid.UUID               { return tr.agreementID }
func (tr *TransferRequest) GetTarget() uuid.UUID                    { return tr.target }
func (tr *TransferRequest) GetFormat() string                       { return tr.format }
func (tr *TransferRequest) GetCallback() *url.URL                   { return tr.callback }
func (tr *TransferRequest) GetSelf() *url.URL                       { return tr.self }
func (tr *TransferRequest) GetState() TransferRequestState          { return tr.state }
func (tr *TransferRequest) GetRole() DataspaceRole                  { return tr.role }
func (tr *TransferRequest) GetTransferRequest() *TransferRequest    { return tr }
func (tr *TransferRequest) GetPublishInfo() *providerv1.PublishInfo { return tr.publishInfo }
func (tr *TransferRequest) GetTransferDirection() TransferDirection { return tr.transferDirection }

func (tr *TransferRequest) SetState(state TransferRequestState) error {
	if !slices.Contains(validTransferTransitions[tr.state], state) {
		return fmt.Errorf("can't transition from %s to %s", tr.state, state)
	}
	tr.state = state
	return nil
}

func (tr *TransferRequest) GetTransferProcess() shared.TransferProcess {
	return shared.TransferProcess{
		Context:     dspaceContext,
		Type:        "dspace:TransferProcess",
		ProviderPID: tr.providerPID.URN(),
		ConsumerPID: tr.consumerPID.URN(),
		State:       tr.state.String(),
	}
}
