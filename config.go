package main

// Config stores the application configuration
type Config struct {
	LibPath       string `env:"LIBPATH" env-required:"true"`
	Port          string `env:"PORT" env-default:"3000"`
	BatchSize     int    `env:"BATCHSIZE" env-default:"100"`
	CoverMaxWidth int    `env:"COVERMAXWIDTH" env-default:"300"`
	SkipReindex   bool   `env:"SKIPREINDEX" env-default:"false"`
	SmtpServer    string `env:"SMTPSERVER" env-default:"mail.gmx.com"`
	SmtpPort      int    `env:"SMTPPORT" env-default:"587"`
	SmtpUser      string `env:"SMTPUSER" env-default:"svera@gmx.us"`
	SmtpPassword  string `env:"SMTPPASSWORD" env-default:"20K!30p4tr414"`
}
