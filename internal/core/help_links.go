package core

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

//go:embed help_links.json
var helpLinksJSON []byte

var helpLinks, helpLinksError = parseHelpLinks(helpLinksJSON)

func parseHelpLinks(data []byte) (map[string]string, error) {
	var links map[string]string
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("invalid help link catalog: %w", err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("empty help link catalog")
	}
	for key, address := range links {
		parsed, err := url.Parse(address)
		if key == "" || err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || strings.ContainsFunc(address, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return nil, fmt.Errorf("invalid help link: %s", key)
		}
	}
	return links, nil
}
