package main

type Config struct {
	LibraryPath string `yaml:"library-path" env:"LIBPATH"`
	Port        string `yaml:"port" env:"PORT" env-default:"3000"`
	BatchSize   int    `yaml:"batch-size" env:"BATCHSIZE" env-default:"100"`
	Verbose     bool   `env:"VERBOSE" env-default:"false"`
}
