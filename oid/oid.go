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
package oid

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// urnPrefix contains the prefix of an OID formatted as an URN.
const urnPrefix = "urn:oid:"

// OID represents an OID as a slice of ints.
type OID []int64

// validOID is a regex that checks if the string starts, and ends, with a number, and only
// contains numbers and periods in between.
var validOID = regexp.MustCompile(`^\d[\d\.]+\d$`)

// Parse parses an OID string formatted as '1.2.3.4.5...' and returns an OID instance. It returns
// an error if the OID string can't be parsed.
func Parse(s string) (OID, error) {
	if len(s) == 0 {
		return OID{}, fmt.Errorf("can't parse empty string")
	}
	s = strings.ToLower(s)
	if strings.HasPrefix(s, urnPrefix) {
		s = strings.Replace(s, urnPrefix, "", 1)
	}
	if !validOID.MatchString(s) {
		return OID{}, fmt.Errorf("invalid OID: %s", s)
	}
	parts := strings.Split(s, ".")
	oid := make(OID, len(parts))
	for i, n := range parts {
		var err error
		oid[i], err = strconv.ParseInt(n, 10, 64)
		if err != nil {
			return oid, fmt.Errorf("could not parse OID string: %w", err)
		}
	}
	return oid, nil
}

// MustParse parses the OID string, but panics on error.
func MustParse(s string) OID {
	oid, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return oid
}

// String returns the string representation of the OID.
func (o OID) String() string {
	if o == nil {
		o = OID{}
	}
	parts := make([]string, len(o))
	for i, n := range o {
		parts[i] = strconv.FormatInt(n, 10)
	}
	return strings.Join(parts, ".")
}

// URN returns the URN of the OID.
func (o OID) URN() string {
	s := o.String()
	return fmt.Sprintf("%s%s", urnPrefix, s)
}
