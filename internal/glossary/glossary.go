package glossary

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Reading struct {
	Pinyin string `json:"pinyin"`
	Note   string `json:"note"`
	Wrong  bool   `json:"wrong"`
}

type Entry struct {
	ID           string    `json:"id"`
	Word         string    `json:"word"`
	Pattern      string    `json:"pattern,omitempty"`
	ContextAfter []string  `json:"-"`
	Type         string    `json:"type"`
	Risk         string    `json:"risk"`
	Default      string    `json:"default"`
	Readings     []Reading `json:"readings"`
	Note         string    `json:"note"`
}

type Seed struct {
	Version string  `json:"version"`
	Updated string  `json:"updated"`
	Entries []Entry `json:"entries"`
}

type Match struct {
	Key     string `json:"key"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Text    string `json:"text"`
	Context string `json:"context"`
	Entry   Entry  `json:"entry"`
}

type Detector struct {
	entries  []Entry
	compiled map[string]*regexp.Regexp
	warnings []string
}

var lookaheadPattern = regexp.MustCompile(`^(.+?)\(\?=(.+)\)$`)

func LoadSeed(fsys fs.FS) (Seed, error) {
	data, err := fs.ReadFile(fsys, "glossary.json")
	if err != nil {
		return Seed{}, fmt.Errorf("read glossary seed: %w", err)
	}
	var seed Seed
	if err := json.Unmarshal(data, &seed); err != nil {
		return Seed{}, fmt.Errorf("decode glossary seed: %w", err)
	}
	if len(seed.Entries) == 0 {
		return Seed{}, fmt.Errorf("glossary seed is empty")
	}
	return seed, nil
}

// NormalizePattern converts the JavaScript-style surname lookahead used by
// the static version into an explicit suffix list. Go's RE2 engine does not
// support lookahead, so this keeps the data compatible without backtracking.
func NormalizePattern(word, pattern string) (string, []string) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", nil
	}
	match := lookaheadPattern.FindStringSubmatch(pattern)
	if match == nil || match[1] != word {
		return pattern, nil
	}
	parts := strings.Split(match[2], "|")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return "", out
}

func NewDetector(entries []Entry) *Detector {
	d := &Detector{
		entries:  append([]Entry(nil), entries...),
		compiled: make(map[string]*regexp.Regexp),
	}
	for _, entry := range d.entries {
		if entry.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(entry.Pattern)
		if err != nil {
			d.warnings = append(d.warnings, fmt.Sprintf("term %s: invalid pattern: %v", entry.ID, err))
			continue
		}
		d.compiled[entry.ID] = re
	}
	sort.Slice(d.entries, func(i, j int) bool {
		if len(d.entries[i].Word) == len(d.entries[j].Word) {
			return d.entries[i].ID < d.entries[j].ID
		}
		return len(d.entries[i].Word) > len(d.entries[j].Word)
	})
	return d
}

func (d *Detector) Warnings() []string {
	return append([]string(nil), d.warnings...)
}

func (d *Detector) Find(text string, limit int) ([]Match, bool) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	matches := make([]Match, 0, 64)
	for _, entry := range d.entries {
		if entry.Word == "" {
			continue
		}
		if re := d.compiled[entry.ID]; re != nil {
			for _, loc := range re.FindAllStringIndex(text, -1) {
				end := loc[0] + len(entry.Word)
				if end > len(text) {
					end = len(text)
				}
				matches = append(matches, makeMatch(text, entry, loc[0], end))
			}
			continue
		}
		offset := 0
		for offset < len(text) {
			index := strings.Index(text[offset:], entry.Word)
			if index < 0 {
				break
			}
			start := offset + index
			end := start + len(entry.Word)
			if len(entry.ContextAfter) == 0 || hasAnyPrefix(text[end:], entry.ContextAfter) {
				matches = append(matches, makeMatch(text, entry, start, end))
			}
			offset = end
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Start == matches[j].Start {
			return matches[i].End > matches[j].End
		}
		return matches[i].Start < matches[j].Start
	})

	deduped := make([]Match, 0, len(matches))
	lastEnd := -1
	for _, match := range matches {
		if match.Start < lastEnd {
			continue
		}
		deduped = append(deduped, match)
		lastEnd = match.End
	}
	truncated := len(deduped) > limit
	if truncated {
		deduped = deduped[:limit]
	}
	return deduped, truncated
}

func makeMatch(text string, entry Entry, start, end int) Match {
	return Match{
		Key:     fmt.Sprintf("%s@%d-%d", entry.ID, start, end),
		Start:   start,
		End:     end,
		Text:    text[start:end],
		Context: contextAround(text, start, end),
		Entry:   entry,
	}
}

func hasAnyPrefix(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.HasPrefix(text, candidate) {
			return true
		}
	}
	return false
}

func contextAround(text string, start, end int) string {
	const radius = 28
	runes := []rune(text)
	runeStart := utf8.RuneCountInString(text[:start])
	runeEnd := runeStart + utf8.RuneCountInString(text[start:end])
	left := runeStart - radius
	if left < 0 {
		left = 0
	}
	right := runeEnd + radius
	if right > len(runes) {
		right = len(runes)
	}
	value := string(runes[left:right])
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}
