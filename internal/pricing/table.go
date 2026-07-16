package pricing

// defaultRates: USD per million tokens, keyed by model-ID prefix
// (longest-prefix match, case-insensitive). Verified against official
// provider pricing pages on 2026-07-16. Update deliberately; keep the
// as-of note current. Tiered prices use the base tier (long-context
// premiums >200k prompt tokens on Gemini Pro / Grok are NOT modeled).
// CacheWrite uses the provider's default (5-minute) tier where tiered.
var defaultRates = map[string]Rate{
	// Anthropic — platform.claude.com/docs/en/about-claude/pricing
	"claude-fable-5":  {Input: 10, Output: 50, CacheRead: 1, CacheWrite: 12.50},
	"claude-mythos-5": {Input: 10, Output: 50, CacheRead: 1, CacheWrite: 12.50},
	"claude-opus-4":   {Input: 5, Output: 25, CacheRead: 0.50, CacheWrite: 6.25}, // 4-5 through 4-8 priced identically
	// Introductory pricing through 2026-08-31; standard becomes 3/15
	// (cacheRead 0.30, cacheWrite 3.75) on 2026-09-01 — update then.
	"claude-sonnet-5":   {Input: 2, Output: 10, CacheRead: 0.20, CacheWrite: 2.50},
	"claude-sonnet-4-6": {Input: 3, Output: 15, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-sonnet-4-5": {Input: 3, Output: 15, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-haiku-4-5":  {Input: 1, Output: 5, CacheRead: 0.10, CacheWrite: 1.25},

	// OpenAI — developers.openai.com/api/docs/pricing
	// (cache-write pricing unverified; reads-only discount modeled)
	"gpt-5.6-sol":   {Input: 5, Output: 30, CacheRead: 0.50},
	"gpt-5.6-terra": {Input: 2.50, Output: 15, CacheRead: 0.25},
	"gpt-5.6-luna":  {Input: 1, Output: 6, CacheRead: 0.10},
	"gpt-5.5-pro":   {Input: 30, Output: 180},
	"gpt-5.5":       {Input: 5, Output: 30, CacheRead: 0.50},
	"gpt-5.4-pro":   {Input: 30, Output: 180},
	"gpt-5.4-mini":  {Input: 0.75, Output: 4.50, CacheRead: 0.075},
	"gpt-5.4-nano":  {Input: 0.20, Output: 1.25, CacheRead: 0.02},
	"gpt-5.4":       {Input: 2.50, Output: 15, CacheRead: 0.25},
	"gpt-5.3-codex": {Input: 1.75, Output: 14, CacheRead: 0.175},

	// Google — ai.google.dev/gemini-api/docs/pricing (base ≤200k tier;
	// per-hour cache storage not modeled)
	"gemini-3.5-flash":      {Input: 1.50, Output: 9, CacheRead: 0.15},
	"gemini-3.1-pro":        {Input: 2, Output: 12, CacheRead: 0.20},
	"gemini-3.1-flash-lite": {Input: 0.25, Output: 1.50, CacheRead: 0.025},
	"gemini-3-flash":        {Input: 0.50, Output: 3, CacheRead: 0.05},
	"gemini-2.5-pro":        {Input: 1.25, Output: 10, CacheRead: 0.125},
	"gemini-2.5-flash-lite": {Input: 0.10, Output: 0.40, CacheRead: 0.01},
	"gemini-2.5-flash":      {Input: 0.30, Output: 2.50, CacheRead: 0.03},

	// DeepSeek — api-docs.deepseek.com (deepseek-chat/reasoner are legacy
	// aliases of v4-flash, deprecated 2026-07-24)
	"deepseek-v4-flash": {Input: 0.14, Output: 0.28, CacheRead: 0.0028},
	"deepseek-v4-pro":   {Input: 0.435, Output: 0.87, CacheRead: 0.003625},
	"deepseek-chat":     {Input: 0.14, Output: 0.28, CacheRead: 0.0028},
	"deepseek-reasoner": {Input: 0.14, Output: 0.28, CacheRead: 0.0028},

	// Mistral — mistral.ai/pricing/api
	"mistral-medium":   {Input: 1.50, Output: 7.50},
	"mistral-large":    {Input: 0.50, Output: 1.50},
	"mistral-small":    {Input: 0.15, Output: 0.60},
	"magistral-medium": {Input: 2, Output: 5},
	"codestral":        {Input: 0.30, Output: 0.90},
	"devstral-medium":  {Input: 0.40, Output: 2},

	// xAI — docs.x.ai/docs/models (base ≤200k tier)
	"grok-4.5": {Input: 2, Output: 6, CacheRead: 0.50},
	"grok-4.3": {Input: 1.25, Output: 2.50, CacheRead: 0.20},
	"grok-4":   {Input: 1.25, Output: 2.50, CacheRead: 0.20},

	// Meta Llama via Groq — groq.com/pricing
	"llama-3.3-70b-versatile": {Input: 0.59, Output: 0.79},
	"llama-3.1-8b-instant":    {Input: 0.05, Output: 0.08},
	"openai/gpt-oss-120b":     {Input: 0.15, Output: 0.60},
}
