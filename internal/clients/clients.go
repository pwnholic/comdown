package clients

type RequestBuilder struct {
	Request interface {
		CollectLinks(metadata *ComicMetadata) ([]string, error)
		CollectImgTagsLink(metadata *ComicMetadata) ([]string, error)
		CollectImage(imgLink string, enhance bool) ([]byte, error)
	}
	Website interface {
		GetHTMLTagAttrFromURL(rawURL string) *ScraperConfig
		GetChapterNumber(urlRaw string) (string, error)
	}
}

func NewRequestBuilder(t *HTTPClientOptions) *RequestBuilder {
	return &RequestBuilder{
		Request: NewClientRequest(t),
		Website: NewWebsiteConfig(),
	}
}
