package contenthtml

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	hexColor  = regexp.MustCompile(`(?i)^#(?:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	fontSize  = regexp.MustCompile(`^(?:[8-9]|[1-3][0-9]|4[0-8])px$`)
	audioPath = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,200}$`)
	policy    = newPolicy()
)

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "u", "s", "h1", "h2", "h3", "blockquote", "ul", "ol", "li", "hr", "span", "div", "table", "thead", "tbody", "tr", "th", "td", "figure", "figcaption", "a", "img", "button")
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowAttrs("data-audio").Matching(audioPath).OnElements("div", "span")
	p.AllowAttrs("data-play-audio").Matching(regexp.MustCompile(`^$`)).OnElements("button")
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,120}$`)).OnElements("div", "span", "button")
	p.AllowAttrs("style").OnElements("p", "span", "div", "h1", "h2", "h3", "th", "td", "table", "img")
	p.AllowStyles("color", "background-color").Matching(hexColor).Globally()
	p.AllowStyles("font-size").Matching(fontSize).Globally()
	p.AllowStyles("font-weight").MatchingEnum("normal", "bold", "400", "500", "600", "700").Globally()
	p.AllowStyles("font-style").MatchingEnum("normal", "italic").Globally()
	p.AllowStyles("text-align").MatchingEnum("left", "center", "right", "justify").Globally()
	p.AllowStyles("text-decoration").MatchingEnum("none", "underline", "line-through").Globally()
	p.AllowStyles("width", "max-width").Matching(regexp.MustCompile(`^(?:100%|[1-9][0-9]{0,2}px)$`)).OnElements("img", "table")
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	return p
}

func HTML(input string) string { return policy.Sanitize(input) }

// JSON accepts the existing lesson JSONB representation and preserves its
// extra keys while sanitizing the only field that is rendered as HTML.
func JSON(input []byte) ([]byte, error) {
	if len(input) == 0 || string(input) == "null" {
		return json.Marshal(map[string]string{"content": "", "tips": ""})
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		// Older lessons may contain a JSON string holding HTML.
		var legacy string
		if err := json.Unmarshal(input, &legacy); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"content": HTML(legacy), "tips": ""})
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	var html string
	if raw, ok := fields["content"]; ok {
		if err := json.Unmarshal(raw, &html); err != nil {
			return nil, err
		}
	}
	clean, _ := json.Marshal(HTML(html))
	fields["content"] = clean
	var tips string
	if raw, ok := fields["tips"]; ok {
		if err := json.Unmarshal(raw, &tips); err != nil {
			return nil, err
		}
	}
	tipsRaw, _ := json.Marshal(strings.TrimSpace(tips))
	fields["tips"] = tipsRaw
	return json.Marshal(fields)
}
