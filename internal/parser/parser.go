// Package parser extracts links and metadata from HTML pages.
package parser

import (
	"golang.org/x/net/html"
	"io"
	"net/url"
	"strings"
)

// Page holds extracted data from a crawled HTML document.
type Page struct {
	Title       string
	Links       []string
	MetaDesc    string
	WordCount   int
	StatusCode  int
}

// Extract parses r as HTML, resolving all discovered href links
// against base, and returns a Page.
func Extract(r io.Reader, base *url.URL) (*Page, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	page := &Page{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			switch n.Data {
			case "title":
				if n.FirstChild != nil {
					page.Title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "a":
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						if link := resolveLink(attr.Val, base); link != "" {
							page.Links = append(page.Links, link)
						}
					}
				}
			case "meta":
				var name, content string
				for _, attr := range n.Attr {
					switch attr.Key {
					case "name":
						name = strings.ToLower(attr.Val)
					case "content":
						content = attr.Val
					}
				}
				if name == "description" {
					page.MetaDesc = content
				}
			case "script", "style", "noscript":
				return // skip these subtrees
			}
		case html.TextNode:
			words := strings.Fields(n.Data)
			page.WordCount += len(words)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return page, nil
}

func resolveLink(href string, base *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "javascript:") {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(u)
	resolved.Fragment = ""
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}
