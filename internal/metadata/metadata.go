package metadata

import (
	"fmt"
	"html/template"
	"time"

	"github.com/svera/coreander/v4/internal/precisiondate"
)

type Metadata struct {
	Title       string
	Authors     []string
	Description template.HTML
	Language    string
	Publication precisiondate.PrecisionDate
	Words       float64
	Series      string
	SeriesIndex float64
	Pages       float64
	Format      string
	Subjects    []string
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
	Cover(documentFullPath string, coverMaxWidth int) ([]byte, error)
	// Illustrations returns the number of images that count as illustrations (excluding cover, size >= minMegapixels).
	Illustrations(documentFullPath string, minMegapixels float64) (int, error)
}
