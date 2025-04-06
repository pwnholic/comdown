package clients

import (
	"bytes"
	"errors"
	"fmt"
	"image/jpeg"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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
			if err != nil || r.IsError() {
				internal.WarningLog("Retrying request due status code [%d] to error: %s (attempt %d)\n", r.StatusCode(), err.Error(), r.Request.Attempt)
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

	cond := len(metadata.ListChapterURL) > 0 && len(metadata.AttrChapter) > 0 && len(metadata.URL) > 0
	if !cond {
		internal.ErrorLog("cannot collect link becuse condition not fullfield")
		return nil, errors.New("condition not fullfield")
	}

	response, err := c.Client.R().Get(metadata.URL)
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
			result, err := completeURL(href, metadata.URL)
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
		links = links[metadata.MinChapter : metadata.MaxChapter+1]
	} else if isSingle && !isRange {
		internal.InfoLog("Selecting single chapter %d\n", metadata.Single+1)
		links = links[metadata.Single : metadata.Single+1]
	}

	return links, nil
}

// Just need URL and attr
func (c *clientRequest) CollectImgTagsLink(metadata *ComicMetadata) ([]string, error) {
	cond := len(metadata.ListChapterURL) > 0 && (len(metadata.AttrImage) > 0 || len(metadata.Pattern) > 0) && len(metadata.URL) > 0
	if !cond {
		internal.ErrorLog("cannot collect image link because condition not fulfilled")
		return nil, errors.New("condition not fulfilled")
	}

	response, err := c.Client.R().Get(metadata.URL)
	if err != nil {
		internal.ErrorLog("Failed to fetch URL: %v\n", err.Error())
		return nil, err
	}
	defer response.RawResponse.Body.Close()

	if response.StatusCode() != http.StatusOK {
		internal.WarningLog("Skipping URL %s because status code is %d\n", metadata.URL, response.StatusCode())
		return nil, nil
	}

	isIPBlocked, reason := statusCode(response)
	if isIPBlocked {
		internal.WarningLog("BLOCKED : %s\n", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.RawResponse.Body, contentType)
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

	isURLValid := func(u string) bool {
		_, err := url.ParseRequestURI(u)
		return err == nil
	}

	document.Find(metadata.ListImageURL).Each(func(i int, s *goquery.Selection) {
		switch len(metadata.AttrImage) > 0 && len(metadata.Pattern) < 1 {
		case true:
			href, exists := s.Attr(metadata.AttrImage)
			if exists {
				links = append(links, href)
			}
		case false:
			re := regexp.MustCompile(metadata.Pattern)
			matches := re.FindStringSubmatch(s.Text())
			if len(matches) > 1 {
				images := strings.SplitSeq(matches[1], ",")
				for imgLink := range images {
					img := strings.Trim(imgLink, "\" ")
					img = strings.ReplaceAll(img, "\\/", "/")
					if isURLValid(img) {
						links = append(links, img)
					}
				}
			}
		default:
			internal.ErrorLog("could not get anything from given metadata")
		}
	})
	internal.InfoLog("Found %d images on page %s\n", len(links), metadata.URL)
	return links, nil
}

func (c *clientRequest) CollectImage(imgLink, ext string, enhance bool) ([]byte, error) {
	resp, err := c.Client.R().Get(imgLink)
	if err != nil {
		internal.ErrorLog("Failed to fetch image after %d attempts: %s\n", resp.Request.Attempt, err.Error())
		return nil, fmt.Errorf("failed after %d attempts: %w", resp.Request.Attempt, err)
	}
	defer resp.Body.Close()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(resp.Body)
	if err != nil {
		internal.ErrorLog("Failed to read image data: %s\n", err.Error())
		return nil, err
	}

	contentType := resp.Header().Get("Content-Type")
	enhanceImage := func(imgBytes []byte) ([]byte, error) {
		img, err := imaging.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			internal.ErrorLog("Failed to decode image for enhancement: %s", err.Error())
			return nil, err
		}

		img = imaging.Resize(img, img.Bounds().Dx()*2, img.Bounds().Dy()*2, imaging.Lanczos)
		img = imaging.Sharpen(img, 0.7)
		img = imaging.AdjustContrast(img, 10)

		outBuff := new(bytes.Buffer)
		err = jpeg.Encode(outBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			internal.ErrorLog("Failed to encode enhanced image: %s", err.Error())
			return nil, err
		}

		return outBuff.Bytes(), nil
	}

	if contentType == "image/webp" || ext == "webp" {
		internal.InfoLog("Processing WEBP image conversion")
		img, err := webp.Decode(buff)
		if err != nil {
			internal.ErrorLog("Failed to decode webp image: %s\n", err.Error())
			return nil, err
		}

		outputBuff := new(bytes.Buffer)
		err = jpeg.Encode(outputBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			internal.ErrorLog("Failed to encode image: %s\n", err.Error())
			return nil, err
		}

		if enhance {
			enhanced, err := enhanceImage(outputBuff.Bytes())
			if err != nil {
				internal.WarningLog("Failed to enhance image: %s\n", err.Error())
				return outputBuff.Bytes(), nil
			}
			internal.InfoLog("WEBP to JPEG conversion with enhancement completed")
			return enhanced, nil
		}

		internal.InfoLog("WEBP to JPEG conversion completed")
		return outputBuff.Bytes(), nil
	}

	if enhance {
		enhanced, err := enhanceImage(buff.Bytes())
		if err != nil {
			internal.WarningLog("Failed to enhance image: %s\n", err.Error())
			return buff.Bytes(), nil
		}
		internal.InfoLog("Image enhancement completed")
		return enhanced, nil
	}
	return buff.Bytes(), nil
}
