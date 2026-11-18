package contenthtml

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	hexColor  = regexp.MustCompile(`(?i)^#(?:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	fontSize      = regexp.MustCompile(`^(?:[8-9]|[1-3][0-9]|4[0-8])px$`)
	fontFamily    = regexp.MustCompile(`^(?:'[a-zA-Z0-9 -]{1,40}'|[a-zA-Z0-9-]{1,40}(?:\s+[a-zA-Z0-9-]{1,40}){0,3})(?:\s*,\s*(?:'[a-zA-Z0-9 -]{1,40}'|[a-zA-Z0-9-]{1,40}(?:\s+[a-zA-Z0-9-]{1,40}){0,3})){0,7}$`)
	letterSpacing = regexp.MustCompile(`^(?:normal|0|-?[0-9]{1,2}(?:\.[0-9]{1,4})?(?:px|rem|em))$`)
	audioPath     = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,200}$`)
	length        = `(?:0|[0-9]{1,4}(?:\.[0-9]{1,4})?(?:px|rem|em|%))`
	calcNumber    = `(?:[0-9]{1,4}(?:\.[0-9]{1,4})?)`
	calcAtom      = `(?:-?` + calcNumber + `(?:px|rem|em|%)?)`
	calcGroup     = `\(\s*` + calcAtom + `(?:\s+[+-]\s+` + calcAtom + `|\s*[*/]\s*` + calcAtom + `)*\s*\)`
	calcOperand   = `(?:` + calcAtom + `|` + calcGroup + `)`
	calcFn        = `calc\(\s*` + calcOperand + `(?:\s+[+-]\s+` + calcOperand + `|\s*[*/]\s*` + calcOperand + `)*\s*\)`
	boundedLength = `(?:` + length + `|` + calcFn + `)`
	spacing       = regexp.MustCompile(`^` + boundedLength + `(?:\s+` + boundedLength + `){0,3}$`)
	marginAtom    = `(?:auto|` + boundedLength + `)`
	marginSpace   = regexp.MustCompile(`^` + marginAtom + `(?:\s+` + marginAtom + `){0,3}$`)
	gridLength    = boundedLength
	gridMinFn  = `min\(\s*` + gridLength + `\s*,\s*` + gridLength + `\s*\)`
	gridFlex   = `(?:[0-9]{1,3}(?:\.[0-9]{1,2})?fr)`
	gridAtom   = `(?:auto|min-content|max-content|` + gridLength + `|` + gridFlex + `)`
	gridMinMax = `minmax\(\s*(?:` + gridAtom + `|` + gridMinFn + `)\s*,\s*(?:` + gridAtom + `|` + gridMinFn + `)\s*\)`
	gridTrack  = `(?:` + gridAtom + `|` + gridMinMax + `)`
	gridRepeat = `repeat\(\s*(?:auto-(?:fit|fill)|[1-9][0-9]?)\s*,\s*` + gridTrack + `(?:\s+` + gridTrack + `){0,3}\s*\)`
	gridItem   = `(?:` + gridTrack + `|` + gridRepeat + `)`
	gridCols   = regexp.MustCompile(`^(?:none|` + gridItem + `(?:\s+` + gridItem + `){0,11})$`)
	flexNum    = `(?:[0-9]{1,2}(?:\.[0-9]{1,2})?)`
	flexBasis  = `(?:auto|content|min-content|max-content|fit-content|` + boundedLength + `)`
	flexValue  = regexp.MustCompile(`^(?:none|auto|initial|` + flexNum + `(?:\s+` + flexNum + `(?:\s+` + flexBasis + `)?|\s+` + flexBasis + `)?|` + flexBasis + `)$`)
	minWidth   = regexp.MustCompile(`^(?:0|auto|min-content|max-content|fit-content|` + boundedLength + `)$`)
	policy     = newPolicy()
)

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("section", "p", "br", "strong", "em", "u", "s", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "ul", "ol", "li", "hr", "span", "div", "table", "thead", "tbody", "tr", "th", "td", "figure", "figcaption", "a", "img", "button", "ruby", "rt", "rp", "rb", "rtc")
	p.AllowNoAttrs().OnElements("rb", "rtc")
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowAttrs("data-audio").Matching(audioPath).OnElements("div", "span", "button")
	p.AllowAttrs("data-play-audio").Matching(regexp.MustCompile(`^$`)).OnElements("button")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^button$`)).OnElements("button")
	p.AllowAttrs("aria-label").OnElements("button")
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,120}$`)).OnElements("div", "span", "button")
	p.AllowAttrs("style").OnElements("section", "p", "span", "div", "strong", "em", "h1", "h2", "h3", "h4", "h5", "h6", "th", "td", "table", "img", "button", "ruby", "rt", "rp", "rb", "rtc")
	p.AllowStyles("color", "background-color").Matching(hexColor).Globally()
	p.AllowStyles("background").Matching(hexColor).Globally()
	p.AllowStyles("font-family").Matching(fontFamily).Globally()
	p.AllowStyles("font-size").Matching(fontSize).Globally()
	p.AllowStyles("font-weight").MatchingEnum("normal", "bold", "400", "500", "600", "700").Globally()
	p.AllowStyles("font-style").MatchingEnum("normal", "italic").Globally()
	p.AllowStyles("letter-spacing").Matching(letterSpacing).Globally()
	p.AllowStyles("text-align").MatchingEnum("left", "center", "right", "justify").Globally()
	p.AllowStyles("text-decoration").MatchingEnum("none", "underline", "line-through").Globally()
	p.AllowStyles("width", "max-width").Matching(regexp.MustCompile(`^(?:100%|[1-9][0-9]{0,2}px)$`)).OnElements("img", "table")
	// Preserve bounded lesson card layouts without permitting executable CSS,
	// URL resources, fixed overlays, arbitrary transforms or z-index.
	cardElements := []string{"section", "div", "button"}
	textAndCardElements := []string{"section", "div", "button", "span", "strong", "em", "p", "h1", "h2", "h3", "h4", "h5", "h6"}
	p.AllowStyles("margin", "margin-top", "margin-right", "margin-bottom", "margin-left").Matching(marginSpace).OnElements(textAndCardElements...)
	p.AllowStyles("padding", "padding-top", "padding-right", "padding-bottom", "padding-left", "border-radius").Matching(spacing).OnElements(textAndCardElements...)
	p.AllowStyles("gap").Matching(spacing).OnElements(cardElements...)
	p.AllowStyles("border").Matching(regexp.MustCompile(`^(?:none|0|[0-9]{1,2}px\s+solid\s+#[0-9a-f]{3}(?:[0-9a-f]{3})?)$`)).OnElements(cardElements...)
	p.AllowStyles("box-shadow").Matching(regexp.MustCompile(`^0\s+[0-9]{1,2}px\s+[0-9]{1,2}px\s+rgba\(\s*0\s*,\s*0\s*,\s*0\s*,\s*(?:0|0\.[0-9]{1,2}|1)\s*\)$`)).OnElements("section", "div")
	p.AllowStyles("display").MatchingEnum("grid", "block", "flex", "inline-flex", "inline-block").OnElements("section", "div")
	p.AllowStyles("display").MatchingEnum("inline", "inline-block", "block", "inline-flex", "flex").OnElements("span", "strong", "em")
	p.AllowStyles("grid-template-columns").Matching(gridCols).OnElements("section", "div")
	p.AllowStyles("box-sizing").MatchingEnum("border-box", "content-box").OnElements("section", "div")
	p.AllowStyles("flex-direction").MatchingEnum("row", "row-reverse", "column", "column-reverse").OnElements("section", "div")
	p.AllowStyles("flex-wrap").MatchingEnum("nowrap", "wrap", "wrap-reverse").OnElements("section", "div")
	p.AllowStyles("justify-content").MatchingEnum("flex-start", "flex-end", "center", "space-between", "space-around", "space-evenly", "start", "end").OnElements("section", "div")
	p.AllowStyles("align-items").MatchingEnum("stretch", "flex-start", "flex-end", "center", "baseline", "start", "end").OnElements("section", "div")
	p.AllowStyles("align-content").MatchingEnum("stretch", "flex-start", "flex-end", "center", "space-between", "space-around", "space-evenly", "start", "end").OnElements("section", "div")
	p.AllowStyles("align-self").MatchingEnum("auto", "stretch", "flex-start", "flex-end", "center", "baseline", "start", "end").OnElements("section", "div")
	p.AllowStyles("flex").Matching(flexValue).OnElements("section", "div")
	p.AllowStyles("flex-grow", "flex-shrink").Matching(regexp.MustCompile(`^` + flexNum + `$`)).OnElements("section", "div")
	p.AllowStyles("flex-basis").Matching(regexp.MustCompile(`^` + flexBasis + `$`)).OnElements("section", "div")
	p.AllowStyles("width").Matching(regexp.MustCompile(`^(?:auto|min-content|max-content|fit-content|` + boundedLength + `)$`)).OnElements("section", "div")
	p.AllowStyles("min-width").Matching(minWidth).OnElements("section", "div")
	p.AllowStyles("max-width").Matching(regexp.MustCompile(`^(?:none|min-content|max-content|fit-content|` + boundedLength + `)$`)).OnElements("section", "div")
	p.AllowStyles("position").MatchingEnum("relative").OnElements("section", "div")
	p.AllowStyles("position").MatchingEnum("absolute").OnElements("button")
	p.AllowStyles("right", "top", "width", "height").Matching(regexp.MustCompile(`^` + boundedLength + `$`)).OnElements("button")
	p.AllowStyles("transform").Matching(regexp.MustCompile(`^translatey\(-50%\)$`)).OnElements("button")
	p.AllowStyles("cursor").MatchingEnum("pointer").OnElements("button")
	p.AllowStyles("line-height").Matching(regexp.MustCompile(`^[1-3](?:\.[0-9]{1,2})?$`)).Globally()
	p.AllowStyles("ruby-align").MatchingEnum("start", "center", "space-between", "space-around").OnElements("ruby", "rb", "rtc")
	p.AllowStyles("ruby-position").MatchingEnum("over", "under", "inter-character", "alternate").OnElements("ruby", "rt", "rtc")
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
