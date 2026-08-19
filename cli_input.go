package main

import (
	"fmt"

	"github.com/alecthomas/kong"
)

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
	// IndexWorkers is the number of goroutines used to extract metadata in parallel during indexing. 0 (default) chooses CPU count; 1 is sequential; 2+ sets an explicit pool size.
	IndexWorkers int `env:"INDEX_WORKERS" name:"index-workers" default:"0" help:"Parallel workers for metadata extraction during indexing. 0 = automatic (CPU count); 1 = sequential; 2+ = explicit pool size."`
	// AuthorImageMaxWidth sets the maximum horizontal size for author images in pixels. Set to 0 to keep original image size
	AuthorImageMaxWidth int `env:"AUTHOR_IMAGE_MAX_WIDTH" default:"600" name:"author-image-max-width" help:"Maximum horizontal size for author images in pixels. Set to 0 to keep original image size"`
	// CoverMaxWidth sets the maximum horizontal size for documents cover thumbnails in pixels
	CoverMaxWidth int `env:"COVER_MAX_WIDTH" default:"600" name:"cover-max-width" help:"Maximum horizontal size for documents cover thumbnails in pixels"`
	// CacheMaxSize sets the maximum total size of the cache directory in megabytes. Set to 0 for unlimited.
	CacheMaxSize int `env:"CACHE_MAX_SIZE" default:"500" name:"cache-max-size" help:"Maximum total size of the cache directory in megabytes. Set to 0 for unlimited."`
	// IllustratedMinAmount is the minimum number of illustrations (excluding cover) for a document to be considered illustrated
	IllustratedMinAmount int `env:"ILLUSTRATED_MIN_AMOUNT" default:"2" name:"illustrated-min-amount" help:"Minimum number of illustrations (excluding cover) for a document to be considered illustrated"`
	// IllustratedMinSize is the minimum size in megapixels for an image to count as an illustration
	IllustratedMinSize float64 `env:"ILLUSTRATED_MIN_SIZE" default:"0.25" name:"illustrated-min-size" help:"Minimum size in megapixels for an image to count as an illustration"`
	// MinOccurrenceRatio is the minimum fraction of the most frequent phrase's (or word's) occurrence count that a phrase or single word must reach to be kept as a search/related-document keyword for EPUB documents. A value of 0 disables text ranking entirely. Must be between 0 and 1, both included; see CLIInput.Validate.
	MinOccurrenceRatio float64 `env:"MIN_OCCURRENCE_RATIO" default:"0.1" name:"min-occurrence-ratio" help:"Minimum fraction of the most frequent phrase's (or word's) occurrence count that a phrase or single word must reach to be kept as a search/related-document keyword for EPUB documents. Set to 0 to disable text ranking. Must be between 0 and 1."`
	// MaxSimilarityCandidates caps how many top-scoring matches a "similar document" query considers before applying MinSimilarityScoreRatio and paginating. Higher: fewer genuinely similar documents cut off before scoring, at the cost of more matches to score and rank per query. Lower: faster queries, but a genuinely similar document can be missed if too many weaker matches outscore it into the discarded tail.
	MaxSimilarityCandidates int `env:"MAX_SIMILARITY_CANDIDATES" default:"200" name:"max-similarity-candidates" help:"Maximum number of top-scoring matches a \"similar document\" query considers before pruning by min-similarity-score-ratio and paginating. Higher values reduce the chance of missing a genuinely similar document at the cost of slower queries; lower values speed queries up but risk missing weaker true matches."`
	// MinSimilarityScoreRatio is the minimum fraction of the best match's score a document must reach to be considered similar enough to show in a "similar document" query. Higher: stricter, fewer but more relevant results (a "similar" list can end up empty). Lower: more results shown, at the risk of weak, coincidental matches. Must be between 0 and 1, both included; see CLIInput.Validate.
	MinSimilarityScoreRatio float64 `env:"MIN_SIMILARITY_SCORE_RATIO" default:"0.3" name:"min-similarity-score-ratio" help:"Minimum fraction of the best match's score a document must reach to be considered similar enough to show in a \"similar document\" query. Higher values give fewer but more relevant results (can end up empty); lower values show more results but risk weak, coincidental matches. Must be between 0 and 1."`
	// MaxSimilarityPhrases caps how many of a document's TextRank phrases are used, at most, to find "similar" documents. Must match index.defaultMaxSimilarityPhrases, since kong's default tag can't reference a Go constant. Set to 0 to disable the cap. Higher: more accurate similarity matching for documents with many phrases, at the cost of slower queries (Bleve evaluates one OR clause per phrase for every candidate document); a document with hundreds of phrases and no cap can make "similar document" queries slow enough to noticeably affect the whole app, and has been observed to trigger a rare bleve/zapx crash under concurrent indexing. Lower: faster, safer queries, but a document with more phrases than the cap only gets matched on its top ones (see Document.TextRankPhrases for how they're ordered).
	MaxSimilarityPhrases int `env:"MAX_SIMILARITY_PHRASES" default:"60" name:"max-similarity-phrases" help:"Maximum number of a document's TextRank phrases used to find \"similar\" documents. Set to 0 to disable the cap. Higher values improve matching accuracy for documents with many phrases but slow queries down; lower values are faster but only match on a document's top phrases."`
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
	// ShareMaxRecipients defines the maximum number of recipients allowed when sharing a document. Defaults to 10.
	ShareMaxRecipients int `env:"SHARE_MAX_RECIPIENTS" default:"10" name:"share-max-recipients" help:"Maximum number of recipients allowed when sharing a document. Defaults to 10."`
	// InviteEmailListMaxLength is the maximum length (in bytes) of the comma-separated invite email field. Defaults to 2000.
	InviteEmailListMaxLength int `env:"INVITE_EMAIL_LIST_MAX_LENGTH" default:"2000" name:"invite-email-list-max-length" help:"Maximum length in bytes of the invitation email list field. Defaults to 2000."`
	// InviteMaxRecipients is the maximum number of distinct addresses per invitation submit. Defaults to 50.
	InviteMaxRecipients int `env:"INVITE_MAX_RECIPIENTS" default:"50" name:"invite-max-recipients" help:"Maximum number of distinct email addresses per invitation form submit. Defaults to 50."`
}

