package main

// Config stores the application configuration
type Config struct {
	LibPath       string `env:"LIBPATH" env-required:"true"`
	Port          string `env:"PORT" env-default:"3000"`
	BatchSize     int    `env:"BATCHSIZE" env-default:"100"`
	CoverMaxWidth int    `env:"COVERMAXWIDTH" env-default:"300"`
	SkipReindex   bool   `env:"SKIPREINDEX" env-default:"false"`
	SmtpServer    string `env:"SMTPSERVER"`
	SmtpPort      int    `env:"SMTPPORT" env-default:"587"`
	SmtpUser      string `env:"SMTPUSER"`
	SmtpPassword  string `env:"SMTPPASSWORD"`
	JwtSecret     []byte `env:"JWT_SECRET"`
	RequireAuth   bool   `env:"REQUIRE_AUTH" env-default:"false"`
}
