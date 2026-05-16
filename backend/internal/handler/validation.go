package handler

import (
	"fmt"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
)

// maxLen returns nil if value fits within maxRunes UTF-8 code points, or a
// ready-to-return 400 response otherwise.
//
// Without this check, a long string lands at the DB layer and PostgreSQL
// silently truncates a VARCHAR(N) column to N characters — the client gets
// a 201 with a value shorter than what it sent. Symptoms range from
// confusing ("my note got cut off") to dangerous (Currency "RUBLE" truncates
// to "RUB", money in the wrong currency).
//
// Caps must mirror the `gorm:"size:N"` tags in internal/model/models.go.
// Rune count, not byte count: PG VARCHAR(N) measures characters, so
// Cyrillic strings (2 bytes/char) must be allowed up to N runes.
func maxLen(c fiber.Ctx, field, value string, maxRunes int) error {
	if utf8.RuneCountInString(value) > maxRunes {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("%s too long (max %d characters)", field, maxRunes),
		})
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
