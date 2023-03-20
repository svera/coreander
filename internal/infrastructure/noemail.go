package infrastructure

type NoEmail struct {
}

func (s *NoEmail) Send(address, subject, body string) error {
	return nil
}

func (s *NoEmail) SendDocument(address string, libraryPath string, fileName string) error {
	return nil
}
