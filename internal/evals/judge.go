package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

// JudgeConfig is the parsed config of an llm_judge assertion. Endpoint and
// API key are deliberately NOT part of it: assertions are creatable via the
// unauthenticated single-user API, so letting them name an env var or URL
// would allow secret exfiltration and SSRF (audit finding #48). The endpoint
// is server configuration — see Endpoint.
type JudgeConfig struct {
	Prompt     string  `json:"prompt"`
	Model      string  `json:"model"`
	SampleRate float64 `json:"sampleRate,omitempty"` // 0..1; 0 means 1.0 (all)
}

// Endpoint is the server-configured judge target: base URL from the
// -judge-url flag, key resolved once at startup from OTTERSCOPE_JUDGE_KEY
// (fallback OPENAI_API_KEY).
type Endpoint struct {
	BaseURL string
	Key     string
}

func parseJudgeConfig(raw string) (JudgeConfig, error) {
	var c JudgeConfig
	dec := json.NewDecoder(strings.NewReader(raw))
	// Unknown fields (e.g. legacy baseUrl/apiKeyEnv) must fail loudly, not
	// be silently ignored.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("llm_judge config invalid (only prompt, model, sampleRate are allowed): %w", err)
	}
	if c.Prompt == "" || c.Model == "" {
		return c, fmt.Errorf("llm_judge config needs prompt and model")
	}
	if c.SampleRate < 0 || c.SampleRate > 1 {
		return c, fmt.Errorf("sampleRate must be within 0..1")
	}
	return c, nil
}

// judgeHTTP is swapped in tests.
var judgeHTTP = &http.Client{Timeout: 60 * time.Second}

// Judge scores a run with an LLM via the server-configured OpenAI-compatible
// endpoint. The verdict must start with PASS or FAIL; the rest becomes the
// detail.
func Judge(ctx context.Context, ep Endpoint, a Assertion, run model.Run, steps []model.Step) Result {
	res := Result{AssertionID: a.ID, Name: a.Name, Type: a.Type}
	cfg, err := parseJudgeConfig(a.Config)
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	if ep.Key == "" {
		res.Detail = "judge skipped: OTTERSCOPE_JUDGE_KEY (or OPENAI_API_KEY) is not set"
		return res
	}

	input := firstUserInput(steps)
	output := FinalOutput(steps)
	body, _ := json.Marshal(map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": cfg.Prompt +
				"\n\nRespond with a single line starting with PASS or FAIL, followed by a brief reason."},
			{"role": "user", "content": fmt.Sprintf("Agent input:\n%s\n\nAgent output:\n%s", input, output)},
		},
		"max_tokens": 300,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(ep.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ep.Key)

	resp, err := judgeHTTP.Do(req)
	if err != nil {
		res.Detail = fmt.Sprintf("judge call failed: %v", err)
		return res
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		res.Detail = fmt.Sprintf("judge endpoint %d: %.200s", resp.StatusCode, raw)
		return res
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		res.Detail = "judge returned an unparseable response"
		return res
	}
	verdict := strings.TrimSpace(parsed.Choices[0].Message.Content)
	res.Detail = verdict
	res.Pass = strings.HasPrefix(strings.ToUpper(verdict), "PASS")
	return res
}

// firstUserInput returns the first LLM step's input messages, for judge
// context.
func firstUserInput(steps []model.Step) string {
	for _, st := range steps {
		if st.LLM == nil || len(st.LLM.InputMessages) == 0 {
			continue
		}
		var sb strings.Builder
		for _, m := range st.LLM.InputMessages {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(m.Role + ": " + m.Content)
		}
		return sb.String()
	}
	return "(no input recorded)"
}
