package main

type book struct {
	Title       string
	Author      string
	Description string
	Language    string
}

func (b book) Type() string {
	return b.Language
}
