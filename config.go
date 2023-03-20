package main

import "time"

// Config stores the application configuration
type Config struct {
	// LibPath holds the absolute path to the folder containing the documents
	LibPath string `env:"LIB_PATH" env-required:"true"`
	// Hostname stores the name of the host the server is running on
	Hostname string `env:"HOSTNAME" env-default:"localhost"`
	// Port defines the port number in which the webserver listens for requests
	Port int `env:"PORT" env-default:"3000"`
	// BatchSize indicates the number of documents persisted by the indexer in one operation
	BatchSize int `env:"BATCH_SIZE" env-default:"100"`
	// CoverMaxWidth sets the maximum horizontal size for documents cover thumbnails in pixels
	CoverMaxWidth int `env:"COVER_MAX_WIDTH" env-default:"300"`
	// SkipIndexing signals whether to bypass the indexing process or not
	SkipIndexing bool `env:"SKIP_INDEXING" env-default:"false"`
	// SmtpServer points to the address of the send mail server
	SmtpServer string `env:"SMTP_SERVER"`
	// SmtpPort defines the port in which the mail server listens for requests
	SmtpPort int `env:"SMTP_PORT" env-default:"587"`
	// SmtpUser holds the user to authenticate against the SMTP server
	SmtpUser string `env:"SMTP_USER"`
	// SmtpUser holds the password to authenticate against the SMTP server
	SmtpPassword string `env:"SMTP_PASSWORD"`
	// JwtSecret stores the string to use to sign JWTs
	JwtSecret []byte `env:"JWT_SECRET"`
	// RequireAuth is a switch to enable the application to require authentication to access any route if true
	RequireAuth bool `env:"REQUIRE_AUTH" env-default:"false"`
	// MinPasswordLength is the minimum length acceptable for passwords
	MinPasswordLength int `env:"MIN_PASSWORD_LENGTH" env-default:"5"`
	// WordsPerMinute defines a default words per minute reading speed that will be used for not logged-in users
	WordsPerMinute float64 `env:"WORDS_PER_MINUTE" env-default:"250"`
	// SessionTimeout specifies the maximum time a user session may last in hours
	SessionTimeout time.Duration `env:"SESSION_TIMEOUT" env-default:"24h"`
}
