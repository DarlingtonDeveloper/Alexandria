package semantic

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/warrentherabbit/alexandria/internal/store"
)

// EntityText builds embeddable text from a CGEntity's fields.
func EntityText(e *store.CGEntity) string {
	var parts []string
	parts = append(parts, e.Type+": "+e.DisplayName)
	if e.Summary != "" {
		parts = append(parts, e.Summary)
	}
	if len(e.Metadata) > 0 {
		var m map[string]any
		if json.Unmarshal(e.Metadata, &m) == nil {
			for k, v := range m {
				if s, ok := v.(string); ok && s != "" {
					parts = append(parts, k+": "+s)
				}
			}
		}
	}
	return strings.Join(parts, ". ")
}

// TextHash returns a SHA-256 hex digest for change detection.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}
