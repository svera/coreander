package metadata

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"

	"github.com/gosimple/slug"
	"github.com/pirmd/epub"
)

const (
	WikidataAuthorIDsProperty = "cdr:wd_id"
	WikidataAuthorIDAttribute = "wd_id"
	CoreanderMetadataPrefix   = "https://github.com/svera/coreander/vocab/"
)

func (e EpubReader) WriteAuthorIDs(filename string, idsBySlug map[string]string) (bool, error) {
	book, err := epub.Open(filename)
	if err != nil {
		return false, err
	}
	info, err := book.Information()
	if err != nil {
		book.Close()
		return false, err
	}
	pkg, err := book.Package()
	closeErr := book.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}

	needsPrefix := false
	epub3 := strings.HasPrefix(pkg.Version, "3")
	attributeName := xml.Name{Space: CoreanderMetadataPrefix, Local: WikidataAuthorIDAttribute}
	targets := make(map[string]bool)
	updatePeople := func(people []epub.Author, elements []epub.AuthorElt, fromCreator bool) bool {
		changed := false
		for i := range people {
			person := &people[i]
			var authors, illustrators []string
			classifyEpubPerson(*person, fromCreator, &authors, &illustrators)
			var ids []string
			matched := false
			for _, name := range append(authors, illustrators...) {
				if id, ok := idsBySlug[slug.Make(name)]; ok {
					matched = true
					if id != "" && !slices.Contains(ids, id) {
						ids = append(ids, id)
					}
				}
			}
			if !matched {
				continue
			}
			if i < len(elements) && elements[i].ID != "" {
				targets["#"+elements[i].ID] = true
			}
			value := strings.Join(ids, " ")
			needsPrefix = needsPrefix || value != ""
			existing := 0
			current := ""
			legacyAttribute := false
			if epub3 {
				for _, refinement := range person.Refinements {
					if refinement.Property == WikidataAuthorIDsProperty {
						existing++
						current = refinement.Value
					}
				}
			} else {
				for _, attribute := range person.Attributes {
					if attribute.Name == (xml.Name{Local: WikidataAuthorIDAttribute}) {
						legacyAttribute = true
					}
					if attribute.Name == attributeName {
						existing++
						current = attribute.Value
					}
				}
			}
			if !legacyAttribute && ((value == "" && existing == 0) || (value != "" && existing == 1 && current == value)) {
				continue
			}
			if epub3 {
				person.Refinements = slices.DeleteFunc(person.Refinements, func(m epub.Meta) bool {
					return m.Property == WikidataAuthorIDsProperty
				})
				if value != "" {
					person.Refinements = append(person.Refinements, epub.Meta{
						Property: WikidataAuthorIDsProperty, Value: value,
					})
				}
			} else {
				person.Attributes = slices.DeleteFunc(person.Attributes, func(a xml.Attr) bool {
					return a.Name == attributeName || a.Name == (xml.Name{Local: WikidataAuthorIDAttribute})
				})
				if value != "" {
					person.Attributes = append(person.Attributes, xml.Attr{
						Name: attributeName, Value: value,
					})
				}
			}
			changed = true
		}
		return changed
	}

	updates := &epub.Information{}
	creatorsChanged := updatePeople(info.Creator, pkg.Metadata.Creator, true)
	contributorsChanged := updatePeople(info.Contributor, pkg.Metadata.Contributor, false)
	retained := make([]epub.GenericMetadata, 0, len(info.Meta))
	legacyChanged := false
	for _, meta := range info.Meta {
		if meta.Name == "epub:refinement" && len(targets) > 0 {
			var refinement epub.Meta
			if err := json.Unmarshal([]byte(meta.Content), &refinement); err != nil {
				return false, fmt.Errorf("decoding legacy EPUB refinement: %w", err)
			}
			if targets[refinement.Refines] && (refinement.Property == WikidataAuthorIDsProperty || refinement.Property == "cdr:wd_author_ids") {
				legacyChanged = true
				continue
			}
		}
		retained = append(retained, meta)
	}
	if legacyChanged {
		updates.Meta = retained
	}
	prefixChanged := needsPrefix && epub3 && info.Prefixes["cdr"] != CoreanderMetadataPrefix
	if !creatorsChanged && !contributorsChanged && !prefixChanged && !legacyChanged {
		return false, nil
	}
	if creatorsChanged {
		updates.Creator = info.Creator
	}
	if contributorsChanged {
		updates.Contributor = info.Contributor
	}
	if prefixChanged {
		updates.Prefixes = map[string]string{"cdr": CoreanderMetadataPrefix}
	}
	if !epub3 && needsPrefix {
		updates.Namespaces = map[string]string{"cdr": CoreanderMetadataPrefix}
	}
	if err := epub.WriteMetadataToFile(filename, updates); err != nil {
		return false, err
	}
	return true, nil
}
