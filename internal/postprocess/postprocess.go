package postprocess

import (
	"context"
	"errors"
	"strings"
	"unicode"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"ai-transcriber-cli/internal/domain"
)

const defaultModel = "gpt-4o-mini"

type responseClient interface {
	Create(ctx context.Context, model, instructions, input string) (responseResult, error)
}

type responseResult struct {
	Text string
}

type Service struct {
	client       responseClient
	defaultModel string
}

func NewService(apiKey string) *Service {
	svc := &Service{defaultModel: defaultModel}
	if strings.TrimSpace(apiKey) != "" {
		svc.client = openAIResponsesClient{client: openai.NewClient(option.WithAPIKey(apiKey))}
	}
	return svc
}

func NewServiceForTest(client responseClient, model string) *Service {
	if strings.TrimSpace(model) == "" {
		model = defaultModel
	}
	return &Service{client: client, defaultModel: model}
}

func (s *Service) Apply(ctx context.Context, t domain.Transcript, enabled bool, model string, prompt string) (domain.Transcript, error) {
	if !enabled {
		return t, nil
	}
	originalText := t.Text
	t.Text = cleanupText(t.Text)
	segmentChanged := false
	for i := range t.Segments {
		cleaned := cleanupText(t.Segments[i].Text)
		if cleaned != t.Segments[i].Text {
			segmentChanged = true
			t.Segments[i].Text = cleaned
		}
	}
	if t.Text != originalText || segmentChanged {
		t.Warnings = domain.AppendWarning(t.Warnings, domain.Warning{
			Code:    "postprocess_applied",
			Message: "postprocess normalized whitespace and paragraph boundaries",
		})
	}
	if len(t.Words) > 0 && (t.Text != originalText || segmentChanged) {
		t.Warnings = domain.AppendWarning(t.Warnings, domain.Warning{
			Code:    "postprocess_word_alignment_unchecked",
			Message: "postprocess updated transcript text without rewriting word timings",
		})
	}
	selectedModel := strings.TrimSpace(model)
	if selectedModel == "" {
		selectedModel = s.defaultModel
	}
	if s.client == nil {
		t.Warnings = domain.AppendWarning(t.Warnings, domain.Warning{
			Code:    "postprocess_skipped",
			Message: "postprocess client is unavailable; returning deterministically cleaned transcript",
		})
		return t, nil
	}
	rewritten, err := s.client.Create(ctx, selectedModel, buildInstructions(prompt), t.Text)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return t, domain.NewError("postprocess_cancelled", "postprocess request was cancelled", domain.ExitNetwork, err)
		}
		t.Warnings = domain.AppendWarning(t.Warnings, domain.Warning{
			Code:    "postprocess_failed_open",
			Message: "postprocess request failed; using deterministically cleaned transcript",
		})
		return t, nil
	}
	if cleaned := cleanupText(rewritten.Text); cleaned != "" && cleaned != t.Text {
		t.Text = cleaned
		t.Warnings = domain.AppendWarning(t.Warnings, domain.Warning{
			Code:    "postprocess_ai_applied",
			Message: "AI postprocess applied transcript-level correction",
		})
	}
	return t, nil
}

type openAIResponsesClient struct {
	client openai.Client
}

func (c openAIResponsesClient) Create(ctx context.Context, model, instructions, input string) (responseResult, error) {
	resp, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        model,
		Store:        openai.Bool(false),
		Temperature:  openai.Float(0),
		Instructions: openai.String(instructions),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(input),
		},
	})
	if err != nil {
		return responseResult{}, err
	}
	return responseResult{Text: resp.OutputText()}, nil
}

func buildInstructions(prompt string) string {
	base := []string{
		"You are correcting automatic speech recognition output.",
		"Return only the corrected transcript text.",
		"Do not summarize.",
		"Do not translate.",
		"Do not change the language.",
		"Do not add bullet points or headings.",
		"Do not add speaker labels unless they are already present in the transcript.",
		"If unsure, keep the original wording.",
	}
	if extra := strings.TrimSpace(prompt); extra != "" {
		base = append(base, "Project-specific correction guidance:", extra)
	}
	return strings.Join(base, "\n")
}

func cleanupText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	paragraphs := splitParagraphs(text)
	for i, paragraph := range paragraphs {
		paragraphs[i] = normalizeWhitespace(paragraph)
	}
	return strings.TrimSpace(strings.Join(filterEmpty(paragraphs), "\n\n"))
}

func splitParagraphs(text string) []string {
	parts := strings.Split(text, "\n\n")
	if len(parts) == 1 {
		return strings.Split(text, "\n")
	}
	return parts
}

func normalizeWhitespace(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if lastSpace {
				continue
			}
			b.WriteRune(' ')
			lastSpace = true
			continue
		}
		lastSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func filterEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
