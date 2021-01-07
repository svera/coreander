package index

type Results struct {
	Page       int
	TotalPages int
	Hits       map[string]Book
	TotalHits  int
}

type Reader interface {
	Search(keywords string, page, resultsPerPage int) (*Results, error)
	Count() (uint64, error)
}
