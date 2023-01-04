package infrastructure

type NoEmail struct {
}

func (s *NoEmail) SendDocument(address string, libraryPath string, fileName string) error {
	return nil
}
