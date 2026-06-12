package ai

import (
	"strings"
	"testing"
)

func TestExtractJSONPlain(t *testing.T) {
	got := ExtractJSON(`{"a": 1}`)
	if got != `{"a": 1}` {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSONCodeFence(t *testing.T) {
	in := "好的，这是食谱：\n```json\n{\"recipes\": []}\n```\n希望有帮助"
	if got := ExtractJSON(in); got != `{"recipes": []}` {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSONBareFence(t *testing.T) {
	in := "```\n{\"a\": {\"b\": 2}}\n```"
	if got := ExtractJSON(in); got != `{"a": {"b": 2}}` {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSONSurroundingProse(t *testing.T) {
	in := `输出如下 {"x": [1,2,{"y":3}]} 完毕`
	if got := ExtractJSON(in); got != `{"x": [1,2,{"y":3}]}` {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSONNone(t *testing.T) {
	if got := ExtractJSON("抱歉，我无法生成。"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestGenPromptMentionsContract(t *testing.T) {
	// Guard against accidental prompt regressions that would break
	// the parse step: the template must demand the seedjson shape.
	for _, must := range []string{"canonical_name", "baby_split_point", "适量", "recipes"} {
		if !strings.Contains(genPromptTemplate+genSystem, must) {
			t.Fatalf("prompt no longer mentions %q", must)
		}
	}
}
