package mirror

import (
	"html"
	"net/url"
	"strings"
)

type htmlReplacement struct {
	start int
	end   int
	value string
}

// processHTML scans start tags without requiring a full browser-grade DOM.
// It intentionally focuses on the assignment tags (a, link, img) and also
// follows script[src] so mirrored pages that rely on local JavaScript keep working.
func (m *Mirrorer) processHTML(data []byte, baseURL *url.URL, currentLocal string) ([]byte, []*url.URL, error) {
	text := string(data)
	var links []*url.URL
	var replacements []htmlReplacement

	for pos := 0; pos < len(text); {
		open := strings.IndexByte(text[pos:], '<')
		if open < 0 {
			break
		}
		open += pos
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
		_, attrName, attrStart := tagAndAttribute(text, open+1, close)
		if attrName == "" {
			pos = close + 1
			continue
		}
		valueStart, valueEnd, ok := findAttributeValue(text, attrStart, close, attrName)
		if !ok {
			pos = close + 1
			continue
		}
		value := html.UnescapeString(text[valueStart:valueEnd])
		resolved := resolveReference(baseURL, value)
		if resolved != nil && m.isAllowed(resolved) && !m.shouldSkip(resolved) {
			links = append(links, resolved)
			if m.opts.ConvertLinks {
				fragment := resolved.Fragment
				converted := relativeLocalLink(currentLocal, localPathFor(resolved))
				if fragment != "" {
					converted += "#" + fragment
				}
				replacements = append(replacements, htmlReplacement{start: valueStart, end: valueEnd, value: converted})
			}
		}
		pos = close + 1
	}

	if len(replacements) == 0 {
		return data, links, nil
	}
	var out strings.Builder
	last := 0
	for _, replacement := range replacements {
		out.WriteString(text[last:replacement.start])
		out.WriteString(replacement.value)
		last = replacement.end
	}
	out.WriteString(text[last:])
	return []byte(out.String()), links, nil
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

func tagAndAttribute(text string, start, end int) (string, string, int) {
	i := start
	for i < end && isHTMLSpace(text[i]) {
		i++
	}
	if i >= end || text[i] == '/' || text[i] == '!' || text[i] == '?' {
		return "", "", i
	}
	tagStart := i
	for i < end && isNameChar(text[i]) {
		i++
	}
	tag := strings.ToLower(text[tagStart:i])
	switch tag {
	case "a", "link":
		return tag, "href", i
	case "img", "script":
		return tag, "src", i
	default:
		return tag, "", i
	}
}

func findAttributeValue(text string, start, end int, wanted string) (int, int, bool) {
	for i := start; i < end; {
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
		name := strings.ToLower(text[nameStart:i])
		for i < end && isHTMLSpace(text[i]) {
			i++
		}
		if i >= end || text[i] != '=' {
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
			quote := text[i]
			i++
			valueStart = i
			for i < end && text[i] != quote {
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
		if name == wanted {
			return valueStart, valueEnd, true
		}
	}
	return 0, 0, false
}

func isHTMLSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
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
