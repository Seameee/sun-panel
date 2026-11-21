package siteFavicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sun-panel/lib/safefile"
	"sun-panel/lib/safeurl"

	"github.com/PuerkitoBio/goquery"
)

const (
	maxHTMLBytes = 512 << 10
	maxIconBytes = 1 << 20
	userAgent    = "Sun-Panel favicon fetcher"
)

var client = safeurl.NewClient(0)

func DownloadSiteFavicon(ctx context.Context, pageURL, uploadDir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, safeurl.DefaultTimeout)
	defer cancel()
	page, finalURL, err := get(ctx, pageURL, maxHTMLBytes)
	if err != nil {
		return "", err
	}
	iconURL, err := findIconURL(finalURL, page)
	if err != nil {
		return "", err
	}
	data, _, err := get(ctx, iconURL, maxIconBytes)
	if err != nil {
		return "", err
	}
	ext, ok := safefile.DetectImageExt(data[:min(len(data), safefile.SniffLen)])
	if !ok {
		return "", errors.New("favicon is not a supported image")
	}
	return safefile.SaveImage(uploadDir, ext, data)
}

func get(ctx context.Context, rawURL string, limit int64) ([]byte, *url.URL, error) {
	req, err := safeurl.NewRequest(http.MethodGet, rawURL)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := safeurl.Do(client, req.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, nil, safeurl.ErrBodyTooLarge
	}
	data, err := safeurl.ReadAllLimited(resp.Body, limit)
	return data, resp.Request.URL, err
}

func findIconURL(base *url.URL, page []byte) (string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return "", err
	}
	var icon string
	doc.Find("link[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		rel, _ := s.Attr("rel")
		href, _ := s.Attr("href")
		for _, value := range strings.Fields(strings.ToLower(rel)) {
			switch value {
			case "icon", "apple-touch-icon", "apple-touch-icon-precomposed", "mask-icon":
				if resolved, err := resolveIconURL(base, href); err == nil {
					icon = resolved
					return false
				}
			}
		}
		return true
	})
	if icon == "" {
		return "", errors.New("favicon not found on the page")
	}
	return icon, nil
}

func resolveIconURL(base *url.URL, href string) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" || base == nil {
		return "", safeurl.ErrInvalidURL
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	abs := base.ResolveReference(ref)
	abs.Fragment = ""
	validated, err := safeurl.Parse(abs.String())
	if err != nil {
		return "", err
	}
	return validated.String(), nil
}
