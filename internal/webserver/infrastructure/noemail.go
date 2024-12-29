package infrastructure

type NoEmail struct {
}

func (s *NoEmail) Send(address, subject, body string) error {
	return nil
}

func (s *NoEmail) SendDocument(address, subject, libraryPath, fileName string) error {
	return nil
}

func (s *NoEmail) From() string {
	return ""
}
