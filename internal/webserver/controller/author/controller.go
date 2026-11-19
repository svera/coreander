package author

import (
	"io/fs"

	"github.com/spf13/afero"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type Sender interface {
	From() string
}

type DataSourceLinkProvider interface {
	AuthorLink(author index.Author, language string) datasourcemodel.DataSourceLink
}

type ProfileInputProvider interface {
	ProfileServiceName() string
	ProfileValue(author index.Author) string
}

type ProfileInput struct {
	Name         string
	ServiceName  string
	Value        string
	Method       string
	Action       string
	FormID       string
	Indicator    string
	Disabled     string
	Invalid      bool
	ErrorMessage string
	ErrorID      string
}

func NewProfileInput(provider ProfileInputProvider, author index.Author, name, method, action string) ProfileInput {
	return ProfileInput{
		Name: name, ServiceName: provider.ProfileServiceName(), Value: provider.ProfileValue(author),
		Method: method, Action: action,
	}
}

func NewMetadataProfileInput(provider ProfileInputProvider, author index.Author, style string) ProfileInput {
	action := "/authors/" + author.Slug
	if style == "clear" {
		action += "?style=clear"
	}
	return NewProfileInput(provider, author, "sourceID", "put", action)
}

// IdxReader defines a set of author reading operations over an index
type IdxReader interface {
	SearchAuthors(searchFields index.AuthorSearchFields, page, resultsPerPage int) (result.Paginated[[]index.Author], error)
	SearchByAuthor(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Author(slug, lang string) (index.Author, error)
	IndexAuthor(author index.Author) error
	Languages() ([]string, error)
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type readingRepository interface {
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type Config struct {
	WordsPerMinute         float64
	CacheDir               string
	CacheMaxSize           int
	AuthorImageMaxWidth    int
	ClientImageCacheTTL    int
	ServerImageCacheTTL    int
	ReviewSourceConfigured bool
}

type Controller struct {
	hlRepository            highlightsRepository
	readingRepository       readingRepository
	idx                     IdxReader
	sender                  Sender
	config                  Config
	dataSource              DataSource
	appFs                   afero.Fs
	embeddedImagesFS        fs.FS
	dataSourceLinkProviders []DataSourceLinkProvider
	bioProvider             BioProvider
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReader, cfg Config, dataSource DataSource, appFs afero.Fs, embeddedImagesFS fs.FS, dataSourceLinkProviders ...DataSourceLinkProvider) *Controller {
	return &Controller{
		hlRepository:            hlRepository,
		readingRepository:       readingRepository,
		idx:                     idx,
		sender:                  sender,
		config:                  cfg,
		dataSource:              dataSource,
		appFs:                   appFs,
		embeddedImagesFS:        embeddedImagesFS,
		dataSourceLinkProviders: dataSourceLinkProviders,
	}
}

func (a *Controller) authorDataSourceLinks(author index.Author, language string) []datasourcemodel.DataSourceLink {
	return DataSourceLinks(author, language, a.dataSourceLinkProviders...)
}

func DataSourceLinks(author index.Author, language string, providers ...DataSourceLinkProvider) []datasourcemodel.DataSourceLink {
	var links []datasourcemodel.DataSourceLink
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		link := provider.AuthorLink(author, language)
		if link.URL != "" {
			links = append(links, link)
		}
	}
	return links
}

func (a *Controller) profileInputs(author index.Author, style, invalidName, submittedValue string) []ProfileInput {
	var inputs []ProfileInput
	if a.dataSource != nil {
		inputs = append(inputs, NewMetadataProfileInput(a.dataSource, author, style))
	}
	if a.bioProvider != nil {
		input := NewProfileInput(a.bioProvider, author, a.bioProvider.ProfileInputName(), "post", "/authors/"+author.Slug+"/bio/refresh")
		input.FormID = "author-bio-refresh"
		input.Indicator = "#author-bio"
		input.Disabled = "find button"
		if invalidName == input.Name {
			input.Value = submittedValue
			input.Invalid = true
			input.ErrorMessage = "Enter a valid profile URL or ID."
			input.ErrorID = "profile-error-" + author.Slug
		}
		inputs = append(inputs, input)
	}
	return inputs
}
