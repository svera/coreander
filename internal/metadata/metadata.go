package metadata

import (
	"fmt"
	"html/template"
	"image"
	"time"

	"github.com/svera/coreander/v5/internal/precisiondate"
)

type Metadata struct {
	Title         string
	Authors       []string
	Illustrators  []string
	Description   template.HTML
	Language      string
	Publication   precisiondate.PrecisionDate
	Words         float64
	Series        string
	SeriesIndex   float64
	Pages         float64
	Format        string
	Subjects      []string
	Illustrations int
}

func (m Metadata) ReadingTime(wordsPerMinute float64) string {
	if m.Words == 0.0 || wordsPerMinute <= 0.0 {
		return ""
	}
	if readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", m.Words/wordsPerMinute)); err == nil {
		return FmtDuration(readingTime)
	}
	return ""
}

// FmtDuration formats a duration as "Xd Yh Zm" or "Yh Zm" if less than 24 hours
func FmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)

	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour

	h := d / time.Hour
	d -= h * time.Hour

	m := d / time.Minute

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

type Reader interface {
	Metadata(file string) (Metadata, error)
	Cover(documentFullPath string, coverMaxWidth int) (image.Image, error)
}

// TextExtractor is implemented by Reader types that can return their full
// extracted text content alongside Metadata in a single pass (currently just
// EpubReader). Callers that also need a TextRanker's analysis right after
// reading Metadata (e.g. indexing a single uploaded file) should use this
// instead of Metadata, and feed the returned text into RankText, to avoid
// extracting and sanitizing the same document's content twice.
type TextExtractor interface {
	MetadataAndText(file string) (Metadata, string, error)
}

// TextSource is implemented by Reader types that can extract just their text
// content (currently just EpubReader), without repeating the metadata/OPF
// parsing and illustration counting that TextExtractor's MetadataAndText also
// does. Callers that already have Metadata and only need text for TextRanker
// (e.g. background enrichment run well after indexing) should use this
// instead of TextExtractor, to avoid redoing that unrelated work.
type TextSource interface {
	Text(file string) (string, error)
}
