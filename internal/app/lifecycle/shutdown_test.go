//go:build linux || darwin

package lifecycle

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShutdown(t *testing.T) {
	SetTimeToForceQuit(time.Hour)
	assert.Equal(t, time.Hour, delayTimeBeforeForceQuit)

	var val int
	called := AddWrapUpListener(func() {
		val++
	})
	WrapUp()
	called()
	assert.Equal(t, 1, val)

	called = AddShutdownListener(func() {
		val += 2
	})
	Shutdown()
	called()
	assert.Equal(t, 3, val)
}

func TestShutdownWaitsForAllHooksAfterPanic(t *testing.T) {
	var manager listenerManager
	var completed atomic.Bool
	wait := manager.addListener(func() { panic("test shutdown hook") })
	manager.addListener(func() { completed.Store(true) })
	manager.notifyListeners()
	wait()
	if !completed.Load() {
		t.Fatal("panic prevented another shutdown hook from completing")
	}
	manager.notifyListeners()
}

func TestNotifyMoreThanOnce(t *testing.T) {
	ch := make(chan struct{}, 1)

	go func() {
		var val int
		called := AddWrapUpListener(func() {
			val++
		})
		WrapUp()
		WrapUp()
		called()
		assert.Equal(t, 1, val)

		called = AddShutdownListener(func() {
			val += 2
		})
		Shutdown()
		Shutdown()
		called()
		assert.Equal(t, 3, val)
		ch <- struct{}{}
	}()

	select {
	case <-ch:
		fmt.Printf("TestNotifyMoreThanOnce done\n")
	case <-time.After(time.Second):
		t.Fatal("timeout, check error logs")
	}
}
