package mirror

import (
	"crypto/sha256"
	"fmt"
	"mime"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func localPathFor(u *url.URL, contentType string) string {
	mediaType := normalizedMediaType(contentType)
	urlPath := u.Path
	var cleaned string
	switch {
	case urlPath == "" || urlPath == "/":
		cleaned = "index" + preferredExtension(mediaType)
		if cleaned == "index" && mediaType == "" {
			cleaned = "index.html"
		}
	case strings.HasSuffix(urlPath, "/"):
		dir := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		ext := preferredExtension(mediaType)
		if ext == "" && mediaType == "" {
			ext = ".html"
		}
		cleaned = path.Join(dir, "index"+ext)
	default:
		cleaned = strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		if path.Ext(cleaned) == "" {
			cleaned += preferredExtension(mediaType)
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

func normalizedMediaType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return strings.ToLower(mediaType)
}

func preferredExtension(mediaType string) string {
	switch mediaType {
	case "text/html", "application/xhtml+xml":
		return ".html"
	case "text/css":
		return ".css"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "application/javascript", "text/javascript":
		return ".js"
	case "application/json":
		return ".json"
	case "text/plain":
		return ".txt"
	}
	if mediaType == "" {
		return ""
	}
	exts, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	sort.Slice(exts, func(i, j int) bool {
		if len(exts[i]) == len(exts[j]) {
			return exts[i] < exts[j]
		}
		return len(exts[i]) < len(exts[j])
	})
	return exts[0]
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
func (m *Mirrorer) convertedLink(current string, target *url.URL) (string, bool) {
	local, ok := m.paths[canonicalKey(target)]
	if !ok {
		return "", false
	}
	converted := relativeLocalLink(current, local)
	if target.Fragment != "" {
		converted += "#" + target.Fragment
	}
	return converted, true
}
