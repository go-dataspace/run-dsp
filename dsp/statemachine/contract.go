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
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

type DataspaceRole int8

const (
	DataspaceConsumer DataspaceRole = iota
	DataspaceProvider
)

var validTransitions = map[ContractState][]ContractState{
	ContractStates.INITIAL: {
		ContractStates.OFFERED,
		ContractStates.REQUESTED,
		ContractStates.TERMINATED,
	},
	ContractStates.REQUESTED: {
		ContractStates.OFFERED,
		ContractStates.AGREED,
		ContractStates.TERMINATED,
	},
	ContractStates.OFFERED: {
		ContractStates.REQUESTED,
		ContractStates.ACCEPTED,
		ContractStates.TERMINATED,
	},
	ContractStates.ACCEPTED: {
		ContractStates.AGREED,
		ContractStates.TERMINATED,
	},
	ContractStates.AGREED: {
		ContractStates.VERIFIED,
		ContractStates.TERMINATED,
	},
	ContractStates.VERIFIED: {
		ContractStates.FINALIZED,
		ContractStates.TERMINATED,
	},
	ContractStates.FINALIZED:  {},
	ContractStates.TERMINATED: {},
}

// Contract represents a contract negotiation.
type Contract struct {
	ProviderPID uuid.UUID
	ConsumerPID uuid.UUID
	State       ContractState
	Offer       odrl.Offer
	Agreement   odrl.Agreement
	Callback    *url.URL
	Self        *url.URL
	Role        DataspaceRole

	initial bool
	ro      bool
}

func (cn *Contract) GetProviderPID() uuid.UUID    { return cn.ProviderPID }
func (cn *Contract) SetProviderPID(u uuid.UUID)   { cn.ProviderPID = u }
func (cn *Contract) GetConsumerPID() uuid.UUID    { return cn.ConsumerPID }
func (cn *Contract) SetConsumerPID(u uuid.UUID)   { cn.ProviderPID = u }
func (cn *Contract) GetState() ContractState      { return cn.State }
func (cn *Contract) GetOffer() odrl.Offer         { return cn.Offer }
func (cn *Contract) GetAgreement() odrl.Agreement { return cn.Agreement }
func (cn *Contract) GetRole() DataspaceRole       { return cn.Role }
func (cn *Contract) GetCallback() *url.URL        { return cn.Callback }
func (cn *Contract) GetSelf() *url.URL            { return cn.Self }
func (cn *Contract) GetContract() *Contract       { return cn }

func (cn *Contract) SetReadOnly()   { cn.ro = true }
func (cn *Contract) ReadOnly() bool { return cn.ro }

func (cn *Contract) StorageKey() []byte {
	id := cn.ConsumerPID
	if cn.Role == DataspaceProvider {
		id = cn.ProviderPID
	}
	return MkTransferKey(id, cn.Role)
}

func (cn *Contract) SetState(state ContractState) error {
	if !slices.Contains(validTransitions[cn.State], state) {
		return fmt.Errorf("can't transition from %s to %s", cn.State, state)
	}
	cn.State = state
	return nil
}

// SetCallback sets the remote callback root.
func (cn *Contract) SetCallback(u string) error {
	nu, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	cn.Callback = nu
	return nil
}

// GetContractNegotiation returns a ContractNegotion message.
func (cn *Contract) GetContractNegotiation() shared.ContractNegotiation {
	return shared.ContractNegotiation{
		Context:     dspaceContext,
		Type:        "dspace:ContractNegotiation",
		ConsumerPID: cn.GetConsumerPID().URN(),
		ProviderPID: cn.GetProviderPID().URN(),
		State:       cn.GetState().String(),
	}
}

// Copy does a deep copy of a contract, here mostly for a workaround that will go away once
// we implement a reconciliation loop.
func (cn *Contract) Copy() *Contract {
	return &Contract{
		ProviderPID: cn.ProviderPID,
		ConsumerPID: cn.ConsumerPID,
		State:       cn.State,
		Offer:       cn.Offer,
		Agreement:   cn.Agreement,
		Callback:    mustURL(cn.Callback),
		Self:        mustURL(cn.Self),
		Role:        cn.Role,
		initial:     cn.initial,
	}
}

func mustURL(u *url.URL) *url.URL {
	n, err := url.Parse(u.String())
	if err != nil {
		panic(err.Error())
	}
	return n
}

func MkContractKey(id uuid.UUID, role DataspaceRole) []byte {
	return []byte("contract-" + id.String() + "-" + strconv.Itoa(int(role)))
}
