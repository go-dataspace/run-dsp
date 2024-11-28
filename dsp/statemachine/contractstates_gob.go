package statemachine

func (p ContractState) GobEncode() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *ContractState) GobDecode(b []byte) error {
	newp, err := ParseContractState(b)
	if err != nil {
		return err
	}

	*p = newp
	return nil
}
