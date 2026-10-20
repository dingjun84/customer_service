package ai

import (
	"fmt"

	"github.com/example/ecommerce-support/internal/domain"
)

// Assembler builds the model context: a fixed system prompt, the active task
// state, the recent complete turns, and nothing else. It never loads the entire
// history and never lets old chat text replace a live business query.
type Assembler struct {
	RecentTurns int
	MaxChars    int
}

// NewAssembler builds an assembler with safe defaults.
func NewAssembler(recentTurns, maxChars int) *Assembler {
	if recentTurns <= 0 {
		recentTurns = 6
	}
	if maxChars <= 0 {
		maxChars = 40000
	}
	return &Assembler{RecentTurns: recentTurns, MaxChars: maxChars}
}

// Build returns the message list for the next model call. The last persisted user
// message must already be part of history (it is the current question).
func (a *Assembler) Build(conv *domain.Conversation, history []domain.Message) []Message {
	out := []Message{{Role: RoleSystem, Content: SystemPrompt}}
	if conv != nil && conv.ActiveTaskStateJSON != "" {
		out = append(out, Message{
			Role:    RoleSystem,
			Content: TaskStateHeader + conv.ActiveTaskStateJSON,
		})
	}
	out = append(out, a.window(history)...)
	return out
}

// window selects the tail of the conversation: at most RecentTurns user turns and
// at most MaxChars characters. Cuts always happen on user-message boundaries so a
// tool call chain is never split.
func (a *Assembler) window(history []domain.Message) []Message {
	filtered := make([]domain.Message, 0, len(history))
	for _, m := range history {
		if m.Role == domain.RoleSystem {
			continue
		}
		filtered = append(filtered, m)
	}
	if len(filtered) == 0 {
		return nil
	}

	start := 0
	turns := 0
	for i := len(filtered) - 1; i >= 0; i-- {
		if filtered[i].Role != domain.RoleUser {
			continue
		}
		turns++
		if turns > a.RecentTurns {
			start = i + 1
			break
		}
	}
	// Never begin with an orphan tool result or assistant fragment.
	for start < len(filtered) && filtered[start].Role != domain.RoleUser {
		start++
	}

	// Enforce the character budget by dropping the oldest complete turn.
	for start < len(filtered)-1 && runeLen(filtered[start:]) > a.MaxChars {
		next := start + 1
		for next < len(filtered) && filtered[next].Role != domain.RoleUser {
			next++
		}
		start = next
	}
	return convert(filtered[start:])
}

func runeLen(msgs []domain.Message) int {
	total := 0
	for _, m := range msgs {
		total += len([]rune(m.Content)) + len([]rune(m.ToolArgumentsJSON)) + len([]rune(m.ToolResultJSON))
	}
	return total
}

// convert maps stored rows onto provider messages, merging the assistant rows that
// belong to one tool-calling turn back into a single assistant message.
func convert(rows []domain.Message) []Message {
	out := make([]Message, 0, len(rows))
	declared := map[string]bool{}

	for _, m := range rows {
		switch m.Role {
		case domain.RoleUser:
			out = append(out, Message{Role: RoleUser, Content: m.Content})
		case domain.RoleAssistant:
			if m.ToolCallID != "" {
				call := ToolCall{ID: m.ToolCallID, Name: m.ToolName, Arguments: defaultArgs(m.ToolArgumentsJSON)}
				declared[call.ID] = true
				if n := len(out); n > 0 && out[n-1].Role == RoleAssistant && len(out[n-1].ToolCalls) > 0 {
					out[n-1].ToolCalls = append(out[n-1].ToolCalls, call)
					if out[n-1].Content == "" {
						out[n-1].Content = m.Content
					}
					continue
				}
				out = append(out, Message{Role: RoleAssistant, Content: m.Content, ToolCalls: []ToolCall{call}})
				continue
			}
			out = append(out, Message{Role: RoleAssistant, Content: m.Content})
		case domain.RoleTool:
			// Drop tool results without a matching call: providers reject them.
			if !declared[m.ToolCallID] {
				continue
			}
			content := m.ToolResultJSON
			if content == "" {
				content = m.Content
			}
			out = append(out, Message{Role: RoleTool, Content: content, ToolCallID: m.ToolCallID, ToolName: m.ToolName})
		}
	}
	return out
}

func defaultArgs(raw string) string {
	if raw == "" {
		return "{}"
	}
	return raw
}

// PromptChars reports the context size, used for logging/cost hints.
func PromptChars(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += len([]rune(m.Content))
		for _, tc := range m.ToolCalls {
			total += len([]rune(tc.Arguments)) + len(tc.Name)
		}
	}
	return total
}

// SummaryLine renders a short context description for logs.
func SummaryLine(msgs []Message) string {
	return fmt.Sprintf("messages=%d chars=%d", len(msgs), PromptChars(msgs))
}
