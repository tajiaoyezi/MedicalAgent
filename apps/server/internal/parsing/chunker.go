package parsing

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// TextSegment 复刻 chunker.ts 的 TextSegment（page/section 可空）。
type TextSegment struct {
	Text           string
	Page           *int
	ParagraphIndex int
	Section        *string
}

const defaultMaxChars = 800

var (
	blankLineRE = regexp.MustCompile(`\n\s*\n`)
	headingRE   = regexp.MustCompile(`^(#{1,6}\s+.+|第[一二三四五六七八九十\d]+[章节].*)$`)
	hashPrefix  = regexp.MustCompile(`^#{1,6}\s+`)
)

func sliceRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ChunkPlainText 复刻 chunkPlainText：按空行分段，小段合并到 ~maxChars，标题作为 section。
func ChunkPlainText(text string, maxChars int) []TextSegment {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	var paragraphs []string
	for _, p := range blankLineRE.Split(text, -1) {
		if t := strings.TrimSpace(p); t != "" {
			paragraphs = append(paragraphs, t)
		}
	}

	segments := []TextSegment{}
	buffer := ""
	var section *string
	index := 0
	flush := func() {
		if strings.TrimSpace(buffer) != "" {
			seg := TextSegment{Text: strings.TrimSpace(buffer), Page: nil, ParagraphIndex: index, Section: section}
			segments = append(segments, seg)
			index++
			buffer = ""
		}
	}

	for _, para := range paragraphs {
		if headingRE.MatchString(para) {
			flush()
			sec := sliceRunes(hashPrefix.ReplaceAllString(para, ""), 120)
			section = &sec
			buffer = para
			flush()
			continue
		}
		if utf8.RuneCountInString(buffer)+utf8.RuneCountInString(para) > maxChars {
			flush()
		}
		if buffer == "" {
			buffer = para
		} else {
			buffer = buffer + "\n" + para
		}
	}
	flush()
	return segments
}

// ChunkFromVisual 复刻 chunkFromVisual：按页码/段落/标题层级切分，保留页码可溯源。
func ChunkFromVisual(result *VisualParseResult) []TextSegment {
	segments := []TextSegment{}
	for _, page := range result.Pages {
		for _, para := range page.Paragraphs {
			if strings.TrimSpace(para.Text) == "" {
				continue
			}
			var section *string
			if para.HeadingLevel != nil && *para.HeadingLevel <= 2 {
				s := sliceRunes(para.Text, 120)
				section = &s
			}
			p := page.Page
			segments = append(segments, TextSegment{
				Text: strings.TrimSpace(para.Text), Page: &p,
				ParagraphIndex: para.ParagraphIndex, Section: section,
			})
		}
	}
	return segments
}
