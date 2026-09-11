package devicesocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// deviceTestServer serves a websocket endpoint that registers every dial as
// the device named by the "device" query parameter.
func deviceTestServer(t *testing.T, dm *DeviceManager, userID int64, maxDevices int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dm.AddDevice(w, r, "session", userID, r.URL.Query().Get("device"), maxDevices)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dialDevice(t *testing.T, srv *httptest.Server, deviceID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?device=" + deviceID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	return conn
}

// 【必须等注册完成再发起下一次 dial】客户端 Dial 在握手响应写回时就返回了，
// 而服务端的 AddDevice 还在注册途中。紧接着用同一个 device ID 再 dial，
// 两次注册的先后是竞态：后注册的那条会把先注册的踢掉——如果旧连接反而
// 后注册，被踢掉的就是新连接，表现为新连接收不到推送（close 1006）。
func waitOnline(t *testing.T, dm *DeviceManager, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&dm.totalOnline) != want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&dm.totalOnline); got != want {
		t.Fatalf("totalOnline = %d, want %d", got, want)
	}
}

// heartbeat replies and subscription pushes write to the same connection
// from different goroutines; the manager must serialize them instead of
// panicking with gorilla/websocket's concurrent-write assertion. Run with
// -race.
func TestConcurrentHeartbeatAndPushWrites(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	const userID = int64(7)
	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv, "dev1")
	waitOnline(t, dm, 1)

	var received atomic.Int64
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			received.Add(1)
		}
	}()

	const pushWorkers = 4
	const pushesPerWorker = 200
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < pushWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < pushesPerWorker; j++ {
				if err := dm.SendToDevice(userID, "dev1", "push"); err != nil {
					t.Errorf("SendToDevice: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					dm.UpdateHeartbeat(userID, "dev1")
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	// 【等的是「收够 800 条」，不是连接状态】超时曾经放宽到 30s 来压
	// flaky，但真正的原因是 dial 之后没等注册完成：第一条 SendToDevice
	// 撞上 "device offline" 会让 push worker 直接 return，剩下的消息
	// 根本没发出去，再长的超时也等不到。注册已由 waitOnline 保证。
	deadline := time.Now().Add(10 * time.Second)
	for received.Load() < pushWorkers*pushesPerWorker && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	conn.Close()
	<-closed
	if got := received.Load(); got < pushWorkers*pushesPerWorker {
		t.Errorf("received %d messages, want at least %d", got, pushWorkers*pushesPerWorker)
	}
}

// The kick notification must reach the client before the connection closes,
// which requires the device to stay registered until OnDeviceKicked returns.
func TestKickDeliversNotificationThenCloses(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	const userID = int64(9)
	var offlineEvents atomic.Int64
	dm.OnDeviceKicked = func(userID int64, deviceID, session string, operator Operator) {
		if err := dm.SendToDevice(userID, deviceID, `{"method":"kicked"}`); err != nil {
			t.Errorf("SendToDevice during kick callback: %v", err)
		}
	}
	dm.OnDeviceOffline = func(userID int64, deviceID, session string, createAt time.Time) {
		offlineEvents.Add(1)
	}

	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv, "dev1")

	dm.KickDevice(userID, "dev1")

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected kick notification before close, got error: %v", err)
	}
	if string(msg) != `{"method":"kicked"}` {
		t.Errorf("kick notification = %q, want %q", msg, `{"method":"kicked"}`)
	}
	// 【必须重设读 deadline】上一行 SetReadDeadline 设的是绝对时间点，
	// 已经被第一次 ReadMessage 消耗掉一部分。第一次读慢的时候（共享 CI
	// runner 上很常见），这一次读剩下的预算可能不足以等到连接关闭——
	// 表现就是该用例耗时正好卡在 2.00s 然后失败。
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("reset read deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("expected connection to close after the kick notification")
	}

	deadline := time.Now().Add(2 * time.Second)
	for offlineEvents.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := offlineEvents.Load(); got != 1 {
		t.Errorf("OnDeviceOffline fired %d times after kick, want 1", got)
	}
}

// Reconnecting with the same device ID must retire the previous socket:
// pushes reach the new connection and the online counter stays at one.
func TestReconnectReplacesPreviousSocket(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	const userID = int64(11)
	srv := deviceTestServer(t, dm, userID, 5)
	oldConn := dialDevice(t, srv, "dev1")
	waitOnline(t, dm, 1)
	newConn := dialDevice(t, srv, "dev1")

	if err := oldConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := oldConn.ReadMessage(); err == nil {
		t.Error("previous socket should be closed after reconnect")
	}

	if err := dm.SendToDevice(userID, "dev1", "hello"); err != nil {
		t.Fatalf("SendToDevice after reconnect: %v", err)
	}
	if err := newConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, msg, err := newConn.ReadMessage()
	if err != nil {
		t.Fatalf("push lost after reconnect: %v", err)
	}
	if string(msg) != "hello" {
		t.Errorf("push = %q, want %q", msg, "hello")
	}

	// 【必须轮询等待，不能断言瞬时值】totalOnline 的递减发生在连接清理的
	// goroutine 里（device.go 有 4 处并发增减）。旧连接读到关闭，不代表
	// 服务端已经处理完下线——直接断言是竞态，本地 40 次能挂 3 次。
	// 写法和同文件里等 offlineEvents 的循环一致。
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&dm.totalOnline) != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&dm.totalOnline); got != 1 {
		t.Errorf("totalOnline = %d after reconnect, want 1", got)
	}
}
