package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"gorm.io/gorm"

	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
	"golang.org/x/crypto/acme/autocert"
)

var version string = "unknown"

func main() {
	var (
		cfg   Config
		appFs = afero.NewOsFs()
		idx   *index.BleveIndexer
	)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadEnv(&cfg); err != nil {
		log.Fatalf("Error parsing configuration from environment variables: %s", err)
	}
	if _, err := os.Stat(cfg.LibPath); os.IsNotExist(err) {
		log.Fatalf("Directory '%s' does not exist, exiting", cfg.LibPath)
	}

	metadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{},
		".pdf":  metadata.PdfReader{},
	}

	indexFile, err := bleve.Open(homeDir + "/coreander/db")
	if err == nil {
		idx = index.NewBleve(indexFile, cfg.LibPath, metadataReaders)
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		cfg.SkipIndexing = false
		idx = createIndex(homeDir, cfg.LibPath, metadataReaders)
	}
	db := infrastructure.Connect(homeDir+"/coreander/db/database.db", cfg.WordsPerMinute)

	run(cfg, db, idx, homeDir, metadataReaders, appFs)
}

func run(cfg Config, db *gorm.DB, idx *index.BleveIndexer, homeDir string, metadataReaders map[string]metadata.Reader, appFs afero.Fs) {
	var sender webserver.Sender

	defer idx.Close()

	if !cfg.SkipIndexing {
		go startIndex(idx, appFs, cfg.BatchSize, cfg.LibPath)
	} else {
		go fileWatcher(idx, cfg.LibPath)
	}

	sender = &infrastructure.NoEmail{}
	if cfg.SmtpServer != "" && cfg.SmtpUser != "" && cfg.SmtpPassword != "" {
		sender = &infrastructure.SMTP{
			Server:   cfg.SmtpServer,
			Port:     cfg.SmtpPort,
			User:     cfg.SmtpUser,
			Password: cfg.SmtpPassword,
		}
	}

	webserverConfig := webserver.Config{
		LibraryPath:       cfg.LibPath,
		HomeDir:           homeDir,
		Version:           version,
		CoverMaxWidth:     cfg.CoverMaxWidth,
		JwtSecret:         cfg.JwtSecret,
		RequireAuth:       cfg.RequireAuth,
		MinPasswordLength: cfg.MinPasswordLength,
		WordsPerMinute:    cfg.WordsPerMinute,
		Hostname:          cfg.Hostname,
		Port:              cfg.Port,
		SessionTimeout:    cfg.SessionTimeout,
	}
	app := webserver.New(idx, webserverConfig, metadataReaders, sender, db)
	fmt.Printf("Coreander version %s started listening on port %d\n\n", version, cfg.Port)
	if cfg.Port == 443 {
		ln := tlsListener(cfg.Hostname, homeDir)
		log.Fatal(app.Listener(ln))
	} else {
		log.Fatal(app.Listen(fmt.Sprintf(":%d", cfg.Port)))
	}
}

func startIndex(idx *index.BleveIndexer, appFs afero.Fs, batchSize int, libPath string) {
	start := time.Now().Unix()
	log.Printf("Indexing books at %s, this can take a while depending on the size of your library.", libPath)
	err := idx.AddLibrary(appFs, batchSize)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now().Unix()
	dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
	log.Printf("Indexing finished, took %d seconds", int(dur.Seconds()))
	fileWatcher(idx, libPath)
}

func createIndex(homeDir, libPath string, metadataReaders map[string]metadata.Reader) *index.BleveIndexer {
	log.Println("No index found, creating a new one")

	indexFile, err := bleve.New(homeDir+"/coreander/db", index.Mapping())
	if err != nil {
		log.Fatal(err)
	}
	return index.NewBleve(indexFile, libPath, metadataReaders)
}

func tlsListener(hostname, homeDir string) net.Listener {
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		// Replace with your domain
		HostPolicy: autocert.HostWhitelist(hostname),
		// Folder to store the certificates
		Cache: autocert.DirCache(homeDir + "/coreander/certs"),
	}

	// TLS Config
	cfg := &tls.Config{
		// Get Certificate from Let's Encrypt
		GetCertificate: m.GetCertificate,
		// By default NextProtos contains the "h2"
		// This has to be removed since Fasthttp does not support HTTP/2
		// Or it will cause a flood of PRI method logs
		// http://webconcepts.info/concepts/http-method/PRI
		NextProtos: []string{
			"http/1.1", "acme-tls/1",
		},
	}
	ln, err := tls.Listen("tcp", ":443", cfg)
	if err != nil {
		log.Fatal(err)
	}

	return ln
}
