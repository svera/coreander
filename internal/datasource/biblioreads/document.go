package biblioreads

// Source retains the persisted key for Goodreads data fetched through BiblioReads.
const Source = "goodreads"

// DocumentData is the persisted Goodreads state used by BiblioReads.
type DocumentData struct {
	URL          string `json:"url,omitempty"`
	RatingCount  string `json:"ratingCount,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`
	LegacyBookID int    `json:"legacyBookId,omitempty"`
	Enriched     bool   `json:"enriched"`
}
