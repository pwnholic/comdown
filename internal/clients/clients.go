package clients

type RequestBuilder struct {
	Request interface {
		CollectLinks(metadata *ComicMetadata) ([]string, error)
		CollectImgTagsLink(metadata *ComicMetadata) ([]string, error)
		CollectImage(imgLink, ext string, enhance bool) ([]byte, string, error)
	}
	Website interface {
		GetHTMLTagAttrFromURL(rawURL string) *ScraperConfig
		GetImageExtension(url string) *string
		GetChapterNumber(urlRaw string) string
	}
}

func NewRequestBuilder(t *HTTPClientOptions) *RequestBuilder {
	return &RequestBuilder{
		Request: NewClientRequest(t),
		Website: NewWebsiteConfig(),
	}
}
