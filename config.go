package main

// Config stores the application configuration
type Config struct {
	LibPath       string `env:"LIBPATH" env-required:"true"`
	Port          string `env:"PORT" env-default:"3000"`
	BatchSize     int    `env:"BATCHSIZE" env-default:"100"`
	CoverMaxWidth int    `env:"COVERMAXWIDTH" env-default:"300"`
}
