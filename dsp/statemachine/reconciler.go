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
	"math/rand/v2"
	"net/url"
	"sync"
	"time"

	"github.com/gammazero/deque"
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

const (
	initialQueueSize     = 100
	reconciliationMillis = 10
	workers              = 1

	// Backoff settings.
	maxAttempts         = 50
	maxDuration         = 1 * time.Minute
	initialRetry        = 500 * time.Millisecond
	multiplier          = 1.5
	randomizationFactor = 0.5
)

type reconciliationOperation struct {
	Submitted       time.Time
	NextAttempt     time.Time
	Attempts        int
	Entry           ReconciliationEntry
	CurrentInterval time.Duration
}

type ReconciliationEntry struct {
	EntityID    uuid.UUID
	Type        ReconciliationType
	Role        DataspaceRole
	TargetState string
	Method      string
	URL         *url.URL
	Body        []byte
	Context     context.Context
}

// Reconciler tries to send out all the http requests, and retries them if something fails.
// A request has an exponential backoff that is defined in calculateNextAttempt.
// But simply said it takes the previous interval, adds 50% to that, and then randomises it a bit.
//
// Right now, almost nothing signals an immediate stop, but the option for that is already
// available.
type Reconciler struct {
	ctx context.Context
	c   chan reconciliationOperation
	r   shared.Requester
	a   Archiver
	q   *deque.Deque[reconciliationOperation]

	// Waitgroup to keep track of management/worker processes, not called from the command yet,
	// as that is pending on the http server respecting contexts.
	WaitGroup sync.WaitGroup
	sync.Mutex
}

func NewReconciler(ctx context.Context, r shared.Requester, a Archiver) *Reconciler {
	q := &deque.Deque[reconciliationOperation]{}
	q.Grow(initialQueueSize)

	return &Reconciler{
		ctx: ctx,
		c:   make(chan reconciliationOperation),
		r:   r,
		a:   a,
		q:   q,
	}
}

func (r *Reconciler) Run() {
	r.WaitGroup.Add(1 + workers)
	go r.manager()
	for range workers {
		go r.worker()
	}
}

func (r *Reconciler) Add(entry ReconciliationEntry) {
	r.Lock()
	defer r.Unlock()
	r.q.PushBack(reconciliationOperation{
		Submitted:       time.Now(),
		NextAttempt:     time.Now(),
		Attempts:        0,
		Entry:           entry,
		CurrentInterval: initialRetry,
	})
}

func (r *Reconciler) manager() {
	// We use a ticker to trigger iterations, this is to not hammer the queue in a tightloop.
	ticker := time.NewTicker(reconciliationMillis * time.Millisecond)
	logger := logging.Extract(r.ctx)
	for {
		select {
		case <-ticker.C:
			if r.q.Len() == 0 {
				continue
			}

			r.Lock()
			op := r.q.PopFront()
			r.Unlock()
			if time.Now().After(op.NextAttempt) {
				logger.Info("Reconciling...", "contract_id", op.Entry.EntityID)
				op.Attempts++
				r.c <- op
				continue
			}

			r.Lock()
			r.q.PushBack(op)
			r.Unlock()
		case <-r.ctx.Done():
			ticker.Stop()
			r.WaitGroup.Done()
			return
		}
	}
}

func (r *Reconciler) worker() {
	// rLogger is the non-entry specific logger for the reconciler
	rLogger := logging.Extract(r.ctx)
	rLogger.Info("Starting reconciliation loop")
	for {
		select {
		case op := <-r.c:
			entry := op.Entry
			ctx := context.WithoutCancel(entry.Context)
			ctx, logger := logging.InjectLabels(ctx,
				"entityType", entry.Type,
				"entityRole", entry.Role,
				"entityID", entry.EntityID.String(),
				"method", entry.Method,
				"url", entry.URL.String(),
			)
			logger.Info("Attempting to reconcile entry")

			// As the dataspace standard doesn't care if we parse this, we won't.
			_, err := r.r.SendHTTPRequest(ctx, entry.Method, entry.URL, entry.Body)
			if err != nil {
				r.handleError(ctx, op, fmt.Errorf("Could not send HTTP request: %w", err))
				continue
			}

			err = r.updateState(ctx, entry, entry.TargetState)
			if err != nil {
				r.handleError(ctx, op, fmt.Errorf("Could not update state: %w", err))
				continue
			}
		case <-r.ctx.Done():
			rLogger.Info("Context done called, exiting.")
			r.WaitGroup.Done()
			return
		}
	}
}

