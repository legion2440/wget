package mirror

import (
	"html"
	"net/url"
	"sort"
	"strings"
)

type htmlReplacement struct {
	start, end int
	value      string
}
type htmlAttribute struct {
	name                 string
	valueStart, valueEnd int
	hasValue             bool
}

func (m *Mirrorer) processHTML(data []byte, baseURL *url.URL, currentLocal string, convert bool) ([]byte, []*url.URL, error) {
	text := string(data)
	var links []*url.URL
	var replacements []htmlReplacement
	lower := asciiLower(text)
	for pos := 0; pos < len(text); {
		relOpen := strings.IndexByte(text[pos:], '<')
		if relOpen < 0 {
			break
		}
		open := pos + relOpen
		if strings.HasPrefix(text[open:], "<!--") {
			end := strings.Index(text[open+4:], "-->")
			if end < 0 {
				break
			}
			pos = open + 4 + end + 3
			continue
		}
		close := findTagEnd(text, open+1)
		if close < 0 {
			break
		}
		tag, closing, attrs := parseTag(text, open+1, close)
		if tag == "" || closing {
			pos = close + 1
			continue
		}
		wanted := ""
		switch tag {
		case "a", "link":
			wanted = "href"
		case "img":
			wanted = "src"
		}
		for _, attr := range attrs {
			if !attr.hasValue {
				continue
			}
			raw := html.UnescapeString(text[attr.valueStart:attr.valueEnd])
			if attr.name == wanted && wanted != "" {
				resolved := resolveReference(baseURL, raw)
				if resolved != nil && m.isAllowed(resolved) && !m.shouldSkip(resolved) {
					links = append(links, resolved)
					if convert {
						if converted, ok := m.convertedLink(currentLocal, resolved); ok {
							replacements = append(replacements, htmlReplacement{attr.valueStart, attr.valueEnd, converted})
						}
					}
				}
			}
			if attr.name == "style" {
				processed, found := m.processCSS([]byte(raw), baseURL, currentLocal, convert)
				links = append(links, found...)
				if convert && string(processed) != raw {
					replacements = append(replacements, htmlReplacement{attr.valueStart, attr.valueEnd, html.EscapeString(string(processed))})
				}
			}
		}
		if tag == "script" {
			if endStart := strings.Index(lower[close+1:], "</script"); endStart >= 0 {
				endStart += close + 1
				if endTag := findTagEnd(text, endStart+2); endTag >= 0 {
					pos = endTag + 1
					continue
				}
			}
		}
		if tag == "style" {
			if endStart := strings.Index(lower[close+1:], "</style"); endStart >= 0 {
				endStart += close + 1
				bodyStart := close + 1
				processed, found := m.processCSS([]byte(text[bodyStart:endStart]), baseURL, currentLocal, convert)
				links = append(links, found...)
				if convert && string(processed) != text[bodyStart:endStart] {
					replacements = append(replacements, htmlReplacement{bodyStart, endStart, string(processed)})
				}
				if endTag := findTagEnd(text, endStart+2); endTag >= 0 {
					pos = endTag + 1
					continue
				}
			}
		}
		pos = close + 1
	}
	if len(replacements) == 0 {
		return data, links, nil
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
	var out strings.Builder
	last := 0
	for _, r := range replacements {
		if r.start < last {
			continue
		}
		out.WriteString(text[last:r.start])
		out.WriteString(r.value)
		last = r.end
	}
	out.WriteString(text[last:])
	return []byte(out.String()), links, nil
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
func findTagEnd(text string, start int) int {
	var quote byte
	for i := start; i < len(text); i++ {
		ch := text[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '>' {
			return i
		}
	}
	return -1
}
func parseTag(text string, start, end int) (string, bool, []htmlAttribute) {
	i := start
	for i < end && isHTMLSpace(text[i]) {
		i++
	}
	closing := false
	if i < end && text[i] == '/' {
		closing = true
		i++
		for i < end && isHTMLSpace(text[i]) {
			i++
		}
	}
	if i >= end || text[i] == '!' || text[i] == '?' {
		return "", closing, nil
	}
	tagStart := i
	for i < end && isNameChar(text[i]) {
		i++
	}
	tag := asciiLower(text[tagStart:i])
	var attrs []htmlAttribute
	for i < end {
		for i < end && (isHTMLSpace(text[i]) || text[i] == '/') {
			i++
		}
		if i >= end {
			break
		}
		nameStart := i
		for i < end && isNameChar(text[i]) {
			i++
		}
		if nameStart == i {
			i++
			continue
		}
		name := asciiLower(text[nameStart:i])
		for i < end && isHTMLSpace(text[i]) {
			i++
		}
		if i >= end || text[i] != '=' {
			attrs = append(attrs, htmlAttribute{name: name})
			continue
		}
		i++
		for i < end && isHTMLSpace(text[i]) {
			i++
		}
		if i >= end {
			break
		}
		valueStart, valueEnd := i, i
		if text[i] == '\'' || text[i] == '"' {
			q := text[i]
			i++
			valueStart = i
			for i < end && text[i] != q {
				i++
			}
			valueEnd = i
			if i < end {
				i++
			}
		} else {
			valueStart = i
			for i < end && !isHTMLSpace(text[i]) && text[i] != '>' {
				i++
			}
			valueEnd = i
		}
		attrs = append(attrs, htmlAttribute{name: name, valueStart: valueStart, valueEnd: valueEnd, hasValue: true})
	}
	return tag, closing, attrs
}
func isHTMLSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}
func isNameChar(ch byte) bool {
	return ch == '-' || ch == '_' || ch == ':' || ch == '.' || ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}
func resolveReference(base *url.URL, value string) *url.URL {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") {
		return nil
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"data:", "mailto:", "javascript:", "tel:"} {
		if strings.HasPrefix(lower, prefix) {
			return nil
		}
	}
	ref, err := url.Parse(value)
	if err != nil {
		return nil
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return nil
	}
	return resolved
}
