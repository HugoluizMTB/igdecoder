package igdecoder

import (
	"encoding/json"
	"strconv"
	"time"
)

type jmap = map[string]any

func asMap(v any) (jmap, bool) {
	m, ok := v.(jmap)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func dig(root any, path ...string) any {
	cur := root
	for _, k := range path {
		m, ok := asMap(cur)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func firstKey(m jmap, keys ...string) any {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			if x == "" {
				continue
			}
		case []any:
			if len(x) == 0 {
				continue
			}
		}
		return v
	}
	return nil
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:

		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	case json.Number:
		n, _ := x.Int64()
		return n
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1"
	case float64:
		return x != 0
	default:
		return false
	}
}

func digStr(root any, path ...string) string    { return toString(dig(root, path...)) }
func digInt(root any, path ...string) int64     { return toInt64(dig(root, path...)) }
func digFloat(root any, path ...string) float64 { return toFloat(dig(root, path...)) }

func epochToTime(v any) time.Time {
	n := toInt64(v)
	if n <= 0 {
		return time.Time{}
	}
	if n > 1e12 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

func walkMaps(root any, visit func(jmap) bool) {
	type frame struct {
		v     any
		depth int
	}
	stack := []frame{{root, 0}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.depth > 40 {
			continue
		}
		switch x := f.v.(type) {
		case jmap:
			if !visit(x) {
				return
			}
			for _, v := range x {
				stack = append(stack, frame{v, f.depth + 1})
			}
		case []any:
			for _, v := range x {
				stack = append(stack, frame{v, f.depth + 1})
			}
		}
	}
}
