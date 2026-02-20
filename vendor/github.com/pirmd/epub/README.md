# EPUB
[![GoDoc](https://godoc.org/github.com/pirmd/epub?status.svg)](https://godoc.org/github.com/pirmd/epub)&nbsp; 
[![Go Report Card](https://goreportcard.com/badge/github.com/pirmd/epub)](https://goreportcard.com/report/github.com/pirmd/epub)&nbsp;

`epub` package provides a way to retrieve stored metadata from epub files.

`epub` package offers also a minimal tool to print to the standard output the
metadata of the given epub file.

# INSTALLATION
Everything should work fine using go standard commands (`build`, `get`,
`install`...).

To install the metadata reading utility, run `go install ./cmd/epub`.

# USAGE
Running `godoc` should give you helpful guidelines on available features.

Metadata reading utility usage is straightforward, just type `epub <epub>`, where
'<epub>' is the path to the epub file you want to read metadata from.

# CONTRIBUTION
If you feel like to contribute, just follow github guidelines on
[forking](https://help.github.com/articles/fork-a-repo/) then [send a pull
request](https://help.github.com/articles/creating-a-pull-request/)

[modeline]: # ( vim: set fenc=utf-8 spell spl=en: )
