package ingest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/otterscope/otterscope/internal/model"
)

// Message extraction, best-effort by design: multimodal parts flatten to
// text, tool interactions render inline. The raw batch is always retained
// (#6), so richer extraction later can backfill via Renormalize.

// genAIMessages parses the gen_ai_latest_experimental message format from
// key: an array of {role, parts:[{type,...}]} recorded either as a
// structured attribute or a JSON string.
func genAIMessages(attrs pcommon.Map, key string) []model.Message {
	v, ok := attrs.Get(key)
	if !ok {
		return nil
	}
	var raw any
	if v.Type() == pcommon.ValueTypeStr {
		if err := json.Unmarshal([]byte(v.Str()), &raw); err != nil {
			return nil
		}
	} else {
		raw = v.AsRaw()
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	var msgs []model.Message
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msg := model.Message{Role: str(m["role"])}
		parts, _ := m["parts"].([]any)
		var sb strings.Builder
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			switch str(part["type"]) {
			case "text":
				sb.WriteString(str(part["content"]))
			case "tool_call":
				sb.WriteString(fmt.Sprintf("[tool_call %s(%s)]", str(part["name"]), compact(part["arguments"])))
			case "tool_call_response":
				sb.WriteString(fmt.Sprintf("[tool_result %s]", compact(part["response"])))
			default:
				sb.WriteString(fmt.Sprintf("[%s]", str(part["type"])))
			}
		}
		msg.Content = sb.String()
		msgs = append(msgs, msg)
	}
	return msgs
}

// genAISystemInstructions prepends gen_ai.system_instructions (string or
// parts array) as a system message when present.
func genAISystemInstructions(attrs pcommon.Map) []model.Message {
	v, ok := attrs.Get("gen_ai.system_instructions")
	if !ok {
		return nil
	}
	if v.Type() == pcommon.ValueTypeStr && !strings.HasPrefix(strings.TrimSpace(v.Str()), "[") {
		return []model.Message{{Role: "system", Content: v.Str()}}
	}
	var raw any
	if v.Type() == pcommon.ValueTypeStr {
		if err := json.Unmarshal([]byte(v.Str()), &raw); err != nil {
			return []model.Message{{Role: "system", Content: v.Str()}}
		}
	} else {
		raw = v.AsRaw()
	}
	parts, ok := raw.([]any)
	if !ok {
		return nil
	}
	var sb strings.Builder
	for _, p := range parts {
		if part, ok := p.(map[string]any); ok {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(str(part["content"]))
		}
	}
	if sb.Len() == 0 {
		return nil
	}
	return []model.Message{{Role: "system", Content: sb.String()}}
}

// openInferenceMessages reassembles OpenInference's dot-index-flattened
// messages (llm.input_messages.0.message.role, ...) under prefix.
func openInferenceMessages(attrs pcommon.Map, prefix string) []model.Message {
	type acc struct {
		role, content string
		parts         map[int]string
		toolCalls     map[int]string
	}
	byIdx := map[int]*acc{}
	get := func(i int) *acc {
		if byIdx[i] == nil {
			byIdx[i] = &acc{parts: map[int]string{}, toolCalls: map[int]string{}}
		}
		return byIdx[i]
	}

	attrs.Range(func(k string, v pcommon.Value) bool {
		if !strings.HasPrefix(k, prefix) {
			return true
		}
		rest := strings.TrimPrefix(k, prefix) // e.g. "0.message.role"
		idx, suffix, ok := splitIndex(rest)
		if !ok {
			return true
		}
		a := get(idx)
		switch {
		case suffix == "message.role":
			a.role = v.AsString()
		case suffix == "message.content":
			a.content = v.AsString()
		case strings.HasPrefix(suffix, "message.contents."):
			if j, tail, ok := splitIndex(strings.TrimPrefix(suffix, "message.contents.")); ok && tail == "message_content.text" {
				a.parts[j] = v.AsString()
			}
		case strings.HasPrefix(suffix, "message.tool_calls."):
			tcRest := strings.TrimPrefix(suffix, "message.tool_calls.")
			if j, tail, ok := splitIndex(tcRest); ok {
				switch tail {
				case "tool_call.function.name":
					a.toolCalls[j] = v.AsString() + a.toolCalls[j]
				case "tool_call.function.arguments":
					a.toolCalls[j] += "(" + v.AsString() + ")"
				}
			}
		}
		return true
	})

	idxs := make([]int, 0, len(byIdx))
	for i := range byIdx {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)

	var msgs []model.Message
	for _, i := range idxs {
		a := byIdx[i]
		content := a.content
		if content == "" && len(a.parts) > 0 {
			js := make([]int, 0, len(a.parts))
			for j := range a.parts {
				js = append(js, j)
			}
			sort.Ints(js)
			var sb strings.Builder
			for _, j := range js {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(a.parts[j])
			}
			content = sb.String()
		}
		if len(a.toolCalls) > 0 {
			js := make([]int, 0, len(a.toolCalls))
			for j := range a.toolCalls {
				js = append(js, j)
			}
			sort.Ints(js)
			var sb strings.Builder
			sb.WriteString(content)
			for _, j := range js {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString("[tool_call " + a.toolCalls[j] + "]")
			}
			content = sb.String()
		}
		msgs = append(msgs, model.Message{Role: a.role, Content: content})
	}
	return msgs
}

// vercelPromptMessages parses ai.prompt.messages: a JSON string of
// {role, content} where content is a string or an array of parts.
func vercelPromptMessages(attrs pcommon.Map, key string) []model.Message {
	v, ok := attrs.Get(key)
	if !ok {
		return nil
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(v.AsString()), &items); err != nil {
		return nil
	}
	var msgs []model.Message
	for _, m := range items {
		msg := model.Message{Role: str(m["role"])}
		switch c := m["content"].(type) {
		case string:
			msg.Content = c
		case []any:
			var sb strings.Builder
			for _, p := range c {
				if part, ok := p.(map[string]any); ok {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					if t := str(part["text"]); t != "" {
						sb.WriteString(t)
					} else {
						sb.WriteString("[" + str(part["type"]) + "]")
					}
				}
			}
			msg.Content = sb.String()
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

func splitIndex(s string) (idx int, rest string, ok bool) {
	dot := strings.IndexByte(s, '.')
	if dot <= 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(s[:dot])
	if err != nil {
		return 0, "", false
	}
	return n, s[dot+1:], true
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// compact renders an arbitrary decoded JSON value on one line.
func compact(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
