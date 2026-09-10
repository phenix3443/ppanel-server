package telegram

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func commandMessage(text string, entityLength int) *models.Message {
	return &models.Message{
		Text: text,
		Entities: []models.MessageEntity{{
			Type:   models.MessageEntityTypeBotCommand,
			Offset: 0,
			Length: entityLength,
		}},
	}
}

func TestMessageCommandParsing(t *testing.T) {
	cases := []struct {
		name    string
		msg     *models.Message
		command string
		args    string
	}{
		{"command with args", commandMessage("/bind token-1", 5), "bind", "token-1"},
		{"command without args", commandMessage("/help", 5), "help", ""},
		{"mention is stripped", commandMessage("/start@my_bot abc", 13), "start", "abc"},
		{"plain text", &models.Message{Text: "hello"}, "", ""},
		{"nil message", nil, "", ""},
		{"entity not at offset zero", &models.Message{
			Text:     "hi /bind x",
			Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Offset: 3, Length: 5}},
		}, "", ""},
		{"entity length out of bounds", commandMessage("/x", 99), "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageCommand(tc.msg); got != tc.command {
				t.Fatalf("messageCommand = %q, want %q", got, tc.command)
			}
			if got := commandArguments(tc.msg); got != tc.args {
				t.Fatalf("commandArguments = %q, want %q", got, tc.args)
			}
		})
	}
}
