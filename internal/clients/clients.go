package clients

type requestBuilder struct {
	Client interface {
		CollectLinks(metadata *ComicMetadata) ([]string, error)
		CollectImgTagsLink(metadata *ComicMetadata) ([]string, error)
	}
}
