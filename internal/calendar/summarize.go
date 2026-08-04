package calendar

import (
	"context"
	"fmt"
	"strings"

	coreai "github.com/aeon022/missionctl-core/ai"
	"github.com/aeon022/calctl/internal/models"
)

const summarizeSystemPrompt = "You are a professional meeting assistant."

// Summarize generates a structured meeting summary for the given event
// using the configured AI provider (Anthropic, OpenAI, Gemini, or a local
// Ollama model — see missionctl-core/ai).
func Summarize(ctx context.Context, event *models.Event) (string, error) {
	var parts []string

	parts = append(parts, fmt.Sprintf("Title: %s", event.Title))
	parts = append(parts, fmt.Sprintf("Date: %s", event.StartTime.Format("Monday, January 2, 2006")))

	if event.AllDay {
		parts = append(parts, "Time: All day")
	} else {
		dur := event.Duration()
		h := int(dur.Hours())
		m := int(dur.Minutes()) % 60
		durStr := ""
		if h > 0 {
			durStr = fmt.Sprintf("%dh", h)
		}
		if m > 0 {
			durStr += fmt.Sprintf("%dm", m)
		}
		parts = append(parts, fmt.Sprintf("Time: %s – %s (%s)",
			event.StartTime.Format("15:04"),
			event.EndTime.Format("15:04"),
			durStr,
		))
	}

	if event.Calendar != "" {
		parts = append(parts, fmt.Sprintf("Calendar: %s", event.Calendar))
	}
	if event.Location != "" && event.Location != "missing value" {
		parts = append(parts, fmt.Sprintf("Location: %s", event.Location))
	}
	if len(event.Attendees) > 0 {
		parts = append(parts, fmt.Sprintf("Attendees: %s", strings.Join(event.Attendees, ", ")))
	}
	if event.Notes != "" {
		parts = append(parts, fmt.Sprintf("Notes:\n%s", event.Notes))
	}

	eventInfo := strings.Join(parts, "\n")

	prompt := fmt.Sprintf(`Based on the calendar event details below, generate a structured meeting summary.

%s

Write a structured meeting summary with these sections:
1. **Overview** — 1–2 sentences describing the meeting purpose and outcome
2. **Key Decisions** — bullet list (or "None recorded" if unknown)
3. **Action Items** — bullet list with owner if determinable (or "None recorded")
4. **Next Steps** — what happens after this meeting

Keep it concise and professional. If specific decisions or action items are not available from the event data, note that they should be filled in after the meeting.`, eventInfo)

	info, err := coreai.Detect("CALCTL")
	if err != nil {
		return "", err
	}
	text, err := coreai.Call(ctx, info, summarizeSystemPrompt, prompt, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
