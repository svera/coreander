package main

import "github.com/alecthomas/kong"

// CLIInput stores all configuration flags and arguments thant can be passed to the application
type CLIInput struct {
	Version kong.VersionFlag `short:"v" name:"version" help:"Get version number."`
	// LibPath holds the absolute path to the folder containing the documents
	LibPath string `arg:"" env:"LIB_PATH" help:"Absolute path to the folder containing the documents." type:"path"`
	// CacheDir defines where cache files will be stored
	CacheDir string `env:"CACHE_DIR" short:"c" name:"cache-dir" help:"Directory where to store cache files. Defaults to ~/.coreander/cache"`
	// FQDN stores the domain name of the server. If the server is listening on a non-standard HTTP / HTTPS port, include it using a colon (e. g. example.com:3000)
	FQDN string `env:"FQDN" short:"d" default:"localhost" name:"fqdn" help:"Domain name of the server. If the server is listening on a non-standard HTTP / HTTPS port, include it using a colon (e. g. example:3000)"`
	// Port defines the port number in which the webserver listens for requests
	Port int `env:"PORT" short:"p" default:"3000" name:"port" help:"Port number in which the webserver listens for requests"`
	// BatchSize indicates the number of documents persisted by the indexer in one operation
	BatchSize int `env:"BATCH_SIZE" short:"b" default:"100" name:"batch-size" help:"Number of documents persisted by the indexer in one operation"`
	// AuthorImageMaxWidth sets the maximum horizontal size for author images in pixels. Set to 0 to keep original image size
	AuthorImageMaxWidth int `env:"AUTHOR_IMAGE_MAX_WIDTH" default:"600" name:"author-image-max-width" help:"Maximum horizontal size for author images in pixels. Set to 0 to keep original image size"`
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
	// InvitationTimeout specifies the maximum time a user invitation link may last in hours
	InvitationTimeout float64 `env:"INVITATION_TIMEOUT" default:"72" name:"invitation-timeout" help:"Maximum time a user invitation link may last in hours"`
	// UploadDocumentMaxSize is the maximum document size allowed to be uploaded to the library, in megabytes.
	// Set this to 0 to unlimit upload size. Defaults to 20 megabytes.
	UploadDocumentMaxSize int `env:"UPLOAD_DOCUMENT_MAX_SIZE" short:"u" default:"20" name:"upload-document-max-size" help:"Maximum document size allowed to be uploaded to the library, in megabytes. Set this to 0 to unlimit upload size."`
	// ClientStaticCacheTTL defines the cache duration for static assets (CSS, JS, images) in seconds. Defaults to 1 year.
	ClientStaticCacheTTL int `env:"CLIENT_STATIC_CACHE_TTL" default:"31536000" name:"client-static-cache-ttl" help:"Client-side cache duration for static assets (CSS, JS, images) in seconds. Defaults to 1 year (31536000 seconds)."`
	// ClientDynamicImageCacheTTL defines the cache duration for dynamically generated images (covers, author images) in seconds. Defaults to 24 hours.
	ClientDynamicImageCacheTTL int `env:"CLIENT_DYNAMIC_IMAGE_CACHE_TTL" default:"86400" name:"client-dynamic-image-cache-ttl" help:"Client-side cache duration for dynamically generated images (covers, author images) in seconds. Defaults to 24 hours (86400 seconds)."`
	// ServerStaticCacheTTL defines the server-side cache duration for static assets (CSS, JS, images) in seconds. Defaults to 1 year.
	ServerStaticCacheTTL int `env:"SERVER_STATIC_CACHE_TTL" default:"31536000" name:"server-static-cache-ttl" help:"Server-side cache duration for static assets (CSS, JS, images) in seconds. Defaults to 1 year (31536000 seconds)."`
	// ServerDynamicImageCacheTTL defines the server-side cache duration for dynamically generated images (covers, author images) in seconds. Defaults to 24 hours.
	ServerDynamicImageCacheTTL int `env:"SERVER_DYNAMIC_IMAGE_CACHE_TTL" default:"86400" name:"server-dynamic-image-cache-ttl" help:"Server-side cache duration for dynamically generated images (covers, author images) in seconds. Defaults to 24 hours (86400 seconds)."`
	// ShareCommentMaxSize defines the maximum length for share comments in characters. Defaults to 280.
	ShareCommentMaxSize int `env:"SHARE_COMMENT_MAX_SIZE" short:"m" default:"280" name:"share-comment-max-size" help:"Maximum length for share comments in characters. Defaults to 280."`
}
