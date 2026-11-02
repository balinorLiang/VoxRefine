package glossary

import (
	"strings"
	"testing"
	"testing/fstest"

	glossarydata "polyphone-checker/data"
)

func TestLoadSeed(t *testing.T) {
	fsys := fstest.MapFS{
		"glossary.json": &fstest.MapFile{Data: []byte(`{
			"version": "test-v1",
			"entries": [{"id": "chongqing", "word": "重庆", "type": "place", "risk": "high", "default": "chong2 qing4"}]
		}`)},
	}
	seed, err := LoadSeed(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if seed.Version != "test-v1" || len(seed.Entries) != 1 || seed.Entries[0].ID != "chongqing" {
		t.Fatalf("unexpected seed: %#v", seed)
	}
}

func TestLoadSeedRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
	}{
		{name: "missing", fsys: fstest.MapFS{}},
		{name: "invalid json", fsys: fstest.MapFS{"glossary.json": &fstest.MapFile{Data: []byte(`{`)}}},
		{name: "empty entries", fsys: fstest.MapFS{"glossary.json": &fstest.MapFile{Data: []byte(`{"version":"v1","entries":[]}`)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadSeed(test.fsys); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestBundledGlossaryIsStructurallyValid(t *testing.T) {
	seed, err := LoadSeed(glossarydata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed.Entries) < 100 {
		t.Fatalf("bundled glossary has only %d entries", len(seed.Entries))
	}
	allowedTypes := map[string]bool{"place": true, "surname": true, "word": true, "name": true, "idiom": true, "brand": true}
	allowedRisks := map[string]bool{"high": true, "medium": true}
	ids := make(map[string]bool, len(seed.Entries))
	for index, entry := range seed.Entries {
		if entry.ID == "" || entry.Word == "" || entry.Type == "" || entry.Risk == "" || entry.Default == "" {
			t.Fatalf("entry %d has empty required fields: %#v", index, entry)
		}
		if ids[entry.ID] {
			t.Fatalf("duplicate term id %q", entry.ID)
		}
		ids[entry.ID] = true
		if !allowedTypes[entry.Type] {
			t.Fatalf("term %s has invalid type %q", entry.ID, entry.Type)
		}
		if !allowedRisks[entry.Risk] {
			t.Fatalf("term %s has invalid risk %q", entry.ID, entry.Risk)
		}
		if len(entry.Readings) == 0 {
			t.Fatalf("term %s has no readings", entry.ID)
		}
		for _, reading := range entry.Readings {
			if strings.TrimSpace(reading.Pinyin) == "" {
				t.Fatalf("term %s has an empty reading pinyin", entry.ID)
			}
		}
	}
}

func TestNormalizePatternEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		word        string
		pattern     string
		wantPattern string
		wantSuffix  []string
	}{
		{name: "empty", word: "曾", pattern: "  "},
		{name: "regular regex", word: "重", pattern: "重[庆]", wantPattern: "重[庆]"},
		{name: "lookahead for another word", word: "区", pattern: "曾(?=先生)", wantPattern: "曾(?=先生)"},
		{name: "deduplicates and trims", word: "曾", pattern: "曾(?= 先生 |女士|先生| )", wantSuffix: []string{"先生", "女士"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pattern, suffixes := NormalizePattern(test.word, test.pattern)
			if pattern != test.wantPattern {
				t.Fatalf("pattern = %q, want %q", pattern, test.wantPattern)
			}
			if strings.Join(suffixes, ",") != strings.Join(test.wantSuffix, ",") {
				t.Fatalf("suffixes = %#v, want %#v", suffixes, test.wantSuffix)
			}
		})
	}
}

func TestDetectorCompilesPatternsAndReportsWarnings(t *testing.T) {
	detector := NewDetector([]Entry{
		{ID: "valid", Word: "重庆", Pattern: "重[庆]", Type: "place", Risk: "high"},
		{ID: "invalid", Word: "银行", Pattern: "(", Type: "word", Risk: "high"},
	})
	matches, _ := detector.Find("重庆和银行", 100)
	if len(matches) != 2 || matches[0].Entry.ID != "valid" || matches[1].Entry.ID != "invalid" {
		t.Fatalf("matches = %#v", matches)
	}
	warnings := detector.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "invalid") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestDetectorLimitBounds(t *testing.T) {
	detector := NewDetector([]Entry{{ID: "chongqing", Word: "重庆", Type: "place", Risk: "high"}})

	matches, truncated := detector.Find(strings.Repeat("重庆", 201), 0)
	if len(matches) != 200 || !truncated {
		t.Fatalf("default limit = %d, truncated=%v", len(matches), truncated)
	}

	matches, truncated = detector.Find(strings.Repeat("重庆", 501), 9999)
	if len(matches) != 500 || !truncated {
		t.Fatalf("capped limit = %d, truncated=%v", len(matches), truncated)
	}
}

func TestDetectorContextIsRuneSafeAndCompact(t *testing.T) {
	detector := NewDetector([]Entry{{ID: "chongqing", Word: "重庆", Type: "place", Risk: "high"}})
	text := strings.Repeat("前", 40) + "\n重庆\r\n" + strings.Repeat("后", 40)
	matches, _ := detector.Find(text, 10)
	if len(matches) != 1 {
		t.Fatalf("matches = %d", len(matches))
	}
	if strings.ContainsAny(matches[0].Context, "\r\n") {
		t.Fatalf("context contains newlines: %q", matches[0].Context)
	}
	if !strings.Contains(matches[0].Context, "重庆") {
		t.Fatalf("context did not retain the match: %q", matches[0].Context)
	}
}

func FuzzDetectorDoesNotPanic(f *testing.F) {
	f.Add("欢迎来到重庆，银行行长出差了。", 100)
	f.Add("", 0)
	f.Add(string([]byte{0xff, 0xfe})+"重"+string([]byte{0x80}), -1)
	f.Add(strings.Repeat("重庆", 600), 9999)

	detector := NewDetector([]Entry{
		{ID: "chongqing", Word: "重庆", Type: "place", Risk: "high"},
		{ID: "yinhang", Word: "银行", Type: "word", Risk: "high"},
		{ID: "pattern", Word: "行长", Pattern: "行长", Type: "word", Risk: "high"},
	})
	f.Fuzz(func(t *testing.T, text string, limit int) {
		_, _ = detector.Find(text, limit)
	})
}
