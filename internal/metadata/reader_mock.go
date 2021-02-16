package metadata

type ReaderMock struct {
	MetadataFake func(file string) (Metadata, error)
	CoverFake    func(bookFullPath string, outputFolder string) error
}

func NewReaderMock() ReaderMock {
	return ReaderMock{
		MetadataFake: func(file string) (Metadata, error) {
			return Metadata{}, nil
		},
		CoverFake: func(bookFullPath string, outputFolder string) error {
			return nil
		},
	}
}

func (e ReaderMock) Metadata(file string) (Metadata, error) {
	return e.MetadataFake(file)
}

func (e ReaderMock) Cover(bookFullPath string, outputFolder string) error {
	return e.CoverFake(bookFullPath, outputFolder)
}
