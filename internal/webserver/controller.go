package webserver

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/webserver/controller/auth"
	"github.com/svera/coreander/v4/internal/webserver/controller/author"
	"github.com/svera/coreander/v4/internal/webserver/controller/document"
	"github.com/svera/coreander/v4/internal/webserver/controller/highlight"
	"github.com/svera/coreander/v4/internal/webserver/controller/home"
	"github.com/svera/coreander/v4/internal/webserver/controller/series"
	"github.com/svera/coreander/v4/internal/webserver/controller/user"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

type Controllers struct {
	Auth       *auth.Controller
	Users      *user.Controller
	Highlights *highlight.Controller
	Documents  *document.Controller
	Home       *home.Controller
	Authors    *author.Controller
	Series     *series.Controller
}

func SetupControllers(cfg Config, db *gorm.DB, metadataReaders map[string]metadata.Reader, idx *index.BleveIndexer, sender Sender, appFs afero.Fs, dataSource author.DataSource) Controllers {
	usersRepository := &model.UserRepository{DB: db}
	highlightsRepository := &model.HighlightRepository{DB: db}
	readingRepository := &model.ReadingRepository{DB: db}

	authCfg := auth.Config{
		MinPasswordLength: cfg.MinPasswordLength,
		Secret:            cfg.JwtSecret,
		Hostname:          cfg.Hostname,
		Port:              cfg.Port,
		SessionTimeout:    cfg.SessionTimeout,
		RecoveryTimeout:   cfg.RecoveryTimeout,
	}

	usersCfg := user.Config{
		MinPasswordLength: cfg.MinPasswordLength,
		WordsPerMinute:    cfg.WordsPerMinute,
		Secret:            cfg.JwtSecret,
	}

	documentsCfg := document.Config{
		WordsPerMinute:        cfg.WordsPerMinute,
		LibraryPath:           cfg.LibraryPath,
		HomeDir:               cfg.HomeDir,
		CoverMaxWidth:         cfg.CoverMaxWidth,
		Hostname:              cfg.Hostname,
		Port:                  cfg.Port,
		UploadDocumentMaxSize: cfg.UploadDocumentMaxSize,
		DynamicImageCacheTTL:  cfg.DynamicImageCacheTTL,
	}

	authorsCfg := author.Config{
		WordsPerMinute:       cfg.WordsPerMinute,
		CacheDir:             cfg.CacheDir,
		AuthorImageMaxWidth:  cfg.AuthorImageMaxWidth,
		DynamicImageCacheTTL: cfg.DynamicImageCacheTTL,
	}

	seriesCfg := series.Config{
		WordsPerMinute: cfg.WordsPerMinute,
	}

	homeCfg := home.Config{
		LibraryPath:     cfg.LibraryPath,
		CoverMaxWidth:   cfg.CoverMaxWidth,
		LatestDocsLimit: 6,
	}

	return Controllers{
		Auth:       auth.NewController(usersRepository, sender, authCfg, translator),
		Users:      user.NewController(usersRepository, usersCfg, sender, translator),
		Highlights: highlight.NewController(highlightsRepository, usersRepository, sender, cfg.WordsPerMinute, idx),
		Documents:  document.NewController(highlightsRepository, readingRepository, sender, idx, metadataReaders, appFs, documentsCfg),
		Home:       home.NewController(highlightsRepository, readingRepository, sender, idx, homeCfg),
		Authors:    author.NewController(highlightsRepository, sender, idx, authorsCfg, dataSource, appFs),
		Series:     series.NewController(highlightsRepository, sender, idx, seriesCfg, appFs)}
}
