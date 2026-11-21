package safefile

import (
	"net/http"
	"strings"
	"testing"
)

func TestDetectImageExt(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if ext, ok := DetectImageExt(png); !ok || ext != ".png" {
		t.Fatalf("png: got %q ok=%v", ext, ok)
	}

	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
	if ext, ok := DetectImageExt(jpeg); !ok || ext != ".jpg" {
		t.Fatalf("jpeg: got %q ok=%v", ext, ok)
	}

	gif := []byte("GIF89a....")
	if ext, ok := DetectImageExt(gif); !ok || ext != ".gif" {
		t.Fatalf("gif: got %q ok=%v", ext, ok)
	}

	webp := []byte("RIFF\x1a\x00\x00\x00WEBPVP8 ")
	if ext, ok := DetectImageExt(webp); !ok || ext != ".webp" {
		t.Fatalf("webp: got %q ok=%v", ext, ok)
	}

	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	if ext, ok := DetectImageExt(svg); !ok || ext != ".svg" {
		t.Fatalf("svg: got %q ok=%v", ext, ok)
	}

	html := []byte(`<!DOCTYPE html><html><script>alert(1)</script></html>`)
	if ext, ok := DetectImageExt(html); ok {
		t.Fatalf("html must not be accepted as image, got %q", ext)
	}

	if _, ok := DetectImageExt(nil); ok {
		t.Fatal("empty payload must be rejected")
	}
}

func TestUploadExtPolicy(t *testing.T) {
	for _, ext := range []string{".png", ".JPG", "jpeg", ".gif", ".webp", ".svg", ".ico", ".avif", ".heic"} {
		if !IsAllowedUploadExt(ext) {
			t.Errorf("IsAllowedUploadExt(%q) = false", ext)
		}
	}
	for _, ext := range []string{".html", ".js", ".php", ".exe", "", ".zip"} {
		if IsAllowedUploadExt(ext) {
			t.Errorf("IsAllowedUploadExt(%q) = true", ext)
		}
	}
}

func TestApplyServeHeadersSVG(t *testing.T) {
	h := http.Header{}
	ApplyServeHeaders(h, ".svg", "logo.svg")

	if got := h.Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if !strings.Contains(h.Get("Content-Disposition"), "inline") {
		t.Fatalf("Content-Disposition = %q", h.Get("Content-Disposition"))
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("CSP must contain sandbox: %q", h.Get("Content-Security-Policy"))
	}
}

func TestApplyServeHeadersActiveContent(t *testing.T) {
	h := http.Header{}
	ApplyServeHeaders(h, ".html", "legacy.html")

	if got := h.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
		t.Fatalf("Content-Disposition = %q", h.Get("Content-Disposition"))
	}
}

func TestFilenameDispositionSanitizes(t *testing.T) {
	got := FilenameDisposition("inline", "a\"b\r\nc.png")
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("header injection not stripped: %q", got)
	}
	if !strings.Contains(got, "filename*=UTF-8''") && !strings.Contains(got, `filename="a_b__c.png"`) {
		t.Fatalf("unexpected disposition: %q", got)
	}
}
