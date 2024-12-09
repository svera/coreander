package author

type info interface {
	Retrieve(name, lang string) (string, error)
}
