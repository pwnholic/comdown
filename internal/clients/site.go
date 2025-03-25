package clients

type ComicMetadata struct {
	MaxChapter int
	MinChapter int
	RawURL     string
	IsSingle   int
	ScraperConfig
}

type ScraperConfig struct {
	Hostname       string `json:"hostname"`
	ListChapterURL string `json:"list_chapter_url"`
	AttrChapter    string `json:"attr_chapter"`
	ListImageURL   string `json:"list_image_url"`
	AttrImage      string `json:"attr_image"`
}
