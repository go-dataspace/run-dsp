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

package contract

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
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

var validTransitions = map[State][]State{
	States.INITIAL: {
		States.OFFERED,
		States.REQUESTED,
		States.TERMINATED,
	},
	States.REQUESTED: {
		States.OFFERED,
		States.AGREED,
		States.TERMINATED,
	},
	States.OFFERED: {
		States.REQUESTED,
		States.ACCEPTED,
		States.TERMINATED,
	},
	States.ACCEPTED: {
		States.AGREED,
		States.TERMINATED,
	},
	States.AGREED: {
		States.VERIFIED,
		States.TERMINATED,
	},
	States.VERIFIED: {
		States.FINALIZED,
		States.TERMINATED,
	},
	States.FINALIZED:  {},
	States.TERMINATED: {},
}

// Negotiation represents a contract negotiation.
type Negotiation struct {
	providerPID uuid.UUID
	consumerPID uuid.UUID
	state       State
	offer       odrl.Offer
	agreement   *odrl.Agreement
	callback    *url.URL
	self        *url.URL
	role        constants.DataspaceRole
	autoAccept  bool

	initial  bool
	ro       bool
	modified bool

	requesterInfo *dsrpc.RequesterInfo
}

type storableNegotiation struct {
	ProviderPID   uuid.UUID
	ConsumerPID   uuid.UUID
	State         State
	Offer         odrl.Offer
	Agreement     *odrl.Agreement
	Callback      *url.URL
	Self          *url.URL
	Role          constants.DataspaceRole
	AutoAccept    bool
	RequesterInfo *dsrpc.RequesterInfo
}

func New(
	providerPID, consumerPID uuid.UUID,
	state State,
	offer odrl.Offer,
	callback, self *url.URL,
	role constants.DataspaceRole,
	autoAccept bool,
	requesterInfo *dsrpc.RequesterInfo,
) *Negotiation {
	return &Negotiation{
		providerPID:   providerPID,
		consumerPID:   consumerPID,
		state:         state,
		offer:         offer,
		callback:      callback,
		self:          self,
		role:          role,
		autoAccept:    autoAccept,
		modified:      true,
		requesterInfo: requesterInfo,
	}
}

func FromBytes(b []byte) (*Negotiation, error) {
	var sn storableNegotiation
	r := bytes.NewReader(b)
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&sn); err != nil {
		return nil, fmt.Errorf("Could not decode bytes into storableNegotiation: %w", err)
	}
	return &Negotiation{
		providerPID:   sn.ProviderPID,
		consumerPID:   sn.ConsumerPID,
		state:         sn.State,
		offer:         sn.Offer,
		agreement:     sn.Agreement,
		callback:      sn.Callback,
		self:          sn.Self,
		role:          sn.Role,
		autoAccept:    sn.AutoAccept,
		requesterInfo: sn.RequesterInfo,
	}, nil
}

// GenerateStorageKey generates a key for a contract negotiation.
func GenerateStorageKey(id uuid.UUID, role constants.DataspaceRole) []byte {
	return []byte("negotiation-" + id.String() + "-" + strconv.Itoa(int(role)))
}

// Negotiation getters.
func (cn *Negotiation) GetProviderPID() uuid.UUID              { return cn.providerPID }
func (cn *Negotiation) GetConsumerPID() uuid.UUID              { return cn.consumerPID }
func (cn *Negotiation) GetState() State                        { return cn.state }
func (cn *Negotiation) GetOffer() odrl.Offer                   { return cn.offer }
func (cn *Negotiation) GetAgreement() *odrl.Agreement          { return cn.agreement }
func (cn *Negotiation) GetRole() constants.DataspaceRole       { return cn.role }
func (cn *Negotiation) GetCallback() *url.URL                  { return cn.callback }
func (cn *Negotiation) GetSelf() *url.URL                      { return cn.self }
func (cn *Negotiation) GetContract() *Negotiation              { return cn }
func (cn *Negotiation) GetRequesterInfo() *dsrpc.RequesterInfo { return cn.requesterInfo }

