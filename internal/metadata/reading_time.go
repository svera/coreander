package metadata

import (
	"fmt"
	"time"
)

func CalculateReadingTime(words, wordsPerMinute float64) string {
	if words == 0.0 {
		return ""
	}
	if readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", words/wordsPerMinute)); err == nil {
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
