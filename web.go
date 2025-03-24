package main

import (
	"math/rand"
	"strings"
)

type htmlTagAttr struct {
	listChapterURL string
	attrChapter    string
	listImageURL   string
	attrImage      string
}

func supportedWebsite(rawURL string) *htmlTagAttr {
	sites := map[string]htmlTagAttr{
		"komikindo2.com": {
			listChapterURL: "ul li span.lchx a",
			attrChapter:    "href",
			listImageURL:   "div#chimg-auh img",
			attrImage:      "src",
		},
	}

	for domain, attr := range sites {
		if strings.Contains(rawURL, domain) {
			return &attr
		}
	}
	return nil
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Safari/605.1.15",
	"Mozilla/5.0 (Linux; Android 10; SM-G980F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36",
}

func randomUserAgent() string {
	return userAgents[rand.Intn(len(userAgents))]
}