func (cn *Negotiation) GetLocalPID() uuid.UUID {
	switch cn.role {
	case constants.DataspaceConsumer:
		return cn.GetConsumerPID()
	case constants.DataspaceProvider:
		return cn.GetProviderPID()
	default:
		panic("not a valid role")
	}
}

func (cn *Negotiation) GetRemotePID() uuid.UUID {
	switch cn.role {
	case constants.DataspaceConsumer:
		return cn.GetProviderPID()
	case constants.DataspaceProvider:
		return cn.GetConsumerPID()
	default:
		panic("not a valid role")
	}
}

// Negotiation setters, these will panic when the negotiation is RO.
func (cn *Negotiation) SetProviderPID(u uuid.UUID) {
	cn.panicRO()
	cn.providerPID = u
	cn.modify()
}

func (cn *Negotiation) SetConsumerPID(u uuid.UUID) {
	cn.panicRO()
	cn.consumerPID = u
	cn.modify()
}

func (cn *Negotiation) SetAgreement(a *odrl.Agreement) {
	cn.panicRO()
	cn.agreement = a
	cn.modify()
}

func (cn *Negotiation) SetState(state State) error {
	cn.panicRO()
	if !slices.Contains(validTransitions[cn.state], state) {
		return fmt.Errorf("can't transition from %s to %s", cn.state, state)
	}
	cn.state = state
	cn.modify()
	return nil
}

// SetCallback sets the remote callback root.
func (cn *Negotiation) SetCallback(u string) error {
	nu, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	cn.callback = nu
	cn.modify()
	return nil
}

// AutoAccept is a property that decides if we're going to accept all operations to do with
// this contract negotiation.
func (cn *Negotiation) AutoAccept() bool { return cn.autoAccept }
func (cn *Negotiation) SetAutoAccept()   { cn.autoAccept = true }

// Properties that decisions are based on.
func (cn *Negotiation) ReadOnly() bool { return cn.ro }
func (cn *Negotiation) Initial() bool  { return cn.initial }
func (cn *Negotiation) Modified() bool { return cn.modified }
func (cn *Negotiation) StorageKey() []byte {
	id := cn.consumerPID
	if cn.role == constants.DataspaceProvider {
		id = cn.providerPID
	}
	return GenerateStorageKey(id, cn.role)
}

// Property setters.
func (cn *Negotiation) SetReadOnly()  { cn.ro = true }
func (cn *Negotiation) SetInitial()   { cn.initial = true }
func (cn *Negotiation) UnsetInitial() { cn.initial = false }

// ToBytes returns a binary representation of the negotiation, one that is compatible with the FromBytes
// function.
func (cn *Negotiation) ToBytes() ([]byte, error) {
	s := storableNegotiation{
		ProviderPID:   cn.providerPID,
		ConsumerPID:   cn.consumerPID,
		State:         cn.state,
		Offer:         cn.offer,
		Agreement:     cn.agreement,
		Callback:      cn.callback,
		Self:          cn.self,
		Role:          cn.role,
		AutoAccept:    cn.autoAccept,
		RequesterInfo: cn.requesterInfo,
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return nil, fmt.Errorf("could not encode negotiation: %w", err)
	}
	return buf.Bytes(), nil
}

// GetContractNegotiation returns a ContractNegotiation message.
func (cn *Negotiation) GetContractNegotiation() shared.ContractNegotiation {
	return shared.ContractNegotiation{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:ContractNegotiation",
		ConsumerPID: cn.GetConsumerPID().URN(),
		ProviderPID: cn.GetProviderPID().URN(),
		State:       cn.GetState().String(),
	}
}

func (cn *Negotiation) panicRO() {
	if cn.ro {
		panic("Trying to write to a read-only negotiation, this is certainly a bug.")
	}
}

func (cn *Negotiation) modify() {
	cn.modified = true
}
