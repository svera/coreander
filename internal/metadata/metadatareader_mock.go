package metadata

type MetadataReaderMock struct {
	MetadataFake func(file string) (Metadata, error)
	CoverFake    func(bookFullPath string, outputFolder string) error
}

func NewMetadataReaderMock() MetadataReaderMock {
	return MetadataReaderMock{
		MetadataFake: func(file string) (Metadata, error) {
			return Metadata{}, nil
		},
		CoverFake: func(bookFullPath string, outputFolder string) error {
			return nil
		},
	}
}

func (e MetadataReaderMock) Metadata(file string) (Metadata, error) {
	return e.MetadataFake(file)
}

func (e MetadataReaderMock) Cover(bookFullPath string, outputFolder string) error {
	return e.CoverFake(bookFullPath, outputFolder)
}
