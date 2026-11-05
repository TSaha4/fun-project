package desktop

import (
	"errors"
	"net/url"
	"strings"
	"sync/atomic"

	"example.com/local-capture/internal/domain"
)

type submissionGate struct{ busy atomic.Bool }

func (g *submissionGate) Acquire() bool { return g.busy.CompareAndSwap(false, true) }
func (g *submissionGate) Release()      { g.busy.Store(false) }
func (g *submissionGate) IsBusy() bool  { return g.busy.Load() }

func NoteTypeForIndex(index int) domain.NoteType {
	types := []domain.NoteType{domain.NoteTypeThought, domain.NoteTypeIdea, domain.NoteTypeTask, domain.NoteTypeLink, domain.NoteTypeImage, domain.NoteTypeQuote, domain.NoteTypeCode, domain.NoteTypeReference}
	if index < 0 || index >= len(types) {
		return domain.NoteTypeThought
	}
	return types[index]
}

func SuggestedTitle(content string) string {
	line := strings.TrimSpace(strings.SplitN(content, "\n", 2)[0])
	if len([]rune(line)) > 80 {
		line = string([]rune(line)[:80])
	}
	if line == "" {
		return "Quick capture"
	}
	return line
}

func ValidateDashboardURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("dashboard URL must be an absolute HTTP(S) URL")
	}
	return nil
}
