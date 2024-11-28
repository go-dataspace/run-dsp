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
	"strconv"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
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
	State             TransferRequestState
	ProviderPID       uuid.UUID
	ConsumerPID       uuid.UUID
	AgreementID       uuid.UUID
	Target            string
	Format            string
	Callback          *url.URL
	Self              *url.URL
	Role              DataspaceRole
	PublishInfo       *providerv1.PublishInfo
	TransferDirection TransferDirection

	ro bool
}

func (tr *TransferRequest) GetProviderPID() uuid.UUID               { return tr.ProviderPID }
func (tr *TransferRequest) GetConsumerPID() uuid.UUID               { return tr.ConsumerPID }
func (tr *TransferRequest) GetAgreementID() uuid.UUID               { return tr.AgreementID }
func (tr *TransferRequest) GetTarget() string                       { return tr.Target }
func (tr *TransferRequest) GetFormat() string                       { return tr.Format }
func (tr *TransferRequest) GetCallback() *url.URL                   { return tr.Callback }
func (tr *TransferRequest) GetSelf() *url.URL                       { return tr.Self }
func (tr *TransferRequest) GetState() TransferRequestState          { return tr.State }
func (tr *TransferRequest) GetRole() DataspaceRole                  { return tr.Role }
func (tr *TransferRequest) GetTransferRequest() *TransferRequest    { return tr }
func (tr *TransferRequest) GetPublishInfo() *providerv1.PublishInfo { return tr.PublishInfo }
func (tr *TransferRequest) GetTransferDirection() TransferDirection { return tr.TransferDirection }

func (tr *TransferRequest) SetReadOnly()   { tr.ro = true }
func (tr *TransferRequest) ReadOnly() bool { return tr.ro }

func (tr *TransferRequest) StorageKey() []byte {
	id := tr.ConsumerPID
	if tr.Role == DataspaceProvider {
		id = tr.ProviderPID
	}
	return MkTransferKey(id, tr.Role)
}

func (tr *TransferRequest) SetState(state TransferRequestState) error {
	if !slices.Contains(validTransferTransitions[tr.State], state) {
		return fmt.Errorf("can't transition from %s to %s", tr.State, state)
	}
	tr.State = state
	return nil
}

func (tr *TransferRequest) GetTransferProcess() shared.TransferProcess {
	return shared.TransferProcess{
		Context:     dspaceContext,
		Type:        "dspace:TransferProcess",
		ProviderPID: tr.ProviderPID.URN(),
		ConsumerPID: tr.ConsumerPID.URN(),
		State:       tr.State.String(),
	}
}

func (tr *TransferRequest) SetProviderPID(id uuid.UUID) { tr.ProviderPID = id }

func MkTransferKey(id uuid.UUID, role DataspaceRole) []byte {
	return []byte("transfer-" + id.String() + "-" + strconv.Itoa(int(role)))
}
