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
		return fmtDuration(readingTime)
	}
	return ""
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	return fmt.Sprintf("%dh %dm", h, m)
}

type Reader interface {
	Metadata(file string) (Metadata, error)
	Cover(documentFullPath string, coverMaxWidth int) ([]byte, error)
}
