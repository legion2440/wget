package mirror

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func localPathFor(u *url.URL) string {
	urlPath := u.Path
	if urlPath == "" || urlPath == "/" {
		return "index.html"
	}
	cleaned := path.Clean("/" + urlPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if strings.HasSuffix(urlPath, "/") {
		cleaned = path.Join(cleaned, "index.html")
	}
	if cleaned == "" || cleaned == "." {
		cleaned = "index.html"
	}
	return filepath.FromSlash(cleaned)
}

func relativeLocalLink(current, target string) string {
	fromDir := filepath.Dir(current)
	rel, err := filepath.Rel(fromDir, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}
