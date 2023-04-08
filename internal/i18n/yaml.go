package i18n

import (
	"golang.org/x/text/message/catalog"
	"gopkg.in/yaml.v2"
)

type yamlDictionary struct {
	Entries map[string]string
}

func (d *yamlDictionary) Lookup(key string) (data string, ok bool) {
	if value, ok := d.Entries[key]; ok {
		// \x02 is ASCII code for hex 02, which is STX (start of text)
		return "\x02" + value, true
	}
	return "", false
}

func ParseDict(file []byte) (catalog.Dictionary, error) {
	data := map[string]string{}
	err := yaml.Unmarshal(file, &data)
	if err != nil {
		return nil, err
	}
	return &yamlDictionary{Entries: data}, nil
}
