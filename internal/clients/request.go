package clients

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/disintegration/imaging"
	"github.com/pwnholic/comdown/internal"
	"golang.org/x/image/webp"
	"golang.org/x/net/html/charset"
	"resty.dev/v3"
)

const (
	defaultRetryCount       = 3
	defaultRetryWaitTime    = 2 * time.Second
	defaultRetryMaxWaitTime = 10 * time.Second
	defaultTimeout          = 30 * time.Second
	defaultUserAgent        = "Mozilla/5.0 (compatible; Resty Client)"
	defaultJPEGQuality      = 100
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

func NewClientRequest(opts *HTTPClientOptions) *clientRequest {
	if opts == nil {
		opts = &HTTPClientOptions{
			RetryCount:       defaultRetryCount,
			RetryWaitTime:    defaultRetryWaitTime,
			RetryMaxWaitTime: defaultRetryMaxWaitTime,
			Timeout:          defaultTimeout,
			UserAgent:        defaultUserAgent,
		}
	}

	opts = normalizeOptions(opts)

	client := resty.New().
		SetRetryCount(opts.RetryCount).
		SetRetryWaitTime(opts.RetryWaitTime).
		SetRetryMaxWaitTime(opts.RetryMaxWaitTime).
		AddRetryConditions(retryCondition).
		AddRetryHooks(retryHook).
		SetHeader("User-Agent", opts.UserAgent).
		SetTimeout(opts.Timeout)

	return &clientRequest{Client: client}
}

func normalizeOptions(opts *HTTPClientOptions) *HTTPClientOptions {
	if opts.RetryCount < 0 {
		opts.RetryCount = 0
	}
	if opts.RetryWaitTime <= 0 {
		opts.RetryWaitTime = defaultRetryWaitTime
	}
	if opts.RetryMaxWaitTime <= 0 {
		opts.RetryMaxWaitTime = defaultRetryMaxWaitTime
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.UserAgent == "" {
		opts.UserAgent = defaultUserAgent
	}
	return opts
}

func retryCondition(r *resty.Response, err error) bool {
	return err != nil || (r != nil && r.StatusCode() >= http.StatusInternalServerError)
}

func retryHook(r *resty.Response, err error) {
	if err != nil || r.IsError() {
		internal.WarningLog("Retrying request due status code [%d] to error: %s (attempt %d)\n",
			r.StatusCode(), err.Error(), r.Request.Attempt)
	}
}

func (c *clientRequest) CollectLinks(metadata *ComicMetadata) ([]string, error) {
	if err := validateMetadataForLinks(metadata); err != nil {
		return nil, err
	}

	response, err := c.Client.R().Get(metadata.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()

	checkBlockStatus(response)

	document, err := parseHTMLResponse(response)
	if err != nil {
		return nil, err
	}

	links := extractLinks(document, metadata)
	links = reverseLinks(links)

	return filterLinks(links, metadata), nil
}

func validateMetadataForLinks(metadata *ComicMetadata) error {
	if len(metadata.ListChapterURL) == 0 || len(metadata.AttrChapter) == 0 || len(metadata.URL) == 0 {
		return errors.New("metadata conditions not fulfilled for collecting links")
	}
	return nil
}

func parseHTMLResponse(response *resty.Response) (*goquery.Document, error) {
	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to create charset reader: %w", err)
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML document: %w", err)
	}
	return document, nil
}

func extractLinks(document *goquery.Document, metadata *ComicMetadata) []string {
	var links []string
	document.Find(metadata.ListChapterURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(metadata.AttrChapter)
		if exists {
			if result, err := completeURL(href, metadata.URL); err == nil {
				links = append(links, result)
			} else {
				internal.ErrorLog("Failed to complete URL: %v\n", err)
			}
		}
	})
	return links
}

func reverseLinks(links []string) []string {
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}
	return links
}

func filterLinks(links []string, metadata *ComicMetadata) []string {
	isRange := metadata.MinChapter > 0 && metadata.MaxChapter >= metadata.MinChapter
	isSingle := metadata.Single != 0

	switch {
	case isRange && !isSingle:
		internal.InfoLog("Filtering chapters range %d-%d\n", metadata.MinChapter, metadata.MaxChapter)
		return links[metadata.MinChapter:metadata.MaxChapter]
	case isSingle && !isRange:
		internal.InfoLog("Selecting single chapter %d\n", metadata.Single-1)
		return links[metadata.Single-1 : metadata.Single]
	default:
		return links
	}
}

