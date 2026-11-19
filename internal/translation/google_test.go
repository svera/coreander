package translation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gopkg.gilang.dev/translator/v2/googletranslate"
)

type fakeClient struct {
	requests []string
	genders  []googletranslate.Gender
	detected string
	err      error
	empty    bool
}

func (f *fakeClient) TranslateWithGender(_ context.Context, text, _, _ string, gender googletranslate.Gender) (*googletranslate.Translated, error) {
	f.requests = append(f.requests, text)
	f.genders = append(f.genders, gender)
	if f.err != nil {
		return nil, f.err
	}
	if f.empty {
		return &googletranslate.Translated{}, nil
	}
	result := &googletranslate.Translated{Text: strings.ToUpper(text)}
	result.From.Language.Iso = f.detected
	return result, nil
}

func TestTranslatePassesTextUnchanged(t *testing.T) {
	client := &fakeClient{detected: "en-US"}
	text := "He wrote \"It\".\\\n\n" + strings.Repeat("A long sentence. ", 400)
	translated, from, err := (&Google{client: client}).Translate(context.Background(), text, AutoDetect, "es", GenderUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || client.requests[0] != text {
		t.Fatal("text must reach the library unchanged in one call")
	}
	if translated != strings.ToUpper(text) || from != "en" {
		t.Fatalf("unexpected translation %q from %q", translated, from)
	}
}

func TestTranslateKeepsGivenSourceLanguage(t *testing.T) {
	client := &fakeClient{detected: "fr"}
	_, from, err := (&Google{client: client}).Translate(context.Background(), "Hola", "es", "en", GenderUnknown)
	if err != nil || from != "es" {
		t.Fatalf("expected the given source language, got %q, %v", from, err)
	}
}

func TestTranslateMapsGender(t *testing.T) {
	for _, tt := range []struct {
		gender Gender
		want   googletranslate.Gender
	}{
		{GenderUnknown, googletranslate.GenderUnknown},
		{GenderMasculine, googletranslate.GenderMasculine},
		{GenderFeminine, googletranslate.GenderFeminine},
	} {
		client := &fakeClient{}
		if _, _, err := (&Google{client: client}).Translate(context.Background(), "Hello", "en", "es", tt.gender); err != nil {
			t.Fatal(err)
		}
		if len(client.genders) != 1 || client.genders[0] != tt.want {
			t.Errorf("gender %v: got %v, want %v", tt.gender, client.genders, tt.want)
		}
	}
}

func TestTranslateErrors(t *testing.T) {
	if _, _, err := (&Google{client: &fakeClient{err: errors.New("boom")}}).Translate(context.Background(), "Hi", AutoDetect, "es", GenderUnknown); err == nil {
		t.Error("expected client errors to be returned")
	}
	if _, _, err := (&Google{client: &fakeClient{}}).Translate(context.Background(), "Hi", AutoDetect, "es", GenderUnknown); err == nil {
		t.Error("expected an error when the source language is not detected")
	}
	if _, _, err := (&Google{client: &fakeClient{empty: true}}).Translate(context.Background(), "Hi", "en", "es", GenderUnknown); err == nil {
		t.Error("an empty translation must not be accepted")
	}
}

func TestNormalizeLanguage(t *testing.T) {
	for code, want := range map[string]string{"en": "en", "zh-CN": "zh", " PT ": "pt", "": ""} {
		if got := NormalizeLanguage(code); got != want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", code, got, want)
		}
	}
}
