package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// DecodeArgs decodes a tool-call argument payload into a validated-friendly map.
// Numbers are kept as json.Number so integer bounds can be checked exactly.
func DecodeArgs(raw json.RawMessage) (Args, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return Args{}, nil
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("工具参数必须是 JSON 对象")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("工具参数不是合法 JSON")
	}
	if dec.More() {
		return nil, fmt.Errorf("工具参数包含多余内容")
	}
	if out == nil {
		out = map[string]any{}
	}
	return Args(out), nil
}

// Validate checks decoded arguments against a JSON-Schema subset: object/string/
// integer/number/boolean/array, required, enum, min/max, minLength/maxLength and
// additionalProperties=false. It is intentionally strict: unknown fields are
// rejected so a model cannot smuggle extra parameters into business services.
func Validate(schema map[string]any, value any) error {
	return validate(schema, value, "")
}

func validate(schema map[string]any, value any, path string) error {
	if schema == nil {
		return nil
	}
	want, _ := schema["type"].(string)
	switch want {
	case "object":
		return validateObject(schema, value, path)
	case "array":
		items, ok := value.([]any)
		if !ok {
			return typeError(path, "数组")
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for i, item := range items {
			if err := validate(itemSchema, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case "string":
		s, ok := value.(string)
		if !ok {
			return typeError(path, "字符串")
		}
		if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
			matched := false
			for _, e := range enum {
				if es, ok := e.(string); ok && es == s {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("参数 %s 取值 %q 不在允许范围内", labelOf(path), s)
			}
		}
		if maxLen, ok := numberOf(schema["maxLength"]); ok && int64(len([]rune(s))) > maxLen {
			return fmt.Errorf("参数 %s 超过最大长度 %d", labelOf(path), maxLen)
		}
		if minLen, ok := numberOf(schema["minLength"]); ok && int64(len([]rune(s))) < minLen {
			return fmt.Errorf("参数 %s 长度不足 %d", labelOf(path), minLen)
		}
		return nil
	case "integer":
		n, ok := numberOf(value)
		if !ok {
			return typeError(path, "整数")
		}
		if min, ok := numberOf(schema["minimum"]); ok && n < min {
			return fmt.Errorf("参数 %s 不能小于 %d", labelOf(path), min)
		}
		if max, ok := numberOf(schema["maximum"]); ok && n > max {
			return fmt.Errorf("参数 %s 不能大于 %d", labelOf(path), max)
		}
		return nil
	case "number":
		if !isNumber(value) {
			return typeError(path, "数字")
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return typeError(path, "布尔值")
		}
		return nil
	default:
		return nil
	}
}

func validateObject(schema map[string]any, value any, path string) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return typeError(path, "对象")
	}
	props := map[string]any{}
	if raw, ok := schema["properties"].(map[string]any); ok {
		props = raw
	}
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			name, _ := r.(string)
			if name == "" {
				continue
			}
			if _, present := obj[name]; !present {
				return fmt.Errorf("缺少必填参数 %s", joinPath(path, name))
			}
		}
	}
	allowAdditional := true
	if v, ok := schema["additionalProperties"].(bool); ok {
		allowAdditional = v
	}
	for key, raw := range obj {
		sub, known := props[key]
		if !known {
			if !allowAdditional {
				return fmt.Errorf("参数 %s 不是允许的字段", joinPath(path, key))
			}
			continue
		}
		subSchema, _ := sub.(map[string]any)
		if err := validate(subSchema, raw, joinPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func labelOf(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

func typeError(path, want string) error {
	return fmt.Errorf("参数 %s 必须是%s", labelOf(path), want)
}

func isNumber(v any) bool {
	switch v.(type) {
	case json.Number, float64, int, int64:
		return true
	default:
		return false
	}
}

func numberOf(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, ferr := n.Float64()
			if ferr != nil {
				return 0, false
			}
			return int64(f), true
		}
		return i, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	default:
		return 0, false
	}
}

// TruncateJSON keeps tool trace payloads bounded.
func TruncateJSON(raw []byte, max int) string {
	if max <= 0 || len(raw) <= max {
		return string(raw)
	}
	return strings.ToValidUTF8(string(raw[:max]), "") + "…(truncated)"
}
