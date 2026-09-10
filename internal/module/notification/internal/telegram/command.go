package telegram

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

// commandEntity returns the bot_command entity when the message starts with
// one. The bounds check guards against malformed entities in untrusted
// webhook payloads.
func commandEntity(msg *models.Message) (models.MessageEntity, bool) {
	if msg == nil || len(msg.Entities) == 0 {
		return models.MessageEntity{}, false
	}
	entity := msg.Entities[0]
	if entity.Type != models.MessageEntityTypeBotCommand || entity.Offset != 0 ||
		entity.Length <= 0 || entity.Length > len(msg.Text) {
		return models.MessageEntity{}, false
	}
	return entity, true
}

// messageCommand extracts the command name without the leading slash or the
// @botname mention suffix. It returns "" when the message is not a command.
func messageCommand(msg *models.Message) string {
	entity, ok := commandEntity(msg)
	if !ok {
		return ""
	}
	command := msg.Text[1:entity.Length]
	if at := strings.Index(command, "@"); at != -1 {
		command = command[:at]
	}
	return command
}

// commandArguments returns the text following the command, or "" when the
// command has no arguments.
func commandArguments(msg *models.Message) string {
	entity, ok := commandEntity(msg)
	if !ok || entity.Length >= len(msg.Text) {
		return ""
	}
	return msg.Text[entity.Length+1:]
}
