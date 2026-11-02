package glossary

import "testing"

func TestNormalizePatternConvertsLookahead(t *testing.T) {
	pattern, suffixes := NormalizePattern("曾", "曾(?=先生|女士|老师)")
	if pattern != "" {
		t.Fatalf("expected empty pattern, got %q", pattern)
	}
	if len(suffixes) != 3 || suffixes[0] != "先生" || suffixes[2] != "老师" {
		t.Fatalf("unexpected suffixes: %#v", suffixes)
	}
}

func TestDetectorMatchesWordsAndSurnameContext(t *testing.T) {
	entries := []Entry{
		{ID: "chongqing", Word: "重庆", Type: "place", Risk: "high", Default: "chong2 qing4"},
		{ID: "zeng", Word: "曾", Type: "surname", Risk: "high", Default: "zeng1", ContextAfter: []string{"先生", "女士"}},
	}
	detector := NewDetector(entries)
	matches, truncated := detector.Find("欢迎来到重庆。曾先生来了，曾经也来过。", 100)
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %#v", len(matches), matches)
	}
	if matches[0].Text != "重庆" || matches[1].Text != "曾" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
	if matches[1].Context == "" {
		t.Fatal("expected context")
	}
}

func TestDetectorDeduplicatesOverlaps(t *testing.T) {
	entries := []Entry{
		{ID: "chongqing", Word: "重庆", Type: "place", Risk: "high"},
		{ID: "chong", Word: "重", Type: "word", Risk: "medium"},
	}
	detector := NewDetector(entries)
	matches, _ := detector.Find("重庆", 100)
	if len(matches) != 1 || matches[0].Text != "重庆" {
		t.Fatalf("expected the longer match only, got %#v", matches)
	}
}

func TestDetectorHonorsLimit(t *testing.T) {
	entries := []Entry{{ID: "chongqing", Word: "重庆", Type: "place", Risk: "high"}}
	detector := NewDetector(entries)
	matches, truncated := detector.Find("重庆重庆重庆", 2)
	if !truncated || len(matches) != 2 {
		t.Fatalf("expected 2 truncated matches, got %d (truncated=%v)", len(matches), truncated)
	}
}
