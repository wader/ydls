package linkicon

import (
	"net/url"
	"regexp"
	"strconv"
)

// TODO: make icon types/guess table arguments?
// TODO: proper parser?

var linkIconRe = regexp.MustCompile(`` +
	`<\s*link\s+` +
	`(?:` +
	`(?:rel=".*?(?P<rel>icon|apple-touch-icon|fluid-icon).*?")|` +
	`(?:href="(?P<href>.*?)")|` +
	`(?:sizes="(?P<width>\d+)x(?P<height>\d+)")|` +
	`.*?` +
	`)*` +
	`\s*/?\s*>` +
	``)

var linkIconGuessWidth = map[string]int{
	"icon":             32,
	"apple-touch-icon": 114,
	"fluid-icon":       256,
}

func Find(baseRawURL string, html string) (string, error) {
	baseURL, baseURLErr := url.Parse(baseRawURL)
	if baseURLErr != nil {
		return "", baseURLErr
	}

	matches := linkIconRe.FindAllStringSubmatch(html, -1)
	if matches == nil {
		return "", nil
	}

	bestHref := ""
	bestWidth := 0
	for _, match := range matches {
		groups := map[string]string{}
		for i, name := range linkIconRe.SubexpNames() {
			if i != 0 {
				groups[name] = match[i]
			}
		}

		rel := groups["rel"]
		href := groups["href"]
		width, _ := strconv.Atoi(groups["width"])
		if width == 0 {
			width = linkIconGuessWidth[rel]
		}
		if href == "" || rel == "" {
			continue
		}
		if bestHref != "" && width <= bestWidth {
			continue
		}

		bestHref = href
		bestWidth = width
	}

	bestURL, bestURLErr := url.Parse(bestHref)
	if bestURLErr != nil {
		return "", bestURLErr
	}

	return baseURL.ResolveReference(bestURL).String(), nil
}