func (r *Reconciler) handleError(ctx context.Context, op reconciliationOperation, err error) {
	logger := logging.Extract(ctx).With(
		"err", err, "submitted", op.Submitted, "attempts", op.Attempts, "orig_next_attempt", op.NextAttempt)
	// If the error is fatal, just immediately terminate the operation.
	if errors.Is(err, ErrFatal) || op.Attempts >= maxAttempts {
		r.terminate(ctx, op.Entry)
		return
	}
	op.NextAttempt, op.CurrentInterval = calculateNextAttempt(op.CurrentInterval, op.Attempts)
	logger = logger.With("next_attempt", op.NextAttempt)
	if op.NextAttempt.Sub(op.Submitted) > maxDuration {
		r.terminate(ctx, op.Entry)
		return
	}
	logger.Error("Requeuing operation")
	r.Lock()
	r.q.PushBack(op)
	r.Unlock()
}

func (r *Reconciler) terminate(ctx context.Context, entry ReconciliationEntry) {
	logger := logging.Extract(ctx)
	logger.Error("Terminating entry")

	// For now, try 10 times to update the state to terminated, if it doesn't succeed panic.
	// We will handle this cleaner the future, but this is to make any bugs obvious.
	var err error
	for range 10 {
		err = r.updateState(ctx, entry, "dspace:TERMINATED")
		if err == nil {
			logger.Debug("Entry terminated")
			return
		}
		logger.Debug("Could not update state", "err", err)
	}
	panic(fmt.Sprintf("Could not set state to terminate, %s", err))
}

func calculateNextAttempt(currentInterval time.Duration, attempts int) (time.Time, time.Duration) {
	// Base interval is currentInterval * multiplier unless it's the first retry
	ci := float64(currentInterval)
	if attempts != 1 {
		ci *= multiplier
	}

	// Do some randomisation based on the randomization factor
	delta := randomizationFactor * ci
	minInterval := ci - delta
	maxInterval := ci + delta
	//nolint:gosec // This is not a security use of rand.
	randomValue := time.Duration(minInterval + (rand.Float64() * (maxInterval - minInterval + 1)))

	nextRun := time.Now().Add(randomValue)
	return nextRun, time.Duration(ci)
}

func (r *Reconciler) updateState(
	ctx context.Context, entry ReconciliationEntry, state string,
) error {
	logger := logging.Extract(ctx)
	switch entry.Type {
	case ReconciliationContract:
		return r.setContractState(ctx, state, entry.Role, entry.EntityID)
	case ReconciliationTransferRequest:
		return r.setTransferState(ctx, state, entry.Role, entry.EntityID)
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
	ctx context.Context, state string, role DataspaceRole, id uuid.UUID,
) error {
	ts, err := ParseTransferRequestState(state)
	if err != nil {
		return fmt.Errorf("%w: Invalid state: %w", ErrFatal, err)
	}
	var tr *TransferRequest
	if role == DataspaceConsumer {
		tr, err = c.a.GetConsumerTransfer(ctx, id)
	} else {
		tr, err = c.a.GetProviderTransfer(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("Can't find transfer request: %w", err)
	}
	err = tr.SetState(ts)
	if err != nil {
		return fmt.Errorf("Can't change state: %w", err)
	}
	if role == DataspaceConsumer {
		err = c.a.PutConsumerTransfer(ctx, tr)
	} else {
		err = c.a.PutProviderTransfer(ctx, tr)
	}
	if err != nil {
		return fmt.Errorf("Can't save transfer request: %w", err)
	}
	return nil
}

//nolint:dupl
func (c *Reconciler) setContractState(
	ctx context.Context, state string, role DataspaceRole, id uuid.UUID,
) error {
	cs, err := ParseContractState(state)
	if err != nil {
		return fmt.Errorf("%w: Invalid state: %w", ErrFatal, err)
	}
	var con *Contract
	if role == DataspaceConsumer {
		con, err = c.a.GetConsumerContract(ctx, id)
	} else {
		con, err = c.a.GetProviderContract(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("Can't find contract: %w", err)
	}
	err = con.SetState(cs)
	if err != nil {
		return fmt.Errorf("Can't change state: %w", err)
	}
	if role == DataspaceConsumer {
		err = c.a.PutConsumerContract(ctx, con)
	} else {
		err = c.a.PutProviderContract(ctx, con)
	}
	if err != nil {
		return fmt.Errorf("Can't save contract: %w", err)
	}
	return nil
}
