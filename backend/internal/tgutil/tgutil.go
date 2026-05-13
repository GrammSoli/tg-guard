// Package tgutil provides shared Telegram-related utilities used by both
// the bot package and the notification worker. Centralising these constants
// and helpers eliminates the drift risk that comes with duplicating them
// across packages.
package tgutil

import "strings"

// Callback-data prefixes written into the inline keyboard of every reminder
// and consumed by the bot's callback router. Format: "<prefix><uuid>",
// 46-47 bytes — well under Telegram's 64-byte limit.
const (
	RenewCallbackPrefix  = "renew_sub_"
	CancelCallbackPrefix = "cancel_sub_"
)

// AllowedCampaignFields is the set of column names that IncrementCampaign
// may safely interpolate into SQL. Any value not in this set MUST be
// rejected to prevent SQL injection.
var AllowedCampaignFields = map[string]bool{
	"clicks":     true,
	"bot_starts": true,
	"auths":      true,
}

// MarkdownReplacer escapes the four characters that Telegram's legacy
// Markdown parse mode interprets: * _ ` [
var MarkdownReplacer = strings.NewReplacer(
	"*", `\*`,
	"_", `\_`,
	"`", "\\`",
	"[", `\[`,
)

// EscapeMarkdown escapes special characters for Telegram legacy Markdown.
func EscapeMarkdown(s string) string {
	return MarkdownReplacer.Replace(s)
}
