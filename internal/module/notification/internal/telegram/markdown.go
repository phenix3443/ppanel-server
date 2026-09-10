package telegram

import (
	"bytes"
	"strings"
	"text/template"
)

// markdownV2Escaper escapes every character MarkdownV2 treats as syntax
// (https://core.telegram.org/bots/api#markdownv2-style). The backslash comes
// first so an escape is never itself re-escaped.
var markdownV2Escaper = strings.NewReplacer(
	`\`, `\\`,
	"_", `\_`, "*", `\*`, "[", `\[`, "]", `\]`, "(", `\(`, ")", `\)`,
	"~", `\~`, "`", "\\`", ">", `\>`, "#", `\#`, "+", `\+`, "-", `\-`,
	"=", `\=`, "|", `\|`, "{", `\{`, "}", `\}`, ".", `\.`, "!", `\!`,
)

// EscapeMarkdownV2 returns s with all MarkdownV2 syntax characters escaped,
// so arbitrary data renders as literal text.
func EscapeMarkdownV2(s string) string {
	return markdownV2Escaper.Replace(s)
}

// RenderMarkdownV2 renders a message template after escaping every data
// value. Templates own their formatting; data is always literal text. This
// is the only correct way to build MarkdownV2 messages from dynamic data:
// one unescaped '.' or '-' in an order number makes Telegram reject the
// whole message.
func RenderMarkdownV2(tpl string, data map[string]string) (string, error) {
	escaped := make(map[string]string, len(data))
	for key, value := range data {
		escaped[key] = EscapeMarkdownV2(value)
	}
	// missingkey=zero renders an absent key as "" instead of "<no value>",
	// whose '>' would make Telegram reject the whole message.
	t, err := template.New("telegram").Option("missingkey=zero").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, escaped); err != nil {
		return "", err
	}
	return buf.String(), nil
}
