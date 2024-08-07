// Code generated by goenums. DO NOT EDIT.
// This file was generated by github.com/zarldev/goenums
// using the command:
// goenums contract_state.go

package statemachine

import (
	"bytes"
	"database/sql/driver"
	"fmt"
	"strconv"
)

type ContractState struct {
	contractState
}

type contractstatesContainer struct {
	INITIAL    ContractState
	REQUESTED  ContractState
	OFFERED    ContractState
	AGREED     ContractState
	ACCEPTED   ContractState
	VERIFIED   ContractState
	FINALIZED  ContractState
	TERMINATED ContractState
}

var ContractStates = contractstatesContainer{
	INITIAL: ContractState{
		contractState: initial,
	},
	REQUESTED: ContractState{
		contractState: requested,
	},
	OFFERED: ContractState{
		contractState: offered,
	},
	AGREED: ContractState{
		contractState: agreed,
	},
	ACCEPTED: ContractState{
		contractState: accepted,
	},
	VERIFIED: ContractState{
		contractState: verified,
	},
	FINALIZED: ContractState{
		contractState: finalized,
	},
	TERMINATED: ContractState{
		contractState: terminated,
	},
}

func (c contractstatesContainer) All() []ContractState {
	return []ContractState{
		c.INITIAL,
		c.REQUESTED,
		c.OFFERED,
		c.AGREED,
		c.ACCEPTED,
		c.VERIFIED,
		c.FINALIZED,
		c.TERMINATED,
	}
}

var invalidContractState = ContractState{}

func ParseContractState(a any) (ContractState, error) {
	res := invalidContractState
	switch v := a.(type) {
	case ContractState:
		return v, nil
	case []byte:
		res = stringToContractState(string(v))
	case string:
		res = stringToContractState(v)
	case fmt.Stringer:
		res = stringToContractState(v.String())
	case int:
		res = intToContractState(v)
	case int64:
		res = intToContractState(int(v))
	case int32:
		res = intToContractState(int(v))
	}
	return res, nil
}

func stringToContractState(s string) ContractState {
	switch s {
	case "INITIAL":
		return ContractStates.INITIAL
	case "dspace:REQUESTED":
		return ContractStates.REQUESTED
	case "dspace:OFFERED":
		return ContractStates.OFFERED
	case "dspace:AGREED":
		return ContractStates.AGREED
	case "dspace:ACCEPTED":
		return ContractStates.ACCEPTED
	case "dspace:VERIFIED":
		return ContractStates.VERIFIED
	case "dspace:FINALIZED":
		return ContractStates.FINALIZED
	case "dspace:TERMINATED":
		return ContractStates.TERMINATED
	}
	return invalidContractState
}

func intToContractState(i int) ContractState {
	if i < 0 || i >= len(ContractStates.All()) {
		return invalidContractState
	}
	return ContractStates.All()[i]
}

func ExhaustiveContractStates(f func(ContractState)) {
	for _, p := range ContractStates.All() {
		f(p)
	}
}

var validContractStates = map[ContractState]bool{
	ContractStates.INITIAL:    true,
	ContractStates.REQUESTED:  true,
	ContractStates.OFFERED:    true,
	ContractStates.AGREED:     true,
	ContractStates.ACCEPTED:   true,
	ContractStates.VERIFIED:   true,
	ContractStates.FINALIZED:  true,
	ContractStates.TERMINATED: true,
}

func (p ContractState) IsValid() bool {
	return validContractStates[p]
}

func (p ContractState) MarshalJSON() ([]byte, error) {
	return []byte(`"` + p.String() + `"`), nil
}

func (p *ContractState) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(bytes.Trim(b, `"`), ` `)
	newp, err := ParseContractState(b)
	if err != nil {
		return err
	}
	*p = newp
	return nil
}

func (p *ContractState) Scan(value any) error {
	newp, err := ParseContractState(value)
	if err != nil {
		return err
	}
	*p = newp
	return nil
}

func (p ContractState) Value() (driver.Value, error) {
	return p.String(), nil
}

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the goenums command to generate them again.
	// Does not identify newly added constant values unless order changes
	var x [1]struct{}
	_ = x[initial-0]
	_ = x[requested-1]
	_ = x[offered-2]
	_ = x[agreed-3]
	_ = x[accepted-4]
	_ = x[verified-5]
	_ = x[finalized-6]
	_ = x[terminated-7]
}

const _contractstates_name = "INITIALdspace:REQUESTEDdspace:OFFEREDdspace:AGREEDdspace:ACCEPTEDdspace:VERIFIEDdspace:FINALIZEDdspace:TERMINATED"

var _contractstates_index = [...]uint16{0, 7, 23, 37, 50, 65, 80, 96, 113}

func (i contractState) String() string {
	if i < 0 || i >= contractState(len(_contractstates_index)-1) {
		return "contractstates(" + (strconv.FormatInt(int64(i), 10) + ")")
	}
	return _contractstates_name[_contractstates_index[i]:_contractstates_index[i+1]]
}
