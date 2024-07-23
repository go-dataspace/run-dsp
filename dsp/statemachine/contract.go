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
	providerPID uuid.UUID
	consumerPID uuid.UUID
	state       ContractState
	offer       odrl.Offer
	agreement   odrl.Agreement
	callback    *url.URL
	self        *url.URL
	role        DataspaceRole

	initial bool
}

func (cn *Contract) GetProviderPID() uuid.UUID    { return cn.providerPID }
func (cn *Contract) SetProviderPID(u uuid.UUID)   { cn.providerPID = u }
func (cn *Contract) GetConsumerPID() uuid.UUID    { return cn.consumerPID }
func (cn *Contract) SetConsumerPID(u uuid.UUID)   { cn.providerPID = u }
func (cn *Contract) GetState() ContractState      { return cn.state }
func (cn *Contract) GetOffer() odrl.Offer         { return cn.offer }
func (cn *Contract) GetAgreement() odrl.Agreement { return cn.agreement }
func (cn *Contract) GetRole() DataspaceRole       { return cn.role }
func (cn *Contract) GetCallback() *url.URL        { return cn.callback }
func (cn *Contract) GetSelf() *url.URL            { return cn.self }
func (cn *Contract) GetContract() *Contract       { return cn }

func (cn *Contract) SetState(state ContractState) error {
	if !slices.Contains(validTransitions[cn.state], state) {
		return fmt.Errorf("can't transition from %s to %s", cn.state, state)
	}
	cn.state = state
	return nil
}

// SetCallback sets the remote callback root.
func (cn *Contract) SetCallback(u string) error {
	nu, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	cn.callback = nu
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
		providerPID: cn.providerPID,
		consumerPID: cn.consumerPID,
		state:       cn.state,
		offer:       cn.offer,
		agreement:   cn.agreement,
		callback:    mustURL(cn.callback),
		self:        mustURL(cn.self),
		role:        cn.role,
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
