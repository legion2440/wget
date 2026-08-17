package mirror

import (
	"crypto/sha256"
	"fmt"
	"mime"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func localPathFor(u *url.URL, contentType string) string {
	mediaType := normalizedMediaType(contentType)
	urlPath := u.Path
	var cleaned string
	switch {
	case urlPath == "" || urlPath == "/":
		ext := preferredExtension(mediaType)
		if ext == "" {
			ext = ".html"
		}
		cleaned = "index" + ext
	case strings.HasSuffix(urlPath, "/"):
		dir := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		ext := preferredExtension(mediaType)
		if ext == "" {
			ext = ".html"
		}
		cleaned = path.Join(dir, "index"+ext)
	default:
		cleaned = strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		if path.Ext(cleaned) == "" {
			ext := preferredExtension(mediaType)
			if ext == "" {
				ext = ".bin"
			}
			cleaned += ext
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
	case "":
		return ""
	case "text/html", "application/xhtml+xml":
		return ".html"
	case "text/css":
		return ".css"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	case "application/javascript", "text/javascript":
		return ".js"
	case "application/json":
		return ".json"
	case "application/xml", "text/xml":
		return ".xml"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "application/gzip", "application/x-gzip":
		return ".gz"
	case "application/x-tar":
		return ".tar"
	case "application/wasm":
		return ".wasm"
	case "application/octet-stream":
		return ".bin"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	case "font/woff", "application/font-woff":
		return ".woff"
	case "font/woff2":
		return ".woff2"
	case "font/ttf":
		return ".ttf"
	case "font/otf":
		return ".otf"
	case "application/vnd.ms-fontobject":
		return ".eot"
	case "audio/mpeg":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	}
	if strings.HasSuffix(mediaType, "+json") {
		return ".json"
	}
	if strings.HasSuffix(mediaType, "+xml") {
		return ".xml"
	}
	return ".bin"
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
