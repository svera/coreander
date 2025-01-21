package main

import "github.com/alecthomas/kong"

// CLIInput stores all configuration flags and arguments thant can be passed to the application
type CLIInput struct {
	Version kong.VersionFlag `short:"v" name:"version" help:"Get version number."`
	// LibPath holds the absolute path to the folder containing the documents
	LibPath string `arg:"" env:"LIB_PATH" help:"Absolute path to the folder containing the documents." type:"path"`
	// FQDN stores the domain name of the server. If the server is listening on a non-standard HTTP / HTTPS port, include it using a colon (e. g. example.com:3000)
	FQDN string `env:"FQDN" short:"d" default:"localhost" name:"fqdn" help:"Domain name of the server. If the server is listening on a non-standard HTTP / HTTPS port, include it using a colon (e. g. example:3000)"`
	// Port defines the port number in which the webserver listens for requests
	Port int `env:"PORT" short:"p" default:"3000" help:"Port number in which the webserver listens for requests"`
	// BatchSize indicates the number of documents persisted by the indexer in one operation
	BatchSize int `env:"BATCH_SIZE" short:"b" default:"100" name:"batch-size" help:"Number of documents persisted by the indexer in one operation"`
	// CoverMaxWidth sets the maximum horizontal size for documents cover thumbnails in pixels
	CoverMaxWidth int `env:"COVER_MAX_WIDTH" default:"600" name:"cover-max-width" help:"Maximum horizontal size for documents cover thumbnails in pixels"`
	// ForceIndexing signals whether to force indexing already indexed documents or not
	ForceIndexing bool `env:"FORCE_INDEXING" short:"f" default:"false" name:"force-indexing" help:"Force indexing already indexed documents"`
	// SmtpServer points to the address of the send mail server
	SmtpServer string `env:"SMTP_SERVER" name:"smtp-server" help:"Address of the send mail server"`
	// SmtpPort defines the port in which the mail server listens for requests
	SmtpPort int `env:"SMTP_PORT" default:"587" name:"smtp-port" help:"Port in which the mail server listens for requests"`
	// SmtpUser holds the user to authenticate against the SMTP server
	SmtpUser string `env:"SMTP_USER" name:"smtp-user" help:"User to authenticate against the SMTP server"`
	// SmtpUser holds the password to authenticate against the SMTP server
	SmtpPassword string `env:"SMTP_PASSWORD" name:"smtp-password" help:"Password to authenticate against the SMTP server"`
	// JwtSecret stores the string to use to sign JWTs
	JwtSecret string `env:"JWT_SECRET" short:"s" name:"jwt-secret" help:"String to use to sign JWTs"`
	// RequireAuth is a switch to enable the application to require authentication to access any route if true
	RequireAuth bool `env:"REQUIRE_AUTH" short:"a" default:"false" name:"require-auth" help:"Require authentication to access any route"`
	// MinPasswordLength is the minimum length acceptable for passwords
	MinPasswordLength int `env:"MIN_PASSWORD_LENGTH" default:"5" name:"min-password-length" help:"Minimum length acceptable for passwords"`
	// WordsPerMinute defines a default words per minute reading speed that will be used for not logged-in users
	WordsPerMinute float64 `env:"WORDS_PER_MINUTE" default:"250" name:"words-per-minute" help:"Default words per minute reading speed that will be used for not logged-in users"`
	// SessionTimeout specifies the maximum time a user session may last in hours
	SessionTimeout float64 `env:"SESSION_TIMEOUT" default:"24" name:"session-timeout" help:"Maximum time a user session may last in hours"`
	// RecoveryTimeout specifies the maximum time a user recovery link may last in hours
	RecoveryTimeout float64 `env:"RECOVERY_TIMEOUT" default:"2" name:"recovery-timeout" help:"Maximum time a user recovery link may last in hours"`
	// UploadDocumentMaxSize is the maximum document size allowed to be uploaded to the library, in megabytes.
	// Set this to 0 to unlimit upload size. Defaults to 20 megabytes.
	UploadDocumentMaxSize int `env:"UPLOAD_DOCUMENT_MAX_SIZE" short:"u" default:"20" name:"upload-document-max-size" help:"Maximum document size allowed to be uploaded to the library, in megabytes. Set this to 0 to unlimit upload size."`
}
