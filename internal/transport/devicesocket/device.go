package devicesocket

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/perfect-panel/server/pkg/logger"
)

type Operator int

const (
	MaxDevices Operator = iota
	Admin
	SubscribeUpdate = "subscribe_update"
)

// Device represents a device structure
type Device struct {
	Session      string
	DeviceID     string
	Conn         *websocket.Conn
	CreatedAt    time.Time
	LastPingTime time.Time

	// writeMu serializes writes on Conn. gorilla/websocket panics on
	// concurrent writes, and pushes (SendToDevice/Broadcast) race the
	// heartbeat replies written by the read loop.
	writeMu sync.Mutex
}

func (d *Device) write(message string) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.Conn.WriteMessage(websocket.TextMessage, []byte(message))
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// DeviceManager manages devices
type DeviceManager struct {
	userDevices      sync.Map // userID -> []*Device
	totalOnline      int32    // total online devices
	userMutexes      sync.Map // userID level locks
	heartbeatTimeout int      // heartbeat timeout (seconds)
	checkInterval    int      // heartbeat check interval (seconds)

	quit     chan struct{}
	quitOnce sync.Once

	// event callbacks
	OnDeviceOnline  func(userID int64, deviceID, session string)
	OnDeviceOffline func(userID int64, deviceID, session string, createAt time.Time)
	OnDeviceKicked  func(userID int64, deviceID, session string, operator Operator)
	OnMessage       func(userID int64, deviceID, session string, message string)
}

