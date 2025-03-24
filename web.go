package main

import (
	"net/url"
	"strings"
)

type htmlTagAttr struct {
	listChapterURL string
	attrChapter    string
	listImageURL   string
	attrImage      string
}

// for exmaple the list of chapter something like this :
// <ul>
// 	<li>
// 		<span class="lchx">
// 			<a href="https://komikindo2.com/eleceed-chapter-342/">Chapter
// 				<chapter>342</chapter>
// 			</a>
// 		</span>
// 	</li>
// ...
// ...
// </ul>

var sites = map[string]htmlTagAttr{
	"komikindo2.com": {
		listChapterURL: "ul li span.lchx a",
		attrChapter:    "href",
		listImageURL:   "div#chimg-auh img",
		attrImage:      "src",
	},
	"komiku.id": {
		listChapterURL: "tbody tr td.judulseries a",
		attrChapter:    "href",
		listImageURL:   "div#Baca_Komik img",
		attrImage:      "src",
	},

	// Ada anti scrapping pada web ini
	// TODO: bypass that shitt
	"apkomik.cc": {
		listChapterURL: "ul li div.chbox div.eph-num a",
		attrChapter:    "href",
		listImageURL:   "div#readerarea img",
		attrImage:      "src",
	},
}

func supportedWebsite(rawURL string) *htmlTagAttr {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		logger.Printf("[ERROR] Failed to parse URL: %v\n", err)
	}
	host := parsed.Hostname()
	for domain, attr := range sites {
		if strings.EqualFold(host, domain) {
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
