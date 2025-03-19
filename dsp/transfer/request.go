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

package transfer

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/odrl"
	providerv1 "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

var validTransferTransitions = map[State][]State{
	States.INITIAL: {
		States.REQUESTED,
		States.TERMINATED,
	},
	States.REQUESTED: {
		States.STARTED,
		States.TERMINATED,
	},
	States.STARTED: {
		States.SUSPENDED,
		States.COMPLETED,
		States.TERMINATED,
	},
	States.SUSPENDED: {
		States.STARTED,
		States.TERMINATED,
	},
	States.COMPLETED: {},
}

type Direction uint8

const (
	DirectionUnknown Direction = iota
	DirectionPull
	DirectionPush
)

// Request represents a transfer request and its state.
type Request struct {
	state             State
	providerPID       uuid.UUID
	consumerPID       uuid.UUID
	agreementID       uuid.UUID
	target            string
	format            string
	callback          *url.URL
	self              *url.URL
	role              constants.DataspaceRole
	publishInfo       *providerv1.PublishInfo
	transferDirection Direction

	ro       bool
	modified bool
}

type storableRequest struct {
	State             State
	ProviderPID       uuid.UUID
	ConsumerPID       uuid.UUID
	AgreementID       uuid.UUID
	Target            string
	Format            string
	Callback          *url.URL
	Self              *url.URL
	Role              constants.DataspaceRole
	PublishInfo       *providerv1.PublishInfo
	TransferDirection Direction
}

func New(
	consumerPID uuid.UUID,
	agreement *odrl.Agreement,
	format string,
	callback, self *url.URL,
	role constants.DataspaceRole,
	state State,
	publishInfo *providerv1.PublishInfo,
) *Request {
	targetID, err := shared.URNtoRawID(agreement.Target)
	if err != nil {
		panic("Misformed agreement, this means database corruption")
	}
	t := &Request{
		state:             state,
		consumerPID:       consumerPID,
		agreementID:       uuid.MustParse(agreement.ID),
		target:            targetID,
		format:            format,
		callback:          callback,
		self:              self,
		role:              role,
		publishInfo:       publishInfo,
		transferDirection: DirectionPush,
		modified:          true,
	}
	if publishInfo == nil {
		t.transferDirection = DirectionPull
	}
	return t
}

func FromBytes(b []byte) (*Request, error) {
	var sr storableRequest
	r := bytes.NewReader(b)
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&sr); err != nil {
		return nil, fmt.Errorf("could not decode bytes into storableRequest: %w", err)
	}
	return &Request{
		state:             sr.State,
		providerPID:       sr.ProviderPID,
		consumerPID:       sr.ConsumerPID,
		agreementID:       sr.AgreementID,
		target:            sr.Target,
		format:            sr.Format,
		callback:          sr.Callback,
		self:              sr.Self,
		role:              sr.Role,
		publishInfo:       sr.PublishInfo,
		transferDirection: sr.TransferDirection,
	}, nil
}

func GenerateKey(id uuid.UUID, role constants.DataspaceRole) []byte {
	return []byte("transfer-" + id.String() + "-" + strconv.Itoa(int(role)))
}

// Request getters.
func (tr *Request) GetProviderPID() uuid.UUID               { return tr.providerPID }
func (tr *Request) GetConsumerPID() uuid.UUID               { return tr.consumerPID }
func (tr *Request) GetAgreementID() uuid.UUID               { return tr.agreementID }
func (tr *Request) GetTarget() string                       { return tr.target }
func (tr *Request) GetFormat() string                       { return tr.format }
func (tr *Request) GetCallback() *url.URL                   { return tr.callback }
func (tr *Request) GetSelf() *url.URL                       { return tr.self }
func (tr *Request) GetState() State                         { return tr.state }
func (tr *Request) GetRole() constants.DataspaceRole        { return tr.role }
func (tr *Request) GetTransferRequest() *Request            { return tr }
func (tr *Request) GetPublishInfo() *providerv1.PublishInfo { return tr.publishInfo }
func (tr *Request) GetTransferDirection() Direction {
	return tr.transferDirection
}

// Request setters, these will panic when the transfer is RO.
func (tr *Request) SetPublishInfo(pi *providerv1.PublishInfo) {
	tr.panicRO()
	tr.publishInfo = pi
	tr.modify()
}

func (tr *Request) SetProviderPID(id uuid.UUID) {
	tr.panicRO()
	tr.providerPID = id
	tr.modify()
}

func (tr *Request) SetState(state State) error {
	tr.panicRO()
	if !slices.Contains(validTransferTransitions[tr.state], state) {
		return fmt.Errorf("can't transition from %s to %s", tr.state, state)
	}
	tr.state = state
	tr.modify()
	return nil
}

// Properties that decisions are based on.
func (tr *Request) ReadOnly() bool { return tr.ro }
func (tr *Request) Modified() bool { return tr.modified }
func (tr *Request) StorageKey() []byte {
	id := tr.consumerPID
	if tr.role == constants.DataspaceProvider {
		id = tr.providerPID
	}
	return GenerateKey(id, tr.role)
}

// Property setters.
func (tr *Request) SetReadOnly() { tr.ro = true }

func (tr *Request) ToBytes() ([]byte, error) {
	s := storableRequest{
		State:             tr.state,
		ProviderPID:       tr.providerPID,
		ConsumerPID:       tr.consumerPID,
		AgreementID:       tr.agreementID,
		Target:            tr.target,
		Format:            tr.format,
		Callback:          tr.callback,
		Self:              tr.self,
		Role:              tr.role,
		PublishInfo:       tr.publishInfo,
		TransferDirection: tr.transferDirection,
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return nil, fmt.Errorf("could not encode negotiation: %w", err)
	}
	return buf.Bytes(), nil
}

func (tr *Request) GetTransferProcess() shared.TransferProcess {
	return shared.TransferProcess{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:TransferProcess",
		ProviderPID: tr.providerPID.URN(),
		ConsumerPID: tr.consumerPID.URN(),
		State:       tr.state.String(),
	}
}

func (tr *Request) panicRO() {
	if tr.ro {
		panic("Trying to write to a read-only request, this is certainly a bug.")
	}
}

func (tr *Request) modify() {
	tr.modified = true
}
