package index

import "time"

type Progress struct {
	InProgress    bool
	RemainingTime time.Duration
	Percentage    float64
}
