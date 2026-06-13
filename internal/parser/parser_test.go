package parser

import (
	"net/url"
	"strings"
	"testing"
)

func TestExtract_Title(t *testing.T) {
	html := `<html><head><title>Hello World</title></head><body><a href="/page">link</a></body></html>`
	base, _ := url.Parse("https://example.com")
	page, err := Extract(strings.NewReader(html), base)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", page.Title)
	}
	if len(page.Links) != 1 || page.Links[0] != "https://example.com/page" {
		t.Errorf("unexpected links: %v", page.Links)
	}
}

func TestExtract_ExternalLink(t *testing.T) {
	html := `<html><body><a href="https://other.com/foo">ext</a></body></html>`
	base, _ := url.Parse("https://example.com")
	page, err := Extract(strings.NewReader(html), base)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Links) != 1 || page.Links[0] != "https://other.com/foo" {
		t.Errorf("unexpected links: %v", page.Links)
	}
}

func TestExtract_SkipsMailto(t *testing.T) {
	html := `<html><body><a href="mailto:a@b.com">mail</a></body></html>`
	base, _ := url.Parse("https://example.com")
	page, _ := Extract(strings.NewReader(html), base)
	if len(page.Links) != 0 {
		t.Errorf("expected no links, got %v", page.Links)
	}
}
