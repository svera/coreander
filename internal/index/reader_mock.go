package index

type ReaderMock struct {
	SearchFake func(keywords string, page, resultsPerPage int) (*Result, error)
	CountFake  func() (uint64, error)
	CloseFake  func() error
}

func NewReaderMock() *ReaderMock {
	return &ReaderMock{
		SearchFake: func(keywords string, page, resultsPerPage int) (*Result, error) {
			return &Result{}, nil
		},
		CountFake: func() (uint64, error) {
			return 0, nil
		},
		CloseFake: func() error {
			return nil
		},
	}
}

func (r *ReaderMock) Search(keywords string, page, resultsPerPage int) (*Result, error) {
	return r.SearchFake(keywords, page, resultsPerPage)
}

func (r *ReaderMock) Count() (uint64, error) {
	return r.CountFake()
}

func (r *ReaderMock) Close() error {
	return r.CloseFake()
}
