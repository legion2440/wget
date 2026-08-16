package mirror

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func localPathFor(u *url.URL) string {
	urlPath := u.Path
	var cleaned string
	switch {
	case urlPath == "" || urlPath == "/":
		cleaned = "index.html"
	case strings.HasSuffix(urlPath, "/"):
		cleaned = path.Join(strings.TrimPrefix(path.Clean("/"+urlPath), "/"), "index.html")
	default:
		cleaned = strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		if path.Ext(cleaned) == "" {
			cleaned += ".html"
		}
	}
	if cleaned == "" || cleaned == "." {
		cleaned = "index.html"
	}
	if u.RawQuery != "" {
		cleaned = addQuerySuffix(cleaned, u.RawQuery)
	}
	return filepath.FromSlash(cleaned)
}

func addQuerySuffix(rel, query string) string {
	sum := sha256.Sum256([]byte(query))
	suffix := fmt.Sprintf("__q_%x", sum[:5])
	ext := path.Ext(rel)
	if ext == "" {
		return rel + suffix
	}
	return strings.TrimSuffix(rel, ext) + suffix + ext
}

func relativeLocalLink(current, target string) string {
	fromDir := filepath.Dir(current)
	rel, err := filepath.Rel(fromDir, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}
