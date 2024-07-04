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

package shared_test

import (
	"testing"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

//nolint:funlen
func TestParseUUIDURN(t *testing.T) {
	type args struct {
		uu string
	}
	tests := []struct {
		name      string
		args      args
		want      uuid.UUID
		assertion assert.ErrorAssertionFunc
	}{
		{
			name: "Valid UUID URN",
			args: args{
				uu: "urn:uuid:7df0c972-5979-4872-aecd-88ab934a46e9",
			},
			want:      uuid.MustParse("7df0c972-5979-4872-aecd-88ab934a46e9"),
			assertion: assert.NoError,
		},
		{
			name: "Valid UUID URN, all caps",
			args: args{
				uu: "URN:UUID:7DF0C972-5979-4872-AECD-88AB934A46E9",
			},
			want:      uuid.MustParse("7df0c972-5979-4872-aecd-88ab934a46e9"),
			assertion: assert.NoError,
		},
		{
			name: "Invalid UUID",
			args: args{
				uu: "urn:uuid:7df0c972-5979-4872-acd-88ab934a46e9",
			},
			want:      uuid.UUID{},
			assertion: assert.Error,
		},
		{
			name: "Invalid URN namespace",
			args: args{
				uu: "urn:ruid:7df0c972-5979-4872-aecd-88ab934a46e9",
			},
			want:      uuid.UUID{},
			assertion: assert.Error,
		},
		{
			name: "Invalid URN scheme",
			args: args{
				uu: "url:uuid:7df0c972-5979-4872-aecd-88ab934a46e9",
			},
			want:      uuid.UUID{},
			assertion: assert.Error,
		},
		{
			name: "Bare UUID",
			args: args{
				uu: "7df0c972-5979-4872-aecd-88ab934a46e9",
			},
			want:      uuid.UUID{},
			assertion: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shared.ParseUUIDURN(tt.args.uu)
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
