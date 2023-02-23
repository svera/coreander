package main

// Config stores the application configuration
type Config struct {
	// LibPath holds the absolute path to the folder containing the documents
	LibPath string `env:"LIBPATH" env-required:"true"`
	// Port defines the port number in which the webserver listens for requests
	Port string `env:"PORT" env-default:"3000"`
	// BatchSize indicates the number of documents persisted by the indexer in one operation
	BatchSize int `env:"BATCHSIZE" env-default:"100"`
	// CoverMaxWidth sets the maximum horizontal size for documents cover thumbnails in pixels
	CoverMaxWidth int `env:"COVERMAXWIDTH" env-default:"300"`
	// SkipReindex signals whether to bypass the indexing process or not
	SkipReindex bool `env:"SKIPREINDEX" env-default:"false"`
	// SmtpServer points to the address of the send mail server
	SmtpServer string `env:"SMTPSERVER"`
	// SmtpPort defines the port in which the mail server listens for requests
	SmtpPort int `env:"SMTPPORT" env-default:"587"`
	// SmtpUser holds the user to authenticate against the SMTP server
	SmtpUser string `env:"SMTPUSER"`
	// SmtpUser holds the password to authenticate against the SMTP server
	SmtpPassword string `env:"SMTPPASSWORD"`
	// JwtSecret stores the string to use to sign JWTs
	JwtSecret []byte `env:"JWT_SECRET"`
	// RequireAuth is a switch to enable the application to require authentication to access any route if true
	RequireAuth bool `env:"REQUIRE_AUTH" env-default:"false"`
	// MinPasswordLength is the minimum length acceptable for passwords
	MinPasswordLength int `env:"MINPASSWORDLENGTH" env-default:"5"`
}
