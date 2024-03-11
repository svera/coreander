package index

import "time"

type Progress struct {
	RemainingTime time.Duration
	Percentage    float64
}
