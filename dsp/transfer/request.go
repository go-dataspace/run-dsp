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

const TransferPrefix = "transfer-"

const (
	DirectionUnknown Direction = iota
	DirectionPull
	DirectionPush
)

func ParseDirection(s string) (Direction, error) {
	switch s {
	case "UNKNOWN":
		return DirectionUnknown, nil
	case "PULL":
		return DirectionPull, nil
	case "PUSH":
		return DirectionPush, nil
	default:
		return 255, fmt.Errorf("invalid direction: %s", s)
	}
}

func (d Direction) String() string {
	switch d {
	case DirectionUnknown:
		return "UNKNOWN"
	case DirectionPull:
		return "PULL"
	case DirectionPush:
		return "PUSH"
	default:
		panic(fmt.Sprintf("unexpected transfer.Direction: %#v", d))
	}
}

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
	publishInfo       *dsrpc.PublishInfo
	requesterInfo     *dsrpc.RequesterInfo
	transferDirection Direction

	ro        bool
	modified  bool
	traceInfo shared.TraceInfo

	// Internal database state, not used for anything except to keep track in the db.
	// Might be shown in debug info.
	internal_id int64
	locked      bool
}

func New(
	ctx context.Context,
	consumerPID uuid.UUID,
	agreement *odrl.Agreement,
	format string,
	callback, self *url.URL,
	role constants.DataspaceRole,
	state State,
	publishInfo *dsrpc.PublishInfo,
	requesterInfo *dsrpc.RequesterInfo,
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
		requesterInfo:     requesterInfo,
		transferDirection: DirectionPush,
		modified:          true,
		traceInfo:         shared.ExtractTraceInfo(ctx),
	}
	if publishInfo == nil {
		t.transferDirection = DirectionPull
	}
	ctxslog.Info(ctx, "creating a new transfer request", t.GetLogFields("")...)
	return t
}

func GenerateKey(id uuid.UUID, role constants.DataspaceRole) []byte {
	return []byte(TransferPrefix + id.String() + "-" + strconv.Itoa(int(role)))
}

// Request getters.
func (tr *Request) GetProviderPID() uuid.UUID              { return tr.providerPID }
func (tr *Request) GetConsumerPID() uuid.UUID              { return tr.consumerPID }
func (tr *Request) GetAgreementID() uuid.UUID              { return tr.agreementID }
func (tr *Request) GetTarget() string                      { return tr.target }
func (tr *Request) GetFormat() string                      { return tr.format }
func (tr *Request) GetCallback() *url.URL                  { return tr.callback }
func (tr *Request) GetSelf() *url.URL                      { return tr.self }
func (tr *Request) GetState() State                        { return tr.state }
func (tr *Request) GetRole() constants.DataspaceRole       { return tr.role }
func (tr *Request) GetTransferRequest() *Request           { return tr }
func (tr *Request) GetRequesterInfo() *dsrpc.RequesterInfo { return tr.requesterInfo }
func (tr *Request) GetPublishInfo() *dsrpc.PublishInfo     { return tr.publishInfo }
func (tr *Request) GetTransferDirection() Direction {
	return tr.transferDirection
}

func (tr *Request) GetLogFields(suffix string) []any {
	return []any{
		"role" + suffix, tr.role.String(),
		"consumerPID" + suffix, tr.GetConsumerPID().String(),
		"providerPID" + suffix, tr.GetProviderPID().String(),
		"agreementID" + suffix, tr.GetAgreementID(),
		"state" + suffix, tr.GetState().String(),
		"callBack" + suffix, tr.GetCallback().String(),
		"selfURL" + suffix, tr.GetSelf().String(),
	}
}

func (tr *Request) GetTraceInfo() shared.TraceInfo { return tr.traceInfo }
func (tr *Request) GetLocalPID() uuid.UUID {
	switch tr.role {
	case constants.DataspaceConsumer:
		return tr.GetConsumerPID()
	case constants.DataspaceProvider:
		return tr.GetProviderPID()
	default:
		panic("not a valid role")
	}
}

