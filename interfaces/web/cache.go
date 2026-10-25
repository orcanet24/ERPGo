package web

import (
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// fileETag builds a strong validator from a file's modification time and
// size: no content is read, so it is cheap on every request. Returns "" (no
// ETag header) when the file is missing, is not a regular file, or comes from
// the embedded filesystem — embed reports the zero ModTime, which cannot
// detect changes anyway (a changed asset only ships with a new binary).
func fileETag(fsys fs.FS, name string) string {
	f, err := fsys.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	d, err := f.Stat()
	if err != nil || d.IsDir() || d.ModTime().IsZero() {
		return ""
	}
	return `"` + strconv.FormatInt(d.ModTime().UnixNano(), 10) + "-" +
		strconv.FormatInt(d.Size(), 10) + `"`
}

// setStaticETag pre-computes the ETag for a /static/ URL before
// http.FileServer runs. net/http's serveContent reads the ETag from the
// response header (checkIfNoneMatch) and replies 304 Not Modified without
// reading the body, so revalidation stays a single stat() on both sides.
// The URL is cleaned first, so only paths really under /static/ are probed.
func setStaticETag(w http.ResponseWriter, root fs.FS, urlPath string) {
	clean := path.Clean("/" + urlPath)
	if !strings.HasPrefix(clean, "/static/") {
		return
	}
	rel := strings.TrimPrefix(clean, "/static/")
	if rel == "" || rel == "." {
		return
	}
	if etag := fileETag(root, rel); etag != "" {
		w.Header().Set("ETag", etag)
	}
}
