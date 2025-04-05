package clients

type RequestBuilder struct {
	Request Request
	Website Website
}

func NewRequestBuilder(t *HTTPClientOptions) *RequestBuilder {
	return &RequestBuilder{
		Request: NewClientRequest(t),
		Website: NewWebsiteConfig(),
	}
}
