package telegram

import "testing"

func commandNames(commands []Command) map[string]bool {
	names := make(map[string]bool, len(commands))
	for _, command := range commands {
		names[command.Command] = true
	}
	return names
}

// The destructive administrator commands must never reach the menu every user
// sees; only binding and help belong there.
func TestPublicCommandsExcludeAdministratorEntries(t *testing.T) {
	names := commandNames(PublicCommands())
	for _, want := range []string{"start", "bind", "help"} {
		if !names[want] {
			t.Fatalf("public menu is missing /%s", want)
		}
	}
	for _, forbidden := range []string{"ban", "reset", "toggle", "dash", "user"} {
		if names[forbidden] {
			t.Fatalf("public menu exposes the administrator command /%s", forbidden)
		}
	}
}

// The administrator menu carries the full command set, and never the inline
// confirmation commands, which only make sense with a generated action id.
func TestAdminCommandsCoverTheDispatchedSet(t *testing.T) {
	names := commandNames(AdminCommands())
	dispatched := []string{
		"dash", "tickets", "tickets_waiting", "tk", "rp", "close", "reopen",
		"user", "user_sub", "user_log", "reset", "toggle", "ban", "help",
	}
	for _, want := range dispatched {
		if !names[want] {
			t.Fatalf("administrator menu is missing /%s", want)
		}
	}
	for _, forbidden := range []string{"confirm_", "cancel_"} {
		if names[forbidden] {
			t.Fatalf("administrator menu exposes the inline command /%s", forbidden)
		}
	}
}
