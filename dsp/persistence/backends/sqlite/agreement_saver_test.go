// Copyright 2026 go-dataspace
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

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite"
	"go-dataspace.eu/run-dsp/logging"
	"go-dataspace.eu/run-dsp/odrl"

	agreementopts "go-dataspace.eu/run-dsp/dsp/persistence/options/agreement"
)

//nolint:unparam // This is sometimes set to true when debugging tests.
func setupEnv(t *testing.T, dbDebug bool) (context.Context, *sqlite.Provider) {
	t.Helper()
	logger := logging.New("error", true)
	ctx := ctxslog.Inject(t.Context(), logger)
	p, err := sqlite.New(ctx, true, dbDebug, "")
	require.Nil(t, err)
	require.Nil(t, p.Migrate(ctx))
	return ctx, p
}

func TestPutAndGetAgreement(t *testing.T) {
	agreementID := uuid.New()
	targetID := uuid.New()
	agreement := odrl.Agreement{
		Type:        "odrl:Agreement",
		ID:          agreementID.URN(),
		Target:      targetID.URN(),
		Timestamp:   time.Date(1974, time.September, 9, 13, 14, 15, 0, time.UTC),
		PolicyClass: odrl.PolicyClass{},
	}
	ctx, p := setupEnv(t, false)
	defer require.Nil(t, p.Close())
	require.Nil(t, p.PutAgreement(ctx, &agreement))
	newAgreement, err := p.GetAgreement(ctx, agreementopts.WithAgreementID(agreementID))
	require.Nil(t, err)
	require.Equal(t, &agreement, newAgreement)
}
