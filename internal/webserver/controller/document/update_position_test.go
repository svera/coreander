package document

import "testing"

func TestParseReadingPositionJSONAlternateKeys(t *testing.T) {
	pos, frac, err := parseReadingPositionJSON([]byte(`{"Position":"epubcfi(/1)","Fraction":0.25}`))
	if err != nil {
		t.Fatal(err)
	}
	if pos != "epubcfi(/1)" {
		t.Fatalf("position %q", pos)
	}
	if frac == nil || *frac != 0.25 {
		t.Fatalf("fraction %v, want 0.25", frac)
	}
}

func TestParseReadingPositionJSONStringFraction(t *testing.T) {
	pos, frac, err := parseReadingPositionJSON([]byte(`{"position":"x","fraction":"0.75"}`))
	if err != nil {
		t.Fatal(err)
	}
	if pos != "x" {
		t.Fatalf("position %q", pos)
	}
	if frac == nil || *frac != 0.75 {
		t.Fatalf("fraction %v, want 0.75", frac)
	}
}
