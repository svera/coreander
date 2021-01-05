package index

type Book struct {
	Title       string
	Author      string
	Description string
	Language    string
}

func (b Book) Type() string {
	return b.Language
}
