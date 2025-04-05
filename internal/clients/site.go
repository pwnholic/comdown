package clients

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/pwnholic/comdown/internal"
)

type ComicMetadata struct {
	MaxChapter int
	MinChapter int
	RawURL     string
	Single     int
	ScraperConfig
}

type ScraperConfig struct {
	Hostname       string `json:"hostname"`
	ListChapterURL string `json:"list_chapter_url"`
	AttrChapter    string `json:"attr_chapter"`
	ListImageURL   string `json:"list_image_url"`
	AttrImage      string `json:"attr_image"`
}

type websiteConfig struct {
	configPath string
}

func NewWebsiteConfig() *websiteConfig {
	cwd, err := os.Getwd()
	if err != nil {
		internal.ErrorLog("Could not get current working dir : %s", err.Error())
		return nil
	}
	fullPath := fmt.Sprintf("%s/config.json", cwd)
	return &websiteConfig{
		configPath: fullPath,
	}
}

func (c *websiteConfig) GetHTMLTagAttrFromURL(rawURL string) *ScraperConfig {
	configFile, err := os.ReadFile(c.configPath)
	if err != nil {
		internal.ErrorLog("Failed to read configuration file: %s\n", err.Error())
		return nil
	}

	var config []ScraperConfig
	if err := json.Unmarshal(configFile, &config); err != nil {
		internal.ErrorLog("Failed to parse JSON configuration: %s\n", err.Error())
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		internal.ErrorLog("Failed to parse URL: %s\n", err.Error())
		return nil
	}

	host := parsedURL.Hostname()
	for _, site := range config {
		if strings.EqualFold(host, site.Hostname) {
			internal.InfoLog("Found configuration for domain: %s\n", host)
			return &site
		}
	}
	internal.WarningLog("Configuration not found for domain: '%s'\n", host)
	return nil
}

func (c *websiteConfig) GetImageExtension(url string) *string {
	fileName := path.Base(url)
	if fileName == "" {
		internal.ErrorLog("invalid URL: no file name found")
		return nil
	}

	ext := strings.ToLower(path.Ext(fileName))
	if ext == "" {
		internal.ErrorLog("no file extension found in URL")
		return nil
	}

	ext = strings.TrimPrefix(ext, ".")

	supportedExtensions := map[string]bool{
		"jpg":  true,
		"jpeg": true,
		"png":  true,
		"webp": true,
		"gif":  true,
	}
	if !supportedExtensions[ext] {
		internal.ErrorLog("unsupported image extension: %s", ext)
		return nil
	}
	return &ext
}

func (w *websiteConfig) GetChapterNumber(urlRaw string) string {
	re := regexp.MustCompile(`chapter-(\d+)(?:-(\d+))?/`)
	matches := re.FindStringSubmatch(urlRaw)

	if len(matches) >= 2 {
		if len(matches) >= 3 && matches[2] != "" {
			return matches[1] + "." + matches[2]
		}
		return matches[1]
	}
	return urlRaw
}
