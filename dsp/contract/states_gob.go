package contract

func (p State) GobEncode() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *State) GobDecode(b []byte) error {
	newp, err := ParseState(b)
	if err != nil {
		return err
	}

	*p = newp
	return nil
}
