package safefile

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadPathCompatibility(t *testing.T) {
	for _, stored := range []string{"./uploads/2025/1/2/icon.png", "uploads/2025/1/2/icon.png", "2025/1/2/icon.png"} {
		if got := UploadURLPath(stored, "./uploads"); got != "/uploads/2025/1/2/icon.png" {
			t.Errorf("UploadURLPath(%q) = %q", stored, got)
		}
	}
}

func TestSaveAndServeUpload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	stored, err := SaveImage(dir, ".png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := UploadPrefix(dir)
	if err != nil {
		t.Fatal(err)
	}
	handler := ServeUploads(dir, prefix)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, UploadURLPath(stored, dir), nil))
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s returned %d %v", method, w.Code, w.Header())
		}
		if method == http.MethodGet && !bytes.Equal(w.Body.Bytes(), data.Bytes()) {
			t.Fatal("saved image changed when serving")
		}
		if method == http.MethodHead && w.Body.Len() != 0 {
			t.Fatal("HEAD returned a body")
		}
	}
	legacy := filepath.Join(dir, "instructions.txt")
	if err := os.WriteFile(legacy, []byte("Legacy documentation"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, UploadURLPath(legacy, dir), nil))
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("legacy file returned %d %v", w.Code, w.Header())
	}
}
