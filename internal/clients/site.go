package clients

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pwnholic/comdown/internal"
)

type ComicMetadata struct {
	MaxChapter int
	MinChapter int
	URL        string
	Single     int
	ScraperConfig
}

type ScraperConfig struct {
	Hostname       string `json:"hostname"`
	ListChapterURL string `json:"list_chapter_url"`
	AttrChapter    string `json:"attr_chapter"`
	ListImageURL   string `json:"list_image_url"`
	AttrImage      string `json:"attr_image"`
	Pattern        string `json:"pattern"`
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

func (w *websiteConfig) GetChapterNumber(urlRaw string) (string, error) {
	re := regexp.MustCompile(`chapter-(\d+)(?:-(\d+))?`)
	match := re.FindStringSubmatch(urlRaw)
	if match == nil {
		return "", fmt.Errorf("number not found")
	}

	mainNum, _ := strconv.Atoi(match[1])
	mainFormatted := fmt.Sprintf("%02d", mainNum)

	if match[2] != "" {
		return fmt.Sprintf("%s.%s", mainFormatted, match[2]), nil
	}

	return mainFormatted, nil
}