// Validate is called by kong.Parse after parsing flags/env vars, to reject
// values that would otherwise reach the indexer/webserver and cause a crash
// or a silently broken feature instead of a clear startup error.
func (c CLIInput) Validate() error {
	if c.MinOccurrenceRatio < 0 || c.MinOccurrenceRatio > 1 {
		return fmt.Errorf("min-occurrence-ratio must be between 0 and 1, got %v", c.MinOccurrenceRatio)
	}
	if c.MinSimilarityScoreRatio < 0 || c.MinSimilarityScoreRatio > 1 {
		return fmt.Errorf("min-similarity-score-ratio must be between 0 and 1, got %v", c.MinSimilarityScoreRatio)
	}
	// BatchSize must be positive: 0 makes AddLibrary/EnrichTextRankKeywords's
	// chunking loop spin forever (chunkStart never advances), and negative
	// values panic on an invalid slice bound.
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch-size must be greater than 0, got %v", c.BatchSize)
	}
	// MaxSimilarityCandidates must not be negative: it's passed straight to
	// bleve's search request size and a negative size panics inside bleve's
	// collector setup.
	if c.MaxSimilarityCandidates < 0 {
		return fmt.Errorf("max-similarity-candidates must not be negative, got %v", c.MaxSimilarityCandidates)
	}
	// ShareCommentMaxSize must not be negative: a negative value is used
	// directly as a slice upper bound when truncating share comments, which
	// panics.
	if c.ShareCommentMaxSize < 0 {
		return fmt.Errorf("share-comment-max-size must not be negative, got %v", c.ShareCommentMaxSize)
	}
	// MinPasswordLength must be positive: 0 or negative disables the minimum
	// length check entirely, silently allowing empty passwords.
	if c.MinPasswordLength <= 0 {
		return fmt.Errorf("min-password-length must be greater than 0, got %v", c.MinPasswordLength)
	}
	// ShareMaxRecipients must be positive: 0 or negative disables the sharing
	// feature entirely (every share request is rejected as over the limit),
	// which isn't a documented "disable" sentinel.
	if c.ShareMaxRecipients <= 0 {
		return fmt.Errorf("share-max-recipients must be greater than 0, got %v", c.ShareMaxRecipients)
	}
	// SessionTimeout, RecoveryTimeout and InvitationTimeout must be positive:
	// a non-positive value produces an already-expired session/link at
	// creation time.
	if c.SessionTimeout <= 0 {
		return fmt.Errorf("session-timeout must be greater than 0, got %v", c.SessionTimeout)
	}
	if c.RecoveryTimeout <= 0 {
		return fmt.Errorf("recovery-timeout must be greater than 0, got %v", c.RecoveryTimeout)
	}
	if c.InvitationTimeout <= 0 {
		return fmt.Errorf("invitation-timeout must be greater than 0, got %v", c.InvitationTimeout)
	}
	// IllustratedMinAmount must not be negative: a negative value makes the
	// "ge .Document.Illustrations .IllustratedMinAmount" template check
	// always true, showing the "illustrated" badge on every document.
	if c.IllustratedMinAmount < 0 {
		return fmt.Errorf("illustrated-min-amount must not be negative, got %v", c.IllustratedMinAmount)
	}
	// UploadDocumentMaxSize must not be negative: 0 is a documented sentinel
	// for "unlimited", but a negative value has no valid meaning.
	if c.UploadDocumentMaxSize < 0 {
		return fmt.Errorf("upload-document-max-size must not be negative, got %v", c.UploadDocumentMaxSize)
	}
	return nil
}
