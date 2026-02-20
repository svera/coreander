package epub

import (
	"encoding/xml"
	"io"

	"golang.org/x/net/html/charset"
)

func decodeXML(r io.Reader, v interface{}) error {
	decoder := xml.NewDecoder(r)
	decoder.Entity = xml.HTMLEntity
	decoder.CharsetReader = charset.NewReaderLabel
	return decoder.Decode(v)
}
