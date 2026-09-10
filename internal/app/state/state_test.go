package state

import (
	"sync"
	"testing"

	"github.com/perfect-panel/server/internal/config"
)

func TestStatePublishesConcurrentConfigUpdates(t *testing.T) {
	state := New(config.Config{})
	const updates = 64

	var writers sync.WaitGroup
	for i := 0; i < updates; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			state.UpdateConfig(func(current *config.Config) {
				current.Port++
			})
		}()
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 16; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = state.Config().Port
				}
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
	if got := state.Config().Port; got != updates {
		t.Fatalf("lost concurrent updates: got %d, want %d", got, updates)
	}
}

func TestStateLifecycleHandlers(t *testing.T) {
	state := New(config.Config{})
	if err := state.Restart(); err != nil {
		t.Fatalf("unexpected unavailable restart result: %v", err)
	}

	restarted := false
	state.SetRestart(func() error { restarted = true; return nil })
	if err := state.Restart(); err != nil || !restarted {
		t.Fatalf("restart handler was not invoked: restarted=%v err=%v", restarted, err)
	}

	var subsystem string
	state.SetReinitialize(func(value string) { subsystem = value })
	state.Reinitialize("node")
	if subsystem != "node" {
		t.Fatalf("unexpected subsystem: %q", subsystem)
	}
}
