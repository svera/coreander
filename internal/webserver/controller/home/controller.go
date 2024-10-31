package home

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
)

const relatedDocuments = 4

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Documents(IDs []string) (map[string]index.Document, error)
	Count() (uint64, error)
}

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error)
}

type Config struct {
	LibraryPath           string
	HomeDir               string
	CoverMaxWidth         int
	Hostname              string
	Port                  int
	UploadDocumentMaxSize int
}

type Controller struct {
	hlRepository highlightsRepository
	idx          IdxReaderWriter
	sender       Sender
	config       Config
	appFs        afero.Fs
}

func NewController(hlRepository highlightsRepository, sender Sender, idx IdxReaderWriter, metadataReaders map[string]metadata.Reader, appFs afero.Fs, cfg Config) *Controller {
	return &Controller{
		hlRepository: hlRepository,
		idx:          idx,
		sender:       sender,
		config:       cfg,
		appFs:        appFs,
	}
}