// Request setters, these will panic when the transfer is RO.
func (tr *Request) SetPublishInfo(pi *dsrpc.PublishInfo) {
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

// FromModel convers a TransferREquest model to a Request.
//
//nolint:cyclop,funlen // It's not worth simplifying this at this time.
func FromModel(req models.TransferRequest, ro bool) (*Request, error) {
	state, err := ParseState(req.State)
	if err != nil {
		return nil, err
	}

	ppid, err := uuid.Parse(*req.ProviderPID)
	if err != nil {
		return nil, err
	}

	cpid, err := uuid.Parse(*req.ConsumerPID)
	if err != nil {
		return nil, err
	}

	agreement_id, err := uuid.Parse(req.Agreement.ID)
	if err != nil {
		return nil, err
	}

	callback, err := url.Parse(req.CallbackURL)
	if err != nil {
		return nil, err
	}

	self, err := url.Parse(req.SelfURL)
	if err != nil {
		return nil, err
	}

	role, err := constants.ParseRole(req.Role)
	if err != nil {
		return nil, err
	}

	var publishInfo *dsrpc.PublishInfo
	if req.PublishInfo != nil {
		if err := json.Unmarshal([]byte(*req.PublishInfo), publishInfo); err != nil {
			return nil, err
		}
	}

	var traceInfo shared.TraceInfo
	if err := json.Unmarshal([]byte(req.TraceInfo), &traceInfo); err != nil {
		return nil, err
	}

	var requesterInfo dsrpc.RequesterInfo
	if err := json.Unmarshal([]byte(req.RequesterInfo), &requesterInfo); err != nil {
		return nil, err
	}

	direction, err := ParseDirection(req.Direction)
	if err != nil {
		return nil, err
	}
	return &Request{
		state:             state,
		providerPID:       ppid,
		consumerPID:       cpid,
		agreementID:       agreement_id,
		target:            req.Target,
		format:            req.Format,
		callback:          callback,
		self:              self,
		role:              role,
		publishInfo:       publishInfo,
		requesterInfo:     &requesterInfo,
		transferDirection: direction,
		ro:                !req.Locked,
		modified:          false,
		traceInfo:         traceInfo,
		internal_id:       req.ID,
		locked:            req.Locked,
	}, nil
}

// ToModel converts this request to its model counterpart.
func (tr *Request) ToModel() (models.TransferRequest, error) {
	empty_urn := uuid.UUID{}.URN()
	pp := (*string)(nil)
	if provider_pid := tr.providerPID.URN(); provider_pid != empty_urn {
		pp = &provider_pid
	}
	cp := (*string)(nil)
	if consumer_pid := tr.consumerPID.URN(); consumer_pid != empty_urn {
		cp = &consumer_pid
	}

	var pi *string
	if tr.GetPublishInfo() != nil {
		b, err := json.Marshal(tr.publishInfo)
		if err != nil {
			return models.TransferRequest{}, err
		}
		s := string(b)
		pi = &s
	}

	reqInfo, err := json.Marshal(tr.requesterInfo)
	if err != nil {
		return models.TransferRequest{}, err
	}

	traceInfo, err := json.Marshal(tr.traceInfo)
	if err != nil {
		return models.TransferRequest{}, err
	}
	return models.TransferRequest{
		ID:            tr.internal_id,
		ProviderPID:   pp,
		ConsumerPID:   cp,
		AgreementID:   tr.agreementID.URN(),
		Target:        tr.target,
		Format:        tr.format,
		PublishInfo:   pi,
		Direction:     tr.transferDirection.String(),
		State:         tr.state.String(),
		CallbackURL:   tr.callback.String(),
		SelfURL:       tr.self.String(),
		Role:          tr.role.String(),
		RequesterInfo: string(reqInfo),
		TraceInfo:     string(traceInfo),
		Locked:        false,
	}, nil
}