func (c *clientRequest) CollectImgTagsLink(metadata *ComicMetadata) ([]string, error) {
	if err := validateMetadataForImages(metadata); err != nil {
		return nil, err
	}
	response, err := c.Client.R().Get(metadata.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode() != http.StatusOK {
		internal.WarningLog("Skipping URL %s with status code %d\n", metadata.URL, response.StatusCode())
		return nil, nil
	}

	checkBlockStatus(response)

	document, err := parseHTMLResponse(response)
	if err != nil {
		return nil, err
	}

	return extractImageLinks(document, metadata), nil
}

func validateMetadataForImages(metadata *ComicMetadata) error {
	if len(metadata.ListImageURL) == 0 || (len(metadata.AttrImage) == 0 && len(metadata.Pattern) == 0) || len(metadata.URL) == 0 {
		return errors.New("metadata conditions not fulfilled for collecting images")
	}
	return nil
}

func extractImageLinks(document *goquery.Document, metadata *ComicMetadata) []string {
	var links []string
	document.Find(metadata.ListImageURL).Each(func(i int, s *goquery.Selection) {
		if len(metadata.AttrImage) > 0 && len(metadata.Pattern) == 0 {
			if href, exists := s.Attr(metadata.AttrImage); exists {
				links = append(links, href)
			}
		} else if len(metadata.Pattern) > 0 {
			processPatternMatch(s, metadata.Pattern, &links)
		}
	})

	internal.InfoLog("Found %d images on page %s\n", len(links), metadata.URL)
	return links
}

func processPatternMatch(s *goquery.Selection, pattern string, links *[]string) {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(s.Text())
	if len(matches) > 1 {
		images := strings.SplitSeq(matches[1], ",")
		for imgLink := range images {
			img := strings.Trim(imgLink, "\" ")
			img = strings.ReplaceAll(img, "\\/", "/")
			if isURLValid(img) {
				*links = append(*links, img)
			}
		}
	}
}

func isURLValid(u string) bool {
	_, err := url.ParseRequestURI(u)
	return err == nil
}

func (c *clientRequest) CollectImage(imgLink string, enhance bool) ([]byte, error) {
	resp, err := c.Client.R().Get(imgLink)
	if err != nil {
		return nil, fmt.Errorf("failed after %d attempts: %w", resp.Request.Attempt, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode() != http.StatusOK {
		internal.WarningLog("Skipping URL %s with status code %d\n", imgLink, resp.StatusCode())
		return nil, nil
	}

	imgBytes, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	return processImage(imgBytes, enhance)
}

func readResponseBody(resp *resty.Response) ([]byte, error) {
	buff := new(bytes.Buffer)
	if _, err := buff.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}
	return buff.Bytes(), nil
}

func processImage(imgBytes []byte, enhance bool) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		internal.WarningLog("Failed to decode image: %v\n", err)
		return nil, nil
	}

	lowCaseFormat := strings.ToLower(format)
	if slices.Contains([]string{"gif"}, format) {
		internal.WarningLog("Skipping gif\n")
	}

	switch lowCaseFormat {
	case "webp":
		return processWebPImage(imgBytes, enhance)
	case "png":
		return processPNGImage(imgBytes, enhance)
	case "jpg", "jpeg":
		return imgBytes, nil
	default:
		return encodeToJPEG(img, enhance)
	}
}

func processWebPImage(imgBytes []byte, enhance bool) ([]byte, error) {
	img, err := webp.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode webp image: %w", err)
	}

	jpegBytes, err := encodeToJPEG(img, enhance)
	if err != nil {
		return nil, err
	}
	return jpegBytes, nil
}

func processPNGImage(imgBytes []byte, enhance bool) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode webp image: %w", err)
	}

	jpegBytes, err := encodeToJPEG(img, enhance)
	if err != nil {
		return nil, err
	}
	return jpegBytes, nil
}

func encodeToJPEG(img image.Image, enhance bool) ([]byte, error) {
	buff := new(bytes.Buffer)
	if err := jpeg.Encode(buff, img, &jpeg.Options{Quality: defaultJPEGQuality}); err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}
	jpgBytes := buff.Bytes()
	if enhance {
		return enhanceImage(jpgBytes, enhance)
	}
	return jpgBytes, nil
}

func enhanceImage(imgBytes []byte, enhance bool) ([]byte, error) {
	img, err := imaging.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image for enhancement: %w", err)
	}

	img = imaging.Resize(img, img.Bounds().Dx()*2, img.Bounds().Dy()*2, imaging.Lanczos)
	img = imaging.Sharpen(img, 0.7)
	img = imaging.AdjustContrast(img, 10)

	return encodeToJPEG(img, enhance)
}

func checkBlockStatus(response *resty.Response) {
	if isBlocked, reason := isIPBlocked(response); isBlocked {
		internal.WarningLog("BLOCKED: %s\n", reason)
	}
}

func isIPBlocked(response *resty.Response) (bool, string) {
	switch response.StatusCode() {
	case http.StatusTooManyRequests:
		return true, "IP blocked: Too Many Requests (429)"
	case http.StatusForbidden:
		return true, "IP blocked: Forbidden (403)"
	case http.StatusServiceUnavailable:
		return true, "IP blocked: Service Unavailable (503)"
	default:
		return false, "IP not blocked"
	}
}

func completeURL(inputURL, defaultHost string) (string, error) {
	if inputURL == "" {
		return "", errors.New("URL cannot be empty")
	}

	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if parsedURL.IsAbs() {
		return inputURL, nil
	}

	if defaultHost == "" {
		return "", errors.New("host needed for relative URL")
	}

	defaultURL, err := url.Parse(defaultHost)
	if err != nil {
		return "", fmt.Errorf("invalid default host: %w", err)
	}
	if defaultURL.Scheme == "" {
		defaultURL.Scheme = "https"
	}
	return defaultURL.ResolveReference(parsedURL).String(), nil
}
