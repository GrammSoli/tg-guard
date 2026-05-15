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

// MarkdownReplacer escapes the four characters that Telegram's LEGACY
// Markdown parse mode interprets: * _ ` [
// Use only when sending with ParseMode: "Markdown". For "MarkdownV2"
// (the modern, strictly-parsed mode) use MarkdownV2Replacer instead —
// it covers a wider character set, and using the wrong escaper for the
// wrong mode breaks rendering.
var MarkdownReplacer = strings.NewReplacer(
	"*", `\*`,
	"_", `\_`,
	"`", "\\`",
	"[", `\[`,
)

// EscapeMarkdown escapes special characters for Telegram LEGACY Markdown.
// Matches the project's current ParseMode: "Markdown" call sites.
func EscapeMarkdown(s string) string {
	return MarkdownReplacer.Replace(s)
}

// MarkdownV2Replacer escapes the full set of characters Telegram's
// MarkdownV2 parser treats as special. From the Bot API spec:
//
//	'_', '*', '[', ']', '(', ')', '~', '`', '>', '#',
//	'+', '-', '=', '|', '{', '}', '.', '!'
//
// MarkdownV2 is stricter than legacy: ANY unescaped special char in a
// non-formatting position throws "can't parse entities". This list is
// the canonical set per Telegram docs.
var MarkdownV2Replacer = strings.NewReplacer(
	`_`, `\_`,
	`*`, `\*`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`~`, `\~`,
	"`", "\\`",
	`>`, `\>`,
	`#`, `\#`,
	`+`, `\+`,
	`-`, `\-`,
	`=`, `\=`,
	`|`, `\|`,
	`{`, `\{`,
	`}`, `\}`,
	`.`, `\.`,
	`!`, `\!`,
)

// EscapeMarkdownV2 escapes user-supplied text for Telegram MarkdownV2.
// Use with ParseMode: "MarkdownV2"; using this output with the legacy
// "Markdown" mode would over-escape (e.g. backslashed dots would render
// literally as "\.").
func EscapeMarkdownV2(s string) string {
	return MarkdownV2Replacer.Replace(s)
}
