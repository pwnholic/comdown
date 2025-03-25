package clients

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
	logger "github.com/pwnholic/comdown/internal"
	"golang.org/x/net/html/charset"
	"resty.dev/v3"
)

type clientRequest struct {
	Client *resty.Client
}

type HTTPClientOptions struct {
	RetryCount       int
	RetryWaitTime    time.Duration
	RetryMaxWaitTime time.Duration
	TimeOut          time.Duration
	UserAgent        string
}

func NewClientRequest(t *HTTPClientOptions) *clientRequest {
	client := resty.New().
		SetRetryCount(t.RetryCount).
		SetRetryWaitTime(t.RetryWaitTime*time.Second).
		SetRetryMaxWaitTime(t.RetryMaxWaitTime*time.Second).
		SetTimeout(t.TimeOut*time.Second).
		SetHeader("User-Agent", t.UserAgent)

	defer client.Close()

	return &clientRequest{
		Client: client,
	}
}

func statusCode(resp *resty.Response) (bool, string) {
	switch resp.StatusCode() {
	case http.StatusTooManyRequests:
		return true, "IP blocked: Too Many Requests (429)"
	case http.StatusForbidden:
		return true, "IP blocked: Forbidden (403)"
	case http.StatusServiceUnavailable:
		return true, "IP blocked: Service Unavailable (503)"
	case http.StatusOK:
		return false, "Status OK"
	}
	return false, "IP not blocked"
}

func completeURL(inputURL, defaultHost string) (string, error) {
	if inputURL == "" {
		return "", fmt.Errorf("URL cannot be empty")
	}
	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	if parsedURL.IsAbs() {
		return inputURL, nil
	}
	if defaultHost == "" {
		return "", fmt.Errorf("host need for relative url: %w", err)
	}
	defaultURL, err := url.Parse(defaultHost)
	if err != nil {
		return "", fmt.Errorf("invalid default host: %v", err)
	}
	if defaultURL.Scheme == "" {
		defaultURL.Scheme = "https"
	}
	resultURL := defaultURL.ResolveReference(parsedURL)
	return resultURL.String(), nil
}

func (c *clientRequest) CollectLinks(metadata *ComicMetadata) ([]string, error) {
	isRange := metadata.MinChapter > 0 && metadata.MaxChapter >= metadata.MinChapter
	isSingle := metadata.IsSingle != 0

	response, err := c.Client.R().Get(metadata.RawURL)
	if err != nil {
		logger.Error("Failed to fetch URL: %sn", err.Error())
		return nil, err
	}
	defer response.Body.Close()

	isIPBlocked, reason := statusCode(response)
	if isIPBlocked {
		logger.Warn("BLOCKERD: %s", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		logger.Error("Failed to create charset reader: %s\n", err.Error())
		return nil, err
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		logger.Error("Failed to parse HTML document: %s\n", err.Error())
		return nil, err
	}

	var links []string
	document.Find(metadata.ListChapterURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(metadata.AttrChapter)
		if exists {
			result, err := completeURL(href, metadata.RawURL)
			if err != nil {
				logger.Error("Failed to add hostname :%s", err.Error())
				return
			}
			links = append(links, result)
		}
	})

	logger.Debug("Found %d chapter links\n", len(links))

	// Reverse links
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	if isRange && !isSingle {
		logger.Info("Filtering chapters range %d-%d\n", metadata.MinChapter, metadata.MaxChapter)
		links = links[metadata.MinChapter-1 : metadata.MaxChapter]
	} else if isSingle && !isRange {
		logger.Info("Selecting single chapter %d\n", metadata.IsSingle)
		links = links[metadata.IsSingle-1 : metadata.IsSingle]
	}
	return links, nil
}

func (c *clientRequest) CollectImgTagsLink(metadata *ComicMetadata) ([]string, error) {
	response, err := c.Client.R().Get(metadata.RawURL)
	if err != nil {
		logger.Error("Failed to fetch URL: %v\n", err.Error())
		return nil, err
	}
	defer response.Body.Close()

	isIPBlocked, reason := statusCode(response)
	if isIPBlocked {
		logger.Warn("BLOCKED : %s\n", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		logger.Error("Failed to create charset reader: %s\n", err.Error())
		return nil, err
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		logger.Error("Failed to parse HTML document: %v\n", err.Error())
		return nil, err
	}

	var links []string
	document.Find(metadata.ListImageURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(metadata.AttrImage)
		if exists {
			links = append(links, href)
		}
	})
	logger.Info("Found %d images on page %s\n", len(links), metadata.RawURL)
	return links, nil

}
