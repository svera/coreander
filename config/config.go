package config

type Config struct {
	LibraryPath string `yaml:"library-path" env:"LIBPATH"`
	Port        string `yaml:"port" env:"PORT" env-default:":3000"`
}
