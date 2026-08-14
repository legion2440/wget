package mirror

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	cssURLPattern    = regexp.MustCompile(`(?i)url\(\s*['"]?([^'"\)]+)['"]?\s*\)`)
	cssImportPattern = regexp.MustCompile(`(?i)@import\s+['"]([^'"]+)['"]`)
)

func (m *Mirrorer) processCSS(data []byte, baseURL *url.URL, currentLocal string) ([]byte, []*url.URL) {
	text := string(data)
	var links []*url.URL
	text, first := m.rewriteCSSMatches(text, cssURLPattern, baseURL, currentLocal, "url(\"%s\")")
	links = append(links, first...)
	text, second := m.rewriteCSSMatches(text, cssImportPattern, baseURL, currentLocal, "@import \"%s\"")
	links = append(links, second...)
	return []byte(text), links
}

func (m *Mirrorer) rewriteCSSMatches(text string, pattern *regexp.Regexp, baseURL *url.URL, currentLocal, replacementFormat string) (string, []*url.URL) {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	var links []*url.URL
	var out strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		fullStart, fullEnd := match[0], match[1]
		valueStart, valueEnd := match[2], match[3]
		if valueStart < 0 || valueEnd < 0 {
			continue
		}
		value := text[valueStart:valueEnd]
		resolved := resolveReference(baseURL, value)
		if resolved == nil || !m.isAllowed(resolved) || m.shouldSkip(resolved) {
			continue
		}
		links = append(links, resolved)
		if !m.opts.ConvertLinks {
			continue
		}
		converted := relativeLocalLink(currentLocal, localPathFor(resolved))
		if resolved.Fragment != "" {
			converted += "#" + resolved.Fragment
		}
		out.WriteString(text[last:fullStart])
		out.WriteString(strings.Replace(replacementFormat, "%s", converted, 1))
		last = fullEnd
	}
	if last == 0 {
		return text, links
	}
	out.WriteString(text[last:])
	return out.String(), links
}