// Get user-level mutex
func (dm *DeviceManager) getUserMutex(userID int64) *sync.Mutex {
	mu, _ := dm.userMutexes.LoadOrStore(userID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// snapshotDevices returns a copy of the user's device list. The slice is
// mutated in place by removals, so readers must copy it under the user lock
// before iterating.
func (dm *DeviceManager) snapshotDevices(userID int64) []*Device {
	mu := dm.getUserMutex(userID)
	mu.Lock()
	defer mu.Unlock()
	if val, ok := dm.userDevices.Load(userID); ok {
		return append([]*Device(nil), val.([]*Device)...)
	}
	return nil
}

// Listen to WebSocket data
func (dm *DeviceManager) listenToDevice(userID int64, device *Device) {
	defer func() {
		dm.removeDevice(userID, device) // remove device when disconnected
	}()

	for {
		_, msg, err := device.Conn.ReadMessage()
		if err != nil {
			logger.Infow("device disconnected", logger.Field("device_id", device.DeviceID), logger.Field("user_id", userID), logger.Field("error", err))
			break
		}

		message := string(msg)
		if message == "ping" || message == "heartbeat" {
			dm.UpdateHeartbeat(userID, device.DeviceID)
			continue
		}

		// Trigger message callback
		if dm.OnMessage != nil {
			go dm.OnMessage(userID, device.DeviceID, device.Session, message)
		}
	}
}

// UpdateHeartbeat updates device heartbeat
func (dm *DeviceManager) UpdateHeartbeat(userID int64, deviceID string) {
	mu := dm.getUserMutex(userID)
	mu.Lock()
	defer mu.Unlock()

	if val, ok := dm.userDevices.Load(userID); ok {
		devices := val.([]*Device)
		for _, d := range devices {
			if d.DeviceID == deviceID {
				d.LastPingTime = time.Now()
				if err := d.write("ping"); err != nil {
					logger.Errorw("device heartbeat response failed", logger.Field("device_id", deviceID), logger.Field("user_id", userID), logger.Field("error", err))
				}
				break
			}
		}
	}
}

// AddDevice **Add: Device connects WebSocket and is added to the manager**
func (dm *DeviceManager) AddDevice(w http.ResponseWriter, r *http.Request, session string, userID int64, deviceID string, maxDevices int) {
	// **Upgrade WebSocket connection**
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorw("device websocket upgrade failed", logger.Field("error", err))
		return
	}

	// A non-positive limit means no explicit cap is configured; fall back to
	// the historical default instead of allowing unlimited connections.
	if maxDevices < 1 {
		maxDevices = 99
	}

	newDevice := &Device{
		Session:      session,
		DeviceID:     deviceID,
		Conn:         conn,
		CreatedAt:    time.Now(),
		LastPingTime: time.Now(),
	}

	var kicked, replaced *Device
	mu := dm.getUserMutex(userID)
	mu.Lock()
	var devices []*Device
	if val, ok := dm.userDevices.Load(userID); ok {
		for _, d := range val.([]*Device) {
			if d.DeviceID == deviceID {
				replaced = d // a reconnect retires the previous socket
				continue
			}
			devices = append(devices, d)
		}
	}

	// **If exceeding the limit, kick out the earliest device**
	if replaced == nil && len(devices) >= maxDevices {
		kicked = devices[0]
		devices = devices[1:]
	}

	// Add new device
	devices = append(devices, newDevice)
	dm.userDevices.Store(userID, devices)
	atomic.AddInt32(&dm.totalOnline, 1)
	mu.Unlock()

	// Side effects run outside the user lock: the kick callback calls back
	// into SendToDevice, which takes the lock, so running it under the lock
	// would deadlock. The kicked device stays registered until the callback
	// returns so its notification is actually delivered.
	if kicked != nil {
		if dm.OnDeviceKicked != nil {
			dm.OnDeviceKicked(userID, kicked.DeviceID, kicked.Session, MaxDevices)
		}
		dm.removeDevice(userID, kicked)
	}
	if replaced != nil {
		replaced.Conn.Close()
		atomic.AddInt32(&dm.totalOnline, -1)
	}

	// Trigger online event
	if dm.OnDeviceOnline != nil {
		go dm.OnDeviceOnline(userID, deviceID, session)
	}

	// Start listening
	go dm.listenToDevice(userID, newDevice)
}

// removeDevice removes a device, matching by connection identity: several
// sockets may share a DeviceID during a reconnect, and each read loop must
// retire only its own connection. Closing the connection, decrementing the
// online counter and emitting OnDeviceOffline happen only when the device is
// still registered, so retried removals are no-ops.
func (dm *DeviceManager) removeDevice(userID int64, device *Device) {
	mu := dm.getUserMutex(userID)
	mu.Lock()
	defer mu.Unlock()

	if val, ok := dm.userDevices.Load(userID); ok {
		devices := val.([]*Device)
		for i, d := range devices {
			if d != device {
				continue
			}
			devices = append(devices[:i], devices[i+1:]...)
			d.Conn.Close()
			atomic.AddInt32(&dm.totalOnline, -1)

			if dm.OnDeviceOffline != nil {
				go dm.OnDeviceOffline(userID, d.DeviceID, d.Session, d.CreatedAt)
			}
			break
		}

		if len(devices) == 0 {
			dm.userDevices.Delete(userID)
		} else {
			dm.userDevices.Store(userID, devices)
		}
	}
}

// KickDevice kicks a device (supports individual device or entire user)
func (dm *DeviceManager) KickDevice(userID int64, deviceID string) {
	devices := dm.snapshotDevices(userID)

	var kicked []*Device
	for _, d := range devices {
		if deviceID == "" || d.DeviceID == deviceID {
			kicked = append(kicked, d)
		}
	}
	if len(kicked) == 0 {
		logger.Infow("user has no online devices to kick", logger.Field("user_id", userID))
		return
	}

	// Callbacks run while the device is still registered so the notification
	// can be delivered through SendToDevice; removeDevice then closes the
	// socket and emits OnDeviceOffline.
	for _, d := range kicked {
		if dm.OnDeviceKicked != nil {
			dm.OnDeviceKicked(userID, d.DeviceID, d.Session, Admin)
		}
		dm.removeDevice(userID, d)
		logger.Infow("device kicked", logger.Field("device_id", d.DeviceID), logger.Field("user_id", userID))
	}
}

// StartHeartbeatCheck periodically checks for heartbeat timeout devices
func (dm *DeviceManager) StartHeartbeatCheck() {
	ticker := time.NewTicker(time.Duration(dm.checkInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dm.quit:
			return
		case <-ticker.C:
			dm.checkHeartbeats()
		}
	}
}

func (dm *DeviceManager) checkHeartbeats() {
	now := time.Now()

	dm.userDevices.Range(func(userID, val interface{}) bool {
		uid := userID.(int64)

		mu := dm.getUserMutex(uid)
		mu.Lock()
		defer mu.Unlock()

		devices := val.([]*Device)
		var activeDevices []*Device
		for _, d := range devices {
			if now.Sub(d.LastPingTime) > time.Duration(dm.heartbeatTimeout)*time.Second {
				logger.Infow("device heartbeat timed out", logger.Field("device_id", d.DeviceID), logger.Field("user_id", uid))
				d.Conn.Close()
				atomic.AddInt32(&dm.totalOnline, -1)

				if dm.OnDeviceOffline != nil {
					go dm.OnDeviceOffline(uid, d.DeviceID, d.Session, d.CreatedAt)
				}
			} else {
				activeDevices = append(activeDevices, d)
			}
		}

		if len(activeDevices) == 0 {
			dm.userDevices.Delete(uid)
		} else {
			dm.userDevices.Store(uid, activeDevices)
		}
		return true
	})
	// Deliberately avoid logging every heartbeat sweep.
}

// NewDeviceManager creates a new device manager
func NewDeviceManager(heartbeatTimeout, checkInterval int) *DeviceManager {
	dm := &DeviceManager{
		heartbeatTimeout: heartbeatTimeout,
		checkInterval:    checkInterval,
		quit:             make(chan struct{}),
	}
	go dm.StartHeartbeatCheck()
	return dm
}

// Stop terminates the heartbeat sweep. It is idempotent so lifecycle hooks
// can call it unconditionally.
func (dm *DeviceManager) Stop() {
	dm.quitOnce.Do(func() { close(dm.quit) })
}

// SendToDevice sends a message to a specific device
func (dm *DeviceManager) SendToDevice(userID int64, deviceID string, message string) error {
	devices := dm.snapshotDevices(userID)
	for _, d := range devices {
		if deviceID == "" {
			if err := d.write(message); err != nil {
				return err
			}
			continue
		}
		if d.DeviceID == deviceID {
			return d.write(message)
		}
	}
	return fmt.Errorf("device %s (User %d) is offline", deviceID, userID)
}

// Broadcast sends a message to all devices
func (dm *DeviceManager) Broadcast(message string) {
	go func(message string) {
		dm.userDevices.Range(func(userID, val interface{}) bool {
			for _, d := range dm.snapshotDevices(userID.(int64)) {
				_ = d.write(message)
			}
			return true
		})
	}(message)

}

// Gracefully shut down all WebSocket connections
func (dm *DeviceManager) Shutdown(ctx context.Context) {
	<-ctx.Done()
	dm.Stop()
	logger.Info("shutting down all device websocket connections")

	dm.userDevices.Range(func(userID, val interface{}) bool {
		uid := userID.(int64)
		for _, d := range dm.snapshotDevices(uid) {
			dm.removeDevice(uid, d)
		}
		return true
	})
}
