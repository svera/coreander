package translation

import (
	"context"
	"encoding/json"
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
	var decoded string
	if err := json.Unmarshal([]byte(`"`+text+`"`), &decoded); err != nil {
		return nil, err
	}
	result := &googletranslate.Translated{Text: strings.ReplaceAll(strings.ToUpper(decoded), "\n", " \n ")}
	result.From.Language.Iso = f.detected
	return result, nil
}

func TestTranslateEscapesTextAndKeepsLineBreaks(t *testing.T) {
	client := &fakeClient{detected: "en"}
	translated, from, err := (&Google{client: client}).Translate(context.Background(), "He wrote \"It\".\n\nThe end", AutoDetect, "es", GenderUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || client.requests[0] != `He wrote \"It\".\n\nThe end` {
		t.Fatalf("text must be JSON-escaped in one request, got %q", client.requests)
	}
	if translated != "HE WROTE \"IT\".\n\nTHE END" || from != "en" {
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

func TestTranslateKeepsGenderAcrossChunks(t *testing.T) {
	client := &fakeClient{}
	if _, _, err := (&Google{client: client}).Translate(context.Background(), strings.Repeat("A long sentence. ", 400), "en", "es", GenderMasculine); err != nil {
		t.Fatal(err)
	}
	if len(client.genders) < 2 {
		t.Fatal("expected more than one translation request")
	}
	for _, gender := range client.genders {
		if gender != googletranslate.GenderMasculine {
			t.Fatalf("gender was not preserved across chunks: %v", client.genders)
		}
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

func TestChunks(t *testing.T) {
	text := "First line.\n\nSecond sentence. Third sentence is long.\nLast"
	for _, size := range []int{5, 12, 20, 1000} {
		parts := chunks(text, size)
		for _, part := range parts {
			if len(part) > size && size >= 12 {
				t.Errorf("size %d: chunk %q is too long", size, part)
			}
		}
		if size >= 1000 && (len(parts) != 1 || parts[0] != text) {
			t.Errorf("short texts must not be split, got %q", parts)
		}
		joined := strings.Join(parts, "\n")
		if strings.Count(joined, "\n\n") < 1 || strings.ReplaceAll(strings.ReplaceAll(joined, "\n", ""), " ", "") != strings.ReplaceAll(strings.ReplaceAll(text, "\n", ""), " ", "") {
			t.Errorf("size %d: chunks must keep the text and its paragraphs, got %q", size, parts)
		}
	}
	if parts := chunks("ñññ", 3); strings.Join(parts, "") != "ñññ" {
		t.Errorf("chunks must not split runes, got %q", parts)
	}
}
