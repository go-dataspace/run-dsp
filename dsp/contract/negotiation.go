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
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite/models"
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

	initial   bool
	ro        bool
	modified  bool
	traceInfo shared.TraceInfo

	requesterInfo *dsrpc.RequesterInfo

	// Internal database state, not used for anything except to keep track in the db.
	// Might be shown in debug info.
	internal_id int64
	locked      bool
}

func New(
	ctx context.Context,
	providerPID, consumerPID uuid.UUID,
	state State,
	offer odrl.Offer,
	callback, self *url.URL,
	role constants.DataspaceRole,
	autoAccept bool,
	requesterInfo *dsrpc.RequesterInfo,
) *Negotiation {
	neg := &Negotiation{
		providerPID:   providerPID,
		consumerPID:   consumerPID,
		state:         state,
		offer:         offer,
		callback:      callback,
		self:          self,
		role:          role,
		autoAccept:    autoAccept,
		modified:      true,
		traceInfo:     shared.ExtractTraceInfo(ctx),
		requesterInfo: requesterInfo,
	}
	ctxslog.Info(ctx, "creating new contract negotiation", neg.GetLogFields("")...)
	return neg
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

// For internal DB state, might not be used with certain storage backend, here ONLY FOR DISPLAY
// AND TESTING.
func (cn *Negotiation) GetInternalID() int64 { return cn.internal_id }
func (cn *Negotiation) GetLocked() bool      { return cn.locked }

func (cn *Negotiation) GetTraceInfo() shared.TraceInfo { return cn.traceInfo }
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

// GetLogFields will return relevant log fields for the negotiation.
// The suffix argument will append a prefix to the keys.
func (cn *Negotiation) GetLogFields(suffix string) []any {
	return []any{
		"role" + suffix, cn.role.String(),
		"consumerPID" + suffix, cn.GetConsumerPID().String(),
		"providerPID" + suffix, cn.GetProviderPID().String(),
		"state" + suffix, cn.GetState().String(),
		"callBack" + suffix, cn.GetCallback().String(),
		"selfURL" + suffix, cn.GetSelf().String(),
		"autoAccept" + suffix, cn.AutoAccept(),
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

// FromModel converts a ContractNegotiation model to a Negotiation.
//
//nolint:cyclop,funlen // It's not worth simplifying this at this time.
func FromModel(neg models.ContractNegotiation, ro bool) (*Negotiation, error) {
	ppid := uuid.UUID{}
	cpid := uuid.UUID{}
	var err error
	if neg.ProviderPID != nil {
		ppid, err = uuid.Parse(*neg.ProviderPID)
		if err != nil {
			return nil, err
		}
	}

	if neg.ConsumerPID != nil {
		cpid, err = uuid.Parse(*neg.ConsumerPID)
		if err != nil {
			return nil, err
		}
	}

	state, err := ParseState(neg.State)
	if err != nil {
		return nil, err
	}

	var offer odrl.Offer
	if err := json.Unmarshal([]byte(neg.Offer), &offer); err != nil {
		return nil, err
	}

	var agreement *odrl.Agreement
	if neg.Agreement != nil {
		var a odrl.Agreement
		if err := json.Unmarshal([]byte(neg.Agreement.ODRL), &a); err != nil {
			return nil, err
		}
		agreement = &a
	}

	callback, err := url.Parse(neg.CallbackURL)
	if err != nil {
		return nil, err
	}

	self, err := url.Parse(neg.SelfURL)
	if err != nil {
		return nil, err
	}

	role, err := constants.ParseRole(neg.Role)
	if err != nil {
		return nil, err
	}

	var traceInfo shared.TraceInfo
	if neg.TraceInfo != "" {
		if err := json.Unmarshal([]byte(neg.TraceInfo), &traceInfo); err != nil {
			return nil, err
		}
	}

	var requesterInfo *dsrpc.RequesterInfo
	if neg.RequesterInfo != nil {
		var ri dsrpc.RequesterInfo
		if err := json.Unmarshal([]byte(*neg.RequesterInfo), &ri); err != nil {
			return nil, err
		}
		requesterInfo = &ri
	}

	return &Negotiation{
		providerPID:   ppid,
		consumerPID:   cpid,
		state:         state,
		offer:         offer,
		agreement:     agreement,
		callback:      callback,
		self:          self,
		role:          role,
		autoAccept:    neg.AutoAccept,
		initial:       false,
		modified:      false,
		traceInfo:     traceInfo,
		requesterInfo: requesterInfo,
		ro:            ro,
		internal_id:   neg.ID,
		locked:        neg.Locked,
	}, nil
}

// ToModel converts this negotation to its model counterpart.
func (cn *Negotiation) ToModel() (models.ContractNegotiation, error) {
	empty_urn := uuid.UUID{}.URN()
	pp := (*string)(nil)
	if provider_pid := cn.providerPID.URN(); provider_pid != empty_urn {
		pp = &provider_pid
	}
	cp := (*string)(nil)
	if consumer_pid := cn.consumerPID.URN(); consumer_pid != empty_urn {
		cp = &consumer_pid
	}

	ai := (*string)(nil)
	if cn.agreement != nil {
		ai = &cn.agreement.ID
	}

	offer, err := json.Marshal(cn.offer)
	if err != nil {
		return models.ContractNegotiation{}, err
	}

	var reqInfo *string
	if cn.requesterInfo != nil {
		ri, err := json.Marshal(cn.requesterInfo)
		if err != nil {
			return models.ContractNegotiation{}, err
		}
		ri2 := string(ri)
		reqInfo = &ri2
	}

	traceInfo, err := json.Marshal(cn.traceInfo)
	if err != nil {
		return models.ContractNegotiation{}, err
	}
	return models.ContractNegotiation{
		ID:            cn.internal_id,
		ProviderPID:   pp,
		ConsumerPID:   cp,
		AgreementID:   ai,
		Offer:         string(offer),
		State:         cn.state.String(),
		CallbackURL:   cn.callback.String(),
		SelfURL:       cn.self.String(),
		Role:          cn.role.String(),
		AutoAccept:    cn.autoAccept,
		RequesterInfo: reqInfo,
		TraceInfo:     string(traceInfo),
		Locked:        false, // If we're saving, this always should be false
	}, nil
}
