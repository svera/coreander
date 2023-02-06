package webserver

import "github.com/svera/coreander/internal/controller"

// ReaderMock implements the Reader interface with a mockable struct. Just assign to each *Fake property
// the function you want to execute
type ReaderMock struct {
	SearchFake func(keywords string, page, resultsPerPage int) (*controller.Result, error)
	CountFake  func() (uint64, error)
	CloseFake  func() error
}

// NewReaderMock returns a new instance of ReaderMock
func NewReaderMock() *ReaderMock {
	return &ReaderMock{
		SearchFake: func(keywords string, page, resultsPerPage int) (*controller.Result, error) {
			return &controller.Result{}, nil
		},
		CountFake: func() (uint64, error) {
			return 0, nil
		},
		CloseFake: func() error {
			return nil
		},
	}
}

// Search runs the faked search method contained in the instance
func (r *ReaderMock) Search(keywords string, page, resultsPerPage int) (*controller.Result, error) {
	return r.SearchFake(keywords, page, resultsPerPage)
}

// Count runs the faked count method contained in the instance
func (r *ReaderMock) Count() (uint64, error) {
	return r.CountFake()
}

// Close runs the faked close method contained in the instance
func (r *ReaderMock) Close() error {
	return r.CloseFake()
}
