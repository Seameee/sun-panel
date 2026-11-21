package safefile

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrUploadPath = errors.New("invalid upload path")

// UploadPrefix preserves the public path of legacy relative source_path values.
func UploadPrefix(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", ErrUploadPath
	}
	name := filepath.ToSlash(filepath.Clean(dir))
	if filepath.IsAbs(dir) {
		name = strings.TrimPrefix(name, "/")
	}
	if !validRelativePath(name) || strings.Contains(name, "*") {
		return "", ErrUploadPath
	}
	return "/" + name, nil
}

func validRelativePath(name string) bool {
	return name != "." && fs.ValidPath(name) && filepath.IsLocal(filepath.FromSlash(name)) && !strings.ContainsAny(name, "\\:\x00")
}

// StoredRelativePath accepts legacy disk paths and preview paths relative to
// the upload directory. Callers must use os.Root for filesystem operations.
func StoredRelativePath(dir, stored string) (string, error) {
	if stored == "" || strings.ContainsAny(stored, "\\\x00") {
		return "", ErrUploadPath
	}
	for _, part := range strings.Split(filepath.ToSlash(stored), "/") {
		if part == ".." {
			return "", ErrUploadPath
		}
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	name, err := filepath.Abs(stored)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, name)
	if err == nil && validRelativePath(filepath.ToSlash(rel)) {
		return filepath.ToSlash(rel), nil
	}
	if validRelativePath(stored) {
		return stored, nil
	}
	return "", ErrUploadPath
}

func UploadURLPath(stored, dir string) string {
	prefix, err := UploadPrefix(dir)
	if err != nil {
		return ""
	}
	rel, err := StoredRelativePath(dir, stored)
	if err != nil {
		return ""
	}
	return (&url.URL{Path: prefix + "/" + rel}).EscapedPath()
}

// ServeUploads serves only files contained by the configured root. It checks
// both the bytes and extension before serving anything inline.
func ServeUploads(dir, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, prefix+"/")
		if !ok || !validRelativePath(name) {
			http.NotFound(w, r)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer root.Close()
		localName := filepath.FromSlash(name)
		info, err := root.Stat(localName)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		f, err := root.Open(localName)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err = f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		var head [SniffLen]byte
		n, err := f.ReadAt(head[:], 0)
		if n == 0 && err != nil && info.Size() != 0 {
			http.NotFound(w, r)
			return
		}
		ext := ExtOf(info.Name())
		if detected, ok := DetectImageExt(head[:n]); !ok || !SameImageType(ext, detected) {
			ext = ""
		}
		ApplyServeHeaders(w.Header(), ext, filepath.Base(localName))
		http.ServeContent(w, r, filepath.Base(localName), info.ModTime(), f)
	}
}

// SaveImage writes validated bytes beneath a dated legacy upload directory.
func SaveImage(dir, ext string, data []byte) (string, error) {
	if _, err := UploadPrefix(dir); err != nil || !IsAllowedUploadExt(ext) {
		return "", ErrUploadPath
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	year, month, day := time.Now().Date()
	date := fmt.Sprintf("%d/%d/%d", year, month, day)
	if err := root.MkdirAll(filepath.FromSlash(date), 0o755); err != nil {
		return "", err
	}
	name := filepath.Join(filepath.FromSlash(date), rand.Text()+ext)
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", errors.Join(err, root.Remove(name))
	}
	// Keep the legacy disk-path format for the default relative configuration.
	return strings.TrimRight(filepath.ToSlash(dir), "/") + "/" + filepath.ToSlash(name), nil
}

func RemoveUpload(dir, stored string) error {
	if _, err := UploadPrefix(dir); err != nil {
		return err
	}
	rel, err := StoredRelativePath(dir, stored)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.FromSlash(rel)
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrUploadPath
	}
	return root.Remove(name)
}
