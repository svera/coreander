package infrastructure

type NoEmail struct {
}

func (s *NoEmail) Send(address, subject, body string) error {
	return nil
}

func (s *NoEmail) SendBCC(addresses []string, subject, body string) error {
	return nil
}

func (s *NoEmail) SendDocument(address, subject string, file []byte, fileName string) error {
	return nil
}

func (s *NoEmail) From() string {
	return ""
}
