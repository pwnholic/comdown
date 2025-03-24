package main

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
)

type htmlTagAttr struct {
	Hostname       string `json:"hostname"`
	ListChapterURL string `json:"list_chapter_url"`
	AttrChapter    string `json:"attr_chapter"`
	ListImageURL   string `json:"list_image_url"`
	AttrImage      string `json:"attr_image"`
}

func supportedWebsite(rawURL string) *htmlTagAttr {
	website, err := os.ReadFile("website.json")
	if err != nil {
		logger.Printf("[ERROR] Failed to read configuration file: %v\n", err)
		return nil
	}

	var sites []htmlTagAttr
	if err := json.Unmarshal(website, &sites); err != nil {
		logger.Printf("[ERROR] Failed to parse JSON configuration: %v\n", err)
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		logger.Printf("[ERROR] Failed to parse URL: %v\n", err)
		return nil
	}

	host := parsedURL.Hostname()
	for _, site := range sites {
		if strings.EqualFold(host, site.Hostname) {
			logger.Printf("[INFO] Found configuration for domain: %s\n", host)
			return &site
		}
	}

	logger.Printf("[WARNING] Configuration not found for domain: '%s'\n", host)
	return nil
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Safari/605.1.15",
	"Mozilla/5.0 (Linux; Android 10; SM-G980F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36",
}
