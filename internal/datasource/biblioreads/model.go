package biblioreads

type SearchBooksResponse struct {
	Status          string         `json:"status"`
	Source          string         `json:"source"`
	APIURL          string         `json:"apiURL"`
	SearchType      string         `json:"searchType"`
	NumberOfResults string         `json:"numberOfResults"`
	Result          []SearchResult `json:"result"`
	LastScraped     string         `json:"lastScraped"`
}

type SearchResult struct {
	ID        int    `json:"id"`
	Cover     string `json:"cover"`
	Title     string `json:"title"`
	BookURL   string `json:"bookURL"`
	Author    string `json:"author"`
	AuthorURL string `json:"authorURL"`
	Rating    string `json:"rating"`
}

type BookResponse struct {
	Status          string          `json:"status"`
	StatusCode      int             `json:"statusCode"`
	Source          string          `json:"source"`
	ScrapeURL       string          `json:"scrapeURL"`
	LegacyBookID    int             `json:"legacyBookID"`
	BookID          string          `json:"bookID"`
	ResourceID      string          `json:"resourceID"`
	Cover           string          `json:"cover"`
	Series          string          `json:"series"`
	SeriesURL       string          `json:"seriesURL"`
	SeriesPosition  string          `json:"seriesPosition"`
	WorkURL         string          `json:"workURL"`
	Title           string          `json:"title"`
	Author          []BookAuthor    `json:"author"`
	Rating          string          `json:"rating"`
	RatingCount     string          `json:"ratingCount"`
	ReviewsCount    string          `json:"reviewsCount"`
	Desc            string          `json:"desc"`
	Genres          []string        `json:"genres"`
	BookEdition     string          `json:"bookEdition"`
	PublishDate     string          `json:"publishDate"`
	Publisher       string          `json:"publisher"`
	Language        string          `json:"language"`
	ASIN            string          `json:"asin"`
	ISBN            string          `json:"isbn"`
	ISBN13          string          `json:"isbn13"`
	ReviewBreakdown ReviewBreakdown `json:"reviewBreakdown"`
	Quotes          string          `json:"quotes"`
	QuotesURL       string          `json:"quotesURL"`
	Questions       string          `json:"questions"`
	QuestionsURL    string          `json:"questionsURL"`
	LastScraped     string          `json:"lastScraped"`
}

type BookAuthor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
	URL  string `json:"url"`
}

type ReviewBreakdown struct {
	Rating1 string `json:"rating1"`
	Rating2 string `json:"rating2"`
	Rating3 string `json:"rating3"`
	Rating4 string `json:"rating4"`
	Rating5 string `json:"rating5"`
}

type ReviewsResponse struct {
	Status        string   `json:"status"`
	StatusCode    int      `json:"statusCode"`
	Source        string   `json:"source"`
	ScrapeURL     string   `json:"scrapeURL"`
	TotalCount    int      `json:"totalCount"`
	NextPageToken string   `json:"nextPageToken"`
	PrevPageToken string   `json:"prevPageToken"`
	Reviews       []Review `json:"reviews"`
	LastScraped   string   `json:"lastScraped"`
}

type Review struct {
	ID           string   `json:"id"`
	Reviewer     Reviewer `json:"reviewer"`
	Rating       int      `json:"rating"`
	Text         string   `json:"text"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
	Spoiler      bool     `json:"spoiler"`
	LikeCount    int      `json:"likeCount"`
	CommentCount int      `json:"commentCount"`
	RecommendFor *string  `json:"recommendFor"`
	Shelf        string   `json:"shelf"`
	ReviewURL    string   `json:"reviewURL"`
}

type Reviewer struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	Avatar        string `json:"avatar"`
	IsAuthor      bool   `json:"isAuthor"`
	ReviewCount   int    `json:"reviewCount"`
	FollowerCount int    `json:"followerCount"`
}
