package clients

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/disintegration/imaging"
	"github.com/pwnholic/comdown/internal"
	"golang.org/x/image/webp"
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
	Timeout          time.Duration
	UserAgent        string
}

func NewClientRequest(t *HTTPClientOptions) *clientRequest {
	if t == nil {
		t = &HTTPClientOptions{
			RetryCount:       3,
			RetryWaitTime:    2,
			RetryMaxWaitTime: 10,
			Timeout:          30,
			UserAgent:        "Mozilla/5.0 (compatible; Resty Client)",
		}
	}

	if t.RetryCount < 0 {
		t.RetryCount = 0
	}
	if t.RetryWaitTime <= 0 {
		t.RetryWaitTime = 2
	}
	if t.RetryMaxWaitTime <= 0 {
		t.RetryMaxWaitTime = 10
	}
	if t.Timeout <= 0 {
		t.Timeout = 30
	}
	if t.UserAgent == "" {
		t.UserAgent = "Mozilla/5.0 (compatible; Resty Client)"
	}

	client := resty.New().
		SetRetryCount(t.RetryCount).
		SetRetryWaitTime(time.Duration(t.RetryWaitTime)*time.Second).
		SetRetryMaxWaitTime(time.Duration(t.RetryMaxWaitTime)*time.Second).
		AddRetryConditions(func(r *resty.Response, err error) bool {
			return err != nil || (r != nil && r.StatusCode() >= http.StatusInternalServerError)
		}).
		AddRetryHooks(func(r *resty.Response, err error) {
			if err != nil {
				internal.WarningLog("Retrying request due to error: %v (attempt %d)\n", err, r.Request.Attempt)
			} else if r != nil {
				internal.WarningLog("Retrying request due to status code: %d (attempt %d)\n", r.StatusCode(), r.Request.Attempt)
			}
		}).
		SetHeader("User-Agent", t.UserAgent).
		SetTimeout(time.Duration(t.Timeout) * time.Second)

	return &clientRequest{
		Client: client,
	}
}

var UserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Safari/605.1.15",
	"Mozilla/5.0 (Linux; Android 10; SM-G980F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36",
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
	isSingle := metadata.Single != 0

	response, err := c.Client.R().Get(metadata.RawURL)
	if err != nil {
		internal.ErrorLog("Failed to fetch URL: %sn", err.Error())
		return nil, err
	}
	defer response.Body.Close()

	isIPBlocked, reason := statusCode(response)
	if isIPBlocked {
		internal.WarningLog("BLOCKED: %s", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		internal.ErrorLog("Failed to create charset reader: %s\n", err.Error())
		return nil, err
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		internal.ErrorLog("Failed to parse HTML document: %s\n", err.Error())
		return nil, err
	}

	var links []string
	document.Find(metadata.ListChapterURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(metadata.AttrChapter)
		if exists {
			result, err := completeURL(href, metadata.RawURL)
			if err != nil {
				internal.ErrorLog("Failed to add hostname :%s", err.Error())
				return
			}
			links = append(links, result)
		}
	})

	internal.DebugLog("Found %d chapter links\n", len(links))

	// Reverse links
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	if isRange && !isSingle {
		internal.InfoLog("Filtering chapters range %d-%d\n", metadata.MinChapter, metadata.MaxChapter)
		links = links[metadata.MinChapter-1 : metadata.MaxChapter]
	} else if isSingle && !isRange {
		internal.InfoLog("Selecting single chapter %d\n", metadata.Single)
		links = links[metadata.Single-1 : metadata.Single]
	}
	return links, nil
}

// Just need URL and attr
func (c *clientRequest) CollectImgTagsLink(metadata *ComicMetadata) ([]string, error) {
	response, err := c.Client.R().Get(metadata.RawURL)
	if err != nil {
		internal.ErrorLog("Failed to fetch URL: %v\n", err.Error())
		return nil, err
	}
	defer response.Body.Close()

	isIPBlocked, reason := statusCode(response)
	if isIPBlocked {
		internal.WarningLog("BLOCKED : %s\n", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		internal.ErrorLog("Failed to create charset reader: %s\n", err.Error())
		return nil, err
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		internal.ErrorLog("Failed to parse HTML document: %v\n", err.Error())
		return nil, err
	}

	var links []string
	document.Find(metadata.ListImageURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(metadata.AttrImage)
		if exists {
			links = append(links, href)
		}
	})

	internal.InfoLog("Found %d images on page %s\n", len(links), metadata.RawURL)
	return links, nil
}

func (c *clientRequest) CollectImage(imgLink, ext string, enhance bool) ([]byte, string, error) {
	resp, err := c.Client.R().Get(imgLink)

	if err != nil {
		internal.ErrorLog("Failed to fetch image after %d attempts: %s\n", resp.Request.Attempt, err.Error())
		return nil, imgLink, fmt.Errorf("failed after %d attempts: %w", resp.Request.Attempt, err)
	}
	defer resp.Body.Close()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(resp.Body)
	if err != nil {
		internal.ErrorLog("Failed to read image data: %s\n", err.Error())
		return nil, imgLink, err
	}

	contentType := resp.Header().Get("Content-Type")
	enhanceImage := func(imgBytes []byte) ([]byte, string, error) {
		img, err := imaging.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			internal.ErrorLog("Failed to decode image for enhancement: %s", err.Error())
			return nil, imgLink, nil
		}

		img = imaging.Resize(img, img.Bounds().Dx()*2, img.Bounds().Dy()*2, imaging.Lanczos)
		img = imaging.Sharpen(img, 0.7)
		img = imaging.AdjustContrast(img, 10)

		outBuff := new(bytes.Buffer)
		err = jpeg.Encode(outBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			internal.ErrorLog("Failed to encode enhanced image: %s", err.Error())
			return nil, imgLink, nil
		}

		return outBuff.Bytes(), imgLink, nil
	}

	if contentType == "image/webp" || ext == "webp" {
		internal.InfoLog("Processing WEBP image conversion")
		img, err := webp.Decode(buff)
		if err != nil {
			internal.ErrorLog("Failed to decode webp image: %s\n", err.Error())
			return nil, imgLink, nil
		}

		outputBuff := new(bytes.Buffer)
		err = jpeg.Encode(outputBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			internal.ErrorLog("Failed to encode image: %s\n", err.Error())
			return nil, imgLink, nil
		}

		if enhance {
			enhanced, imgLink, err := enhanceImage(outputBuff.Bytes())
			if err != nil {
				internal.WarningLog("Failed to enhance image: %s\n", err.Error())
				return outputBuff.Bytes(), imgLink, nil
			}
			internal.InfoLog("WEBP to JPEG conversion with enhancement completed")
			return enhanced, imgLink, nil
		}

		internal.InfoLog("WEBP to JPEG conversion completed")
		return outputBuff.Bytes(), imgLink, nil
	}

	if enhance {
		enhanced, imgLink, err := enhanceImage(buff.Bytes())
		if err != nil {
			internal.WarningLog("Failed to enhance image: %s\n", err.Error())
			return buff.Bytes(), imgLink, nil
		}
		internal.InfoLog("Image enhancement completed")
		return enhanced, imgLink, nil
	}
	return buff.Bytes(), imgLink, nil
}
