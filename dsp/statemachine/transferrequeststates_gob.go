package statemachine

func (p TransferRequestState) GobEncode() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *TransferRequestState) GobDecode(b []byte) error {
	newp, err := ParseTransferRequestState(b)
	if err != nil {
		return err
	}

	*p = newp
	return nil
}
