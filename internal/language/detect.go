package language

import (
	"slices"
	"strings"

	"github.com/pemistahl/lingua-go"
)

var languages = []lingua.Language{
	lingua.Spanish,
	lingua.English,
	lingua.German,
	lingua.French,
	lingua.Italian,
	lingua.Portuguese,
}

func Detect(text string) string {
	detector := lingua.NewLanguageDetectorBuilder().
		FromLanguages(languages...).
		Build()

	if language, exists := detector.DetectLanguageOf(text); exists {
		if slices.Contains(languages, language) {
			return strings.ToLower(language.IsoCode639_1().String())
		}
	}

	return ""
}
