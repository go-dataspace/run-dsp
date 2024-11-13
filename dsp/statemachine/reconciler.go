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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

var (
	ErrFatal     = errors.New("fatal error")
	ErrTransient = errors.New("transient error")
)

type ReconciliationType uint

const (
	ReconciliationUndefined ReconciliationType = iota
	ReconciliationContract
	ReconciliationTransferRequest
)

type ReconciliationEntry struct {
	EntityID    uuid.UUID
	Type        ReconciliationType
	Role        DataspaceRole
	TargetState string
	NextAttempt time.Time
	Method      string
	URL         *url.URL
	Body        []byte
	Attempts    int
	Context     context.Context
}

// Reconciler is a naive reconciler loop. This should eventually be in its own package.
// TODO: This needs to be much more robust, right now it doesn't really do anything with the
// next attempt time or te amount of attempts. It also doesn't support any persistence.
type Reconciler struct {
	ctx context.Context
	c   chan ReconciliationEntry
	r   shared.Requester
	a   Archiver
}

func NewReconciler(ctx context.Context, r shared.Requester, a Archiver) *Reconciler {
	return &Reconciler{
		ctx: ctx,
		c:   make(chan ReconciliationEntry),
		r:   r,
		a:   a,
	}
}

func (r *Reconciler) Run() {
	go r.run()
}

func (r *Reconciler) Add(entry ReconciliationEntry) {
	r.c <- entry
}

func (r *Reconciler) run() {
	// rLogger is the non-entry specific logger for the reconciler
	rLogger := logging.Extract(r.ctx)
	rLogger.Info("Starting reconciliation loop")
	for {
		select {
		case entry := <-r.c:
			ctx := context.WithoutCancel(entry.Context)
			ctx, logger := logging.InjectLabels(ctx,
				"entityType", entry.Type,
				"entityRole", entry.Role,
				"entityID", entry.EntityID.String(),
				"method", entry.Method,
				"url", entry.URL.String(),
			)
			logger.Info("Attempting to reconcile entry")
			err := r.updateState(ctx, entry.EntityID, entry.Role, entry.Type, entry.TargetState)
			if err != nil {
				logger.Error("Could not update state", "err", err)
				continue
			}
			// As the dataspace standard doesn't care if we parse this, we won't.
			// TODO: care more about this. Implement requeuing logic.
			_, err = r.r.SendHTTPRequest(ctx, entry.Method, entry.URL, entry.Body)
			if err != nil {
				err := r.updateState(ctx, entry.EntityID, entry.Role, entry.Type, "dspace:TERMINATED")
				logger.Error("Could not send HTTP request", "err", err)
				continue
			}
		case <-r.ctx.Done():
			rLogger.Info("Context done called, exiting.")
			return
		}
	}
}

func (c *Reconciler) updateState(
	ctx context.Context, id uuid.UUID, role DataspaceRole, reconType ReconciliationType, state string,
) error {
	logger := logging.Extract(ctx)
	switch reconType {
	case ReconciliationContract:
		return c.setContractState(ctx, state, logger, role, id)
	case ReconciliationTransferRequest:
		return c.setTransferState(ctx, state, logger, role, id)
	case ReconciliationUndefined:
		logger.Error("Undefined type")
		return fmt.Errorf("Undefined type")
	default:
		logger.Error("Undefined type")
		return fmt.Errorf("Undefined type")
	}
}

//nolint:dupl
func (c *Reconciler) setTransferState(
	ctx context.Context, state string, logger *slog.Logger, role DataspaceRole, id uuid.UUID,
) error {
	ts, err := ParseTransferRequestState(state)
	if err != nil {
		logger.Error("Invalid state", "err", err)
		return ErrFatal
	}
	var tr *TransferRequest
	if role == DataspaceConsumer {
		tr, err = c.a.GetConsumerTransfer(ctx, id)
	} else {
		tr, err = c.a.GetProviderTransfer(ctx, id)
	}
	if err != nil {
		logger.Error("Can't find transfer request", "err", err)
		return ErrTransient
	}
	err = tr.SetState(ts)
	if err != nil {
		logger.Error("Can't change state", "err", err)
		return ErrFatal
	}
	if role == DataspaceConsumer {
		err = c.a.PutConsumerTransfer(ctx, tr)
	} else {
		err = c.a.PutProviderTransfer(ctx, tr)
	}
	if err != nil {
		logger.Error("Can't save transfer request")
		return ErrTransient
	}
	return nil
}

//nolint:dupl
func (c *Reconciler) setContractState(
	ctx context.Context, state string, logger *slog.Logger, role DataspaceRole, id uuid.UUID,
) error {
	cs, err := ParseContractState(state)
	if err != nil {
		logger.Error("Invalid state", "err", err)
		return ErrFatal
	}
	var con *Contract
	if role == DataspaceConsumer {
		con, err = c.a.GetConsumerContract(ctx, id)
	} else {
		con, err = c.a.GetProviderContract(ctx, id)
	}
	if err != nil {
		logger.Error("Can't find contract", "err", err)
		return ErrTransient
	}
	err = con.SetState(cs)
	if err != nil {
		logger.Error("Can't change state", "err", err)
		return ErrFatal
	}
	if role == DataspaceConsumer {
		err = c.a.PutConsumerContract(ctx, con)
	} else {
		err = c.a.PutProviderContract(ctx, con)
	}
	if err != nil {
		logger.Error("Can't save contract")
		return ErrTransient
	}
	return nil
}
