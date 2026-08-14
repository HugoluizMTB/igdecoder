package igdecoder

import "testing"

func TestToInt64(t *testing.T) {
	if toInt64(float64(42)) != 42 {
		t.Error("float64")
	}
	if toInt64("123") != 123 {
		t.Error("string")
	}
	if toInt64(nil) != 0 {
		t.Error("nil")
	}
	if toInt64(true) != 0 {
		t.Error("bool")
	}
}

func TestToStringFromFloatKeepsInteger(t *testing.T) {
	if got := toString(float64(456)); got != "456" {
		t.Errorf("got %q", got)
	}
	if got := toString("abc"); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestToBool(t *testing.T) {
	if !toBool(true) || !toBool("true") || !toBool(float64(1)) {
		t.Error("valores verdadeiros")
	}
	if toBool("false") || toBool(nil) || toBool(float64(0)) {
		t.Error("valores falsos")
	}
}

func TestEpochToTime(t *testing.T) {
	sec := epochToTime(float64(1700000000))
	if sec.IsZero() || sec.Year() != 2023 {
		t.Errorf("segundos: %v", sec)
	}
	ms := epochToTime(float64(1700000000000))
	if ms.Year() != 2023 {
		t.Errorf("ms: %v", ms)
	}
	if !epochToTime(0).IsZero() || !epochToTime(nil).IsZero() {
		t.Error("epoch invalido deveria ser zero")
	}
}

func TestDig(t *testing.T) {
	root := map[string]any{"a": map[string]any{"b": "c"}}
	if digStr(root, "a", "b") != "c" {
		t.Error("dig presente")
	}
	if dig(root, "a", "x") != nil {
		t.Error("dig ausente")
	}
	if dig(root, "z", "y") != nil {
		t.Error("dig caminho inexistente")
	}
}

func TestNextCursor(t *testing.T) {
	pi := map[string]any{"paging_info": map[string]any{"more_available": true, "max_id": "NEXT"}}
	if nextCursor(pi) != "NEXT" {
		t.Error("paging_info")
	}
	nm := map[string]any{"next_max_id": "N2"}
	if nextCursor(nm) != "N2" {
		t.Error("next_max_id")
	}
	done := map[string]any{"paging_info": map[string]any{"more_available": false}}
	if nextCursor(done) != "" {
		t.Error("sem próxima página")
	}
}

func TestFirstKey(t *testing.T) {
	m := map[string]any{"b": "", "c": "achou"}
	if firstStr(m, "a", "b", "c") != "achou" {
		t.Error("firstKey deveria pular vazios")
	}
}
