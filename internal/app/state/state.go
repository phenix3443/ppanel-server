// Package appstate owns the process-wide mutable runtime state. Configuration
// is published as immutable snapshots so request and queue goroutines never
// race with an administrator-triggered reinitialization.
package state

import (
	"sync"
	"sync/atomic"

	tgbot "github.com/go-telegram/bot"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network"
)

type restartHandler struct{ fn func() error }
type reinitializeHandler struct{ fn func(string) }

type State struct {
	configMu sync.Mutex
	config   atomic.Pointer[config.Config]

	telegramBot           atomic.Pointer[tgbot.Bot]
	nodeMultiplierManager atomic.Pointer[network.MultiplierManager]
	restart               atomic.Pointer[restartHandler]
	reinitialize          atomic.Pointer[reinitializeHandler]
}

func New(initial config.Config) *State {
	state := new(State)
	state.config.Store(&initial)
	return state
}

// Config returns the current immutable configuration snapshot.
func (s *State) Config() config.Config {
	if current := s.config.Load(); current != nil {
		return *current
	}
	return config.Config{}
}

// UpdateConfig serializes partial updates and atomically publishes the result.
// Callers must replace nested slices/maps instead of mutating values retained
// from an older snapshot.
func (s *State) UpdateConfig(update func(*config.Config)) {
	if update == nil {
		return
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	next := s.Config()
	update(&next)
	s.config.Store(&next)
}

func (s *State) TelegramBot() *tgbot.Bot { return s.telegramBot.Load() }

func (s *State) SetTelegramBot(bot *tgbot.Bot) { s.telegramBot.Store(bot) }

func (s *State) NodeMultiplierManager() *network.MultiplierManager {
	return s.nodeMultiplierManager.Load()
}

func (s *State) SetNodeMultiplierManager(manager *network.MultiplierManager) {
	s.nodeMultiplierManager.Store(manager)
}

func (s *State) SetRestart(handler func() error) {
	if handler != nil {
		s.restart.Store(&restartHandler{fn: handler})
	}
}

func (s *State) Restart() error {
	if handler := s.restart.Load(); handler != nil {
		return handler.fn()
	}
	return nil
}

func (s *State) SetReinitialize(handler func(string)) {
	if handler != nil {
		s.reinitialize.Store(&reinitializeHandler{fn: handler})
	}
}

func (s *State) Reinitialize(subsystem string) {
	if handler := s.reinitialize.Load(); handler != nil {
		handler.fn(subsystem)
	}
}
