package browser

import (
	"testing"
	"time"
)

func TestFindSnapshotReturnsRefsAndCompactExcerpts(t *testing.T) {
	snapshot := Snapshot{
		URL:        "https://example.com/news",
		Title:      "News",
		Text:       "Primeira notícia sobre economia. Outra notícia sobre política.",
		Backend:    "chromium",
		Generation: 7,
		Elements: []Element{
			{Ref: "g7e1", Role: "link", Name: "Notícia de economia"},
		},
	}
	result, err := FindSnapshot(snapshot, "economia", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 || result.Matches[0].Ref != "g7e1" || result.Generation != 7 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestNormalizeWaitCondition(t *testing.T) {
	condition, err := NormalizeWaitCondition(WaitCondition{Text: "ready"})
	if err != nil || condition.Timeout != 10*time.Second {
		t.Fatalf("condition=%+v err=%v", condition, err)
	}
	if _, err := NormalizeWaitCondition(WaitCondition{Text: "ready", URLContains: "/done"}); err == nil {
		t.Fatal("expected mutually exclusive wait condition error")
	}
	if _, err := NormalizeWaitCondition(WaitCondition{Text: "ready", Timeout: 31 * time.Second}); err == nil {
		t.Fatal("expected timeout bound error")
	}
}
