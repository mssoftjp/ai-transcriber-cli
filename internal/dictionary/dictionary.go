package dictionary

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"ai-transcriber-cli/internal/domain"
)

type Rule struct {
	From            []string `yaml:"from"`
	To              string   `yaml:"to"`
	ContextKeywords []string `yaml:"context_keywords"`
	Category        string   `yaml:"category"`
	Priority        int      `yaml:"priority"`
}

type LanguageRules struct {
	Definite   []Rule `yaml:"definite_corrections"`
	Contextual []Rule `yaml:"contextual_corrections"`
}

type Dictionary map[string]LanguageRules

func Load(path string) (Dictionary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var dict Dictionary
	if err := yaml.Unmarshal(data, &dict); err != nil {
		return nil, err
	}
	return dict, nil
}

func Apply(t domain.Transcript, dict Dictionary) domain.Transcript {
	rules, ok := dict[t.Language]
	if !ok {
		return t
	}
	sortRules(rules.Definite)
	sortRules(rules.Contextual)
	for _, rule := range rules.Definite {
		t = applyRule(t, rule)
	}
	for _, rule := range rules.Contextual {
		if matchesContext(t.Text, rule.ContextKeywords) {
			t = applyRule(t, rule)
		}
	}
	return t
}

func PromptHints(dict Dictionary, language string, maxRules int) string {
	rules, ok := dict[language]
	if !ok || maxRules <= 0 {
		return ""
	}
	lines := make([]string, 0, maxRules)
	sortRules(rules.Definite)
	sortRules(rules.Contextual)
	for _, rule := range rules.Definite {
		if len(lines) >= maxRules || len(rule.From) == 0 {
			break
		}
		lines = append(lines, fmt.Sprintf("Prefer %q over %q.", rule.To, rule.From[0]))
	}
	for _, rule := range rules.Contextual {
		if len(lines) >= maxRules || len(rule.From) == 0 {
			break
		}
		if len(rule.ContextKeywords) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("In contexts related to %s, prefer %q over %q.", strings.Join(rule.ContextKeywords, ", "), rule.To, rule.From[0]))
	}
	return strings.Join(lines, "\n")
}

func sortRules(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority > rules[j].Priority })
}

func matchesContext(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			return true
		}
	}
	return len(keywords) == 0
}

func applyRule(t domain.Transcript, rule Rule) domain.Transcript {
	replaced := false
	for _, from := range rule.From {
		updated := strings.ReplaceAll(t.Text, from, rule.To)
		if updated != t.Text {
			replaced = true
			t.Text = updated
		}
		for i := range t.Segments {
			segmentUpdated := strings.ReplaceAll(t.Segments[i].Text, from, rule.To)
			if segmentUpdated != t.Segments[i].Text {
				replaced = true
				t.Segments[i].Text = segmentUpdated
			}
		}
	}
	if replaced && len(t.Words) > 0 {
		t.Warnings = domain.AppendWarning(t.Warnings, domain.Warning{
			Code:    "word_alignment_out_of_sync",
			Message: "dictionary replacements changed transcript text without updating word timings",
		})
	}
	return t
}
