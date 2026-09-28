package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// ChatConfig configures an OpenAI-compatible chat completions endpoint.
// Azure OpenAI uses the same client with a different URL and api-key header.
type ChatConfig struct {
	Name         string
	BaseURL      string
	APIKey       string
	Model        string
	Temperature  float64
	MaxTokens    int
	APIVersion   string
	Azure        bool
	SystemPrompt string
	HTTP         *http.Client
}

// ChatProvider calls a chat completions API and parses the strict JSON schema.
type ChatProvider struct {
	cfg ChatConfig
}

func NewChatProvider(cfg ChatConfig) *ChatProvider {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 1200
	}
	if cfg.Name == "" {
		if cfg.Azure {
			cfg.Name = "azure-openai"
		} else {
			cfg.Name = "openai-compatible"
		}
	}
	return &ChatProvider{cfg: cfg}
}

func (p *ChatProvider) Name() string { return p.cfg.Name }

func (p *ChatProvider) Analyze(ctx context.Context, pack domain.EvidencePack) (domain.Analysis, error) {
	if p.cfg.BaseURL == "" || p.cfg.Model == "" {
		return domain.Analysis{}, fmt.Errorf("%w: base URL and model are required", ErrUnavailable)
	}
	payload, err := json.Marshal(pack)
	if err != nil {
		return domain.Analysis{}, err
	}
	reqBody := map[string]any{
		"model":       p.cfg.Model,
		"temperature": p.cfg.Temperature,
		"max_tokens":  p.cfg.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": p.prompt()},
			{"role": "user", "content": string(payload)},
		},
	}
	if !p.cfg.Azure {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return domain.Analysis{}, err
	}
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.cfg.Azure {
		endpoint = endpoint + "/openai/deployments/" + p.cfg.Model + "/chat/completions?api-version=" + or(p.cfg.APIVersion, "2024-10-21")
	} else {
		endpoint = endpoint + "/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return domain.Analysis{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		if p.cfg.Azure {
			req.Header.Set("api-key", p.cfg.APIKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		}
	}
	resp, err := p.cfg.HTTP.Do(req)
	if err != nil {
		return domain.Analysis{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return domain.Analysis{}, err
	}
	if resp.StatusCode >= 300 {
		return domain.Analysis{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return domain.Analysis{}, fmt.Errorf("decode provider response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return domain.Analysis{}, fmt.Errorf("%w: empty completion", ErrUnavailable)
	}
	var analysis domain.Analysis
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), &analysis); err != nil {
		return domain.Analysis{}, fmt.Errorf("provider JSON is not the RCA schema: %w", err)
	}
	if err := ValidateRequiredFields(analysis); err != nil {
		return domain.Analysis{}, err
	}
	analysis.AIGenerated = true
	return analysis, nil
}

func (p *ChatProvider) prompt() string {
	if strings.TrimSpace(p.cfg.SystemPrompt) != "" {
		return p.cfg.SystemPrompt
	}
	return SystemPrompt()
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// SystemPrompt is the versioned instruction sent with every analysis.
// The file under prompts/ is the source used by the platform process when present.
func SystemPrompt() string {
	return `You are an SRE assistant. You receive a JSON evidence pack and must return only JSON with this schema:
{"summary":"","impact":"","timeline":[],"evidence":[],"hypotheses":[{"id":"","statement":"","grade":"","evidence_ids":[],"ai_generated":true}],"confidence":0,"confidence_label":"","recommended_actions":[],"rollback_recommended":false,"missing_evidence":[]}
Rules:
- Use only facts present in the evidence pack. Do not invent traces, metrics, logs, timestamps, or customer impact.
- confidence_label must be one of: confirmed, strongly_correlated, probable, possible, insufficient_evidence.
- Use confirmed only when the pack already contains evidence graded confirmed.
- A deployment near an error spike is correlation, never proof.
- Every hypothesis must cite evidence_ids that exist in the pack.
- Recommend human verification. Do not claim you changed production.
- Do not repeat secrets. If a field looks redacted, leave it redacted.
- If evidence is missing, list it in missing_evidence.`
}
