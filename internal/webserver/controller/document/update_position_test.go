package document

import "testing"

func TestParseReadingPositionJSONAlternateKeys(t *testing.T) {
	pos, prog, err := parseReadingPositionJSON([]byte(`{"Position":"epubcfi(/1)","Fraction":0.25}`))
	if err != nil {
		t.Fatal(err)
	}
	if pos != "epubcfi(/1)" {
		t.Fatalf("position %q", pos)
	}
	if prog == nil || *prog != 25 {
		t.Fatalf("progress %v", prog)
	}
}

func TestParseReadingPositionJSONStringFraction(t *testing.T) {
	pos, prog, err := parseReadingPositionJSON([]byte(`{"position":"x","fraction":"0.75"}`))
	if err != nil {
		t.Fatal(err)
	}
	if pos != "x" {
		t.Fatalf("position %q", pos)
	}
	if prog == nil || *prog != 75 {
		t.Fatalf("progress %v", prog)
	}
}
