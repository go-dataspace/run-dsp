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

// Package OID contains tools to work with OIDs.
package oid_test

import (
	"testing"

	"github.com/go-dataspace/run-dsp/oid"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name      string
		args      args
		want      oid.OID
		assertion assert.ErrorAssertionFunc
	}{
		{
			name: "Parses normal OID.",
			args: args{
				s: "1.3.6.1.4.1.311.21.20",
			},
			want:      oid.OID{1, 3, 6, 1, 4, 1, 311, 21, 20},
			assertion: assert.NoError,
		},
		{
			name: "Errors on wrong character.",
			args: args{
				s: "1.3.6.1a.4.1.311.21.20",
			},
			want:      oid.OID{},
			assertion: assert.Error,
		},
		{
			name: "Errors when not starting with number.",
			args: args{
				s: ".3.6.1.4.1.311.21.20",
			},
			want:      oid.OID{},
			assertion: assert.Error,
		},
		{
			name: "Errors when not ending with number.",
			args: args{
				s: "1.3.6.1.4.1.311.21.",
			},
			want:      oid.OID{},
			assertion: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oid.Parse(tt.args.s)
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMustParse(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Panics on broken uuid",
			args: args{
				s: "1.3a.6.1.4.1.311.21.20",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() { _ = oid.MustParse(tt.args.s) })
		})
	}
}

func TestOID_String(t *testing.T) {
	tests := []struct {
		name string
		o    oid.OID
		want string
	}{
		{
			name: "Check formatting on normal OID",
			o:    oid.OID{1, 3, 6, 1, 4, 1, 311, 21, 20},
			want: "1.3.6.1.4.1.311.21.20",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.o.String())
		})
	}
}

func TestOID_URN(t *testing.T) {
	tests := []struct {
		name string
		o    oid.OID
		want string
	}{
		{
			name: "Check formatting on normal OID",
			o:    oid.OID{1, 3, 6, 1, 4, 1, 311, 21, 20},
			want: "urn:oid:1.3.6.1.4.1.311.21.20",
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.o.URN())
		})
	}
}
