package handler

import (
	"fmt"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
)

// maxLen returns nil when value fits within maxRunes UTF-8 code points,
// or a *fiber.Error (400) that flows through the globalErrorHandler in
// cmd/server/main.go.
//
// Subtle but important: the previous implementation called
// c.Status(400).JSON(...) directly and returned the result. Fiber's
// JSON returns nil on a successful write, so callers checking
// `if err := maxLen(...); err != nil { return err }` saw nil and
// kept executing the handler — the response body got overwritten
// later by the DB layer's 500. The fix is to return a sentinel error
// (fiber.NewError) and let globalErrorHandler do the response write.
//
// Caps must mirror the `gorm:"size:N"` tags in internal/model/models.go.
// Rune count, not byte count: PG VARCHAR(N) measures characters, so
// Cyrillic strings (2 bytes/char) must be allowed up to N runes.
//
// The fiber.Ctx parameter is kept on the signature for source-level
// backwards-compat with the dozens of call sites in subscription/room/
// user handlers; the helper itself no longer uses it.
func maxLen(_ fiber.Ctx, field, value string, maxRunes int) error {
	if utf8.RuneCountInString(value) > maxRunes {
		return fiber.NewError(
			fiber.StatusBadRequest,
			fmt.Sprintf("%s too long (max %d characters)", field, maxRunes),
		)
	}
	return nil
}

// maxLenPtr is the *string variant for PATCH-style optional fields: a nil
// pointer means "field not present in the body" and is allowed.
func maxLenPtr(c fiber.Ctx, field string, value *string, maxRunes int) error {
	if value == nil {
		return nil
	}
	return maxLen(c, field, *value, maxRunes)
}
