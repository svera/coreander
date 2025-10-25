package infrastructure

import "sync"

type SMTPMock struct {
	calledSend         bool
	calledSendDocument bool
	mu                 sync.Mutex
	Wg                 sync.WaitGroup
	LastBody           string
}

func (s *SMTPMock) Send(address, subject, body string) error {
	defer s.Wg.Done()

	s.mu.Lock()
	s.calledSend = true
	s.LastBody = body
	s.mu.Unlock()
	return nil
}

func (s *SMTPMock) SendDocument(address, subject, libraryPath, fileName string) error {
	defer s.Wg.Done()

	s.mu.Lock()
	s.calledSendDocument = true
	s.mu.Unlock()
	return nil
}

func (s *SMTPMock) From() string {
	return ""
}

func (s *SMTPMock) CalledSend() bool {
	return s.calledSend
}
