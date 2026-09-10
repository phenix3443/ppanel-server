package telegram

import (
	"context"
	"testing"
)

type recordingTelegramMessenger struct {
	chatID   int64
	threadID int64
	message  string
	markdown bool
}

func (m *recordingTelegramMessenger) Send(chatID, threadID int64, message string) error {
	m.chatID = chatID
	m.threadID = threadID
	m.message = message
	m.markdown = false
	return nil
}

func (m *recordingTelegramMessenger) SendMarkdown(chatID, threadID int64, message string) error {
	m.chatID = chatID
	m.threadID = threadID
	m.message = message
	m.markdown = true
	return nil
}

func TestTelegramBindUsesInjectedMessenger(t *testing.T) {
	messenger := &recordingTelegramMessenger{}
	logic := NewTelegramLogic(context.Background(), TelegramLogicDependencies{Messenger: messenger})

	if err := logic.bind(42, ""); err != nil {
		t.Fatalf("bind error = %v", err)
	}
	if messenger.chatID != 42 || messenger.message != "Please provide a bind token. Usage: /bind <token>" {
		t.Fatalf("message = (%d, %q)", messenger.chatID, messenger.message)
	}
}
