package order

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/repository"
)

type reconcileOrderRepo struct {
	repository.OrderRepo
	orders []*orderEntity.Order
}

func (r reconcileOrderRepo) QueryOrdersByStatusAfterID(_ context.Context, status uint8, afterID int64, limit int) ([]*orderEntity.Order, error) {
	result := make([]*orderEntity.Order, 0, limit)
	for _, item := range r.orders {
		if item.Status == status && item.Id > afterID {
			result = append(result, item)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

type reconcileStore struct {
	repository.Store
	orders repository.OrderRepo
}

func (s reconcileStore) Order() repository.OrderRepo { return s.orders }

func newReconcileTestContext(t *testing.T, orders []*orderEntity.Order) (Dependencies, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisOpt := asynq.RedisClientOpt{Addr: redisServer.Addr()}
	queue := taskqueue.NewClient(asynq.NewClient(redisOpt))
	t.Cleanup(func() { _ = queue.Close() })
	inspector := asynq.NewInspector(redisOpt)
	t.Cleanup(func() { _ = inspector.Close() })
	return Dependencies{
		Store:     reconcileStore{orders: reconcileOrderRepo{orders: orders}},
		Queue:     queue,
		Inspector: inspector,
	}, redisServer
}

func TestReconcilePaidOrdersEnqueuesEachPaidOrderIdempotently(t *testing.T) {
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: "paid-1", Status: OrderStatusPaid},
		{Id: 2, OrderNo: "pending", Status: OrderStatusPending},
		{Id: 3, OrderNo: "paid-2", Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("duplicate ProcessTask: %v", err)
	}
	tasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected exactly 2 activation tasks, got %d", len(tasks))
	}
}

func TestReconcilePaidOrdersArchivedRecovery(t *testing.T) {
	orderNo := "archived-test-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("first ProcessTask: %v", err)
	}

	err := deps.Inspector.ArchiveTask("default", taskID)
	if err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	archivedTasks, err := deps.Inspector.ListArchivedTasks("default")
	if err != nil {
		t.Fatalf("ListArchivedTasks: %v", err)
	}
	if len(archivedTasks) != 1 {
		t.Fatalf("expected 1 archived task, got %d", len(archivedTasks))
	}

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("recovery ProcessTask: %v", err)
	}

	taskInfo, err := deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if taskInfo.State != asynq.TaskStatePending {
		t.Fatalf("expected task to be pending after recovery, got %v", taskInfo.State)
	}
	if taskInfo.Type != taskqueue.ForthwithActivateOrder {
		t.Fatalf("expected task type %s, got %s", taskqueue.ForthwithActivateOrder, taskInfo.Type)
	}

	pendingTasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pendingTasks) != 1 {
		t.Fatalf("expected exactly 1 pending task, got %d", len(pendingTasks))
	}
}

func TestReconcilePaidOrdersNonArchivedPreserved(t *testing.T) {
	orderNo := "preserved-test-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("first ProcessTask: %v", err)
	}

	info, err := deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if info.State != asynq.TaskStatePending {
		t.Fatalf("expected pending initially, got %v", info.State)
	}

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("second ProcessTask: %v", err)
	}

	info, err = deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if info.State != asynq.TaskStatePending {
		t.Fatalf("expected pending preserved, got %v", info.State)
	}

	pendingTasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pendingTasks) != 1 {
		t.Fatalf("expected exactly 1 pending task, got %d", len(pendingTasks))
	}
}

func TestReconcilePaidOrdersArchivedTypeMismatch(t *testing.T) {
	orderNo := "type-mismatch-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	payload, _ := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: orderNo})
	wrongTypeTask := asynq.NewTask("some-other-type", payload, asynq.MaxRetry(5))
	_, err := deps.Queue.EnqueueContext(context.Background(), wrongTypeTask, asynq.TaskID(taskID))
	if err != nil {
		t.Fatalf("EnqueueContext with wrong type: %v", err)
	}

	err = deps.Inspector.ArchiveTask("default", taskID)
	if err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask must repair a type-mismatched task, got: %v", err)
	}

	assertRepairedActivationTask(t, deps, taskID, orderNo)
}

func TestReconcilePaidOrdersArchivedPayloadOrderNoMismatch(t *testing.T) {
	orderNo := "order-match-1"
	otherOrderNo := "order-match-other"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	payload, _ := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: otherOrderNo})
	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, payload, asynq.MaxRetry(5))
	_, err := deps.Queue.EnqueueContext(context.Background(), task, asynq.TaskID(taskID))
	if err != nil {
		t.Fatalf("EnqueueContext: %v", err)
	}

	err = deps.Inspector.ArchiveTask("default", taskID)
	if err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask must repair an OrderNo-mismatched task, got: %v", err)
	}

	assertRepairedActivationTask(t, deps, taskID, orderNo)
}

// The 2026-08-05 production incident: an archived activation task whose
// payload carries no order_no ("{}") made every reconcile run error out and
// retry forever. The run must repair the task and finish cleanly.
func TestReconcilePaidOrdersArchivedEmptyPayloadRepaired(t *testing.T) {
	orderNo := "empty-payload-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, []byte("{}"), asynq.MaxRetry(5))
	if _, err := deps.Queue.EnqueueContext(context.Background(), task, asynq.TaskID(taskID)); err != nil {
		t.Fatalf("EnqueueContext: %v", err)
	}
	if err := deps.Inspector.ArchiveTask("default", taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask must repair an empty-payload task, got: %v", err)
	}

	assertRepairedActivationTask(t, deps, taskID, orderNo)
}

// assertRepairedActivationTask verifies the corrupt task was replaced by a
// pending activation task carrying the canonical payload.
func assertRepairedActivationTask(t *testing.T, deps Dependencies, taskID, orderNo string) {
	t.Helper()
	taskInfo, err := deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if taskInfo.State != asynq.TaskStatePending {
		t.Fatalf("expected repaired task to be pending, got %v", taskInfo.State)
	}
	if taskInfo.Type != taskqueue.ForthwithActivateOrder {
		t.Fatalf("expected task type %s, got %s", taskqueue.ForthwithActivateOrder, taskInfo.Type)
	}
	var payload taskqueue.ForthwithActivateOrderPayload
	if err := json.Unmarshal(taskInfo.Payload, &payload); err != nil {
		t.Fatalf("unmarshal repaired payload: %v", err)
	}
	if payload.OrderNo != orderNo {
		t.Fatalf("expected repaired payload order_no %s, got %q", orderNo, payload.OrderNo)
	}
}

func TestReconcilePaidOrdersNotFoundReenqueue(t *testing.T) {
	orderNo := "notfound-1"
	deps, redisSrv := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("first ProcessTask: %v", err)
	}

	err := deps.Inspector.ArchiveTask("default", taskID)
	if err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	redisSrv.Del("asynq:{default}:t:" + taskID)
	redisSrv.ZRem("asynq:{default}:archived", taskID)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("recovery ProcessTask: %v", err)
	}

	taskInfo, err := deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if taskInfo.State != asynq.TaskStatePending {
		t.Fatalf("expected task to be re-enqueued as pending, got %v", taskInfo.State)
	}

	pendingTasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pendingTasks) != 1 {
		t.Fatalf("expected exactly 1 pending task, got %d", len(pendingTasks))
	}
}

func TestReconcilePaidOrdersOnlyPaidAreEnqueued(t *testing.T) {
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: "paid-1", Status: OrderStatusPaid},
		{Id: 2, OrderNo: "close-1", Status: OrderStatusClose},
		{Id: 3, OrderNo: "failed-1", Status: OrderStatusFailed},
		{Id: 4, OrderNo: "finished-1", Status: OrderStatusFinished},
		{Id: 5, OrderNo: "paid-2", Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	tasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected exactly 2 activation tasks for paid orders, got %d", len(tasks))
	}
}

func TestIsStalePaid(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		updatedAt time.Time
		wantStale bool
	}{
		{name: "now", updatedAt: now, wantStale: false},
		{name: "9 minutes", updatedAt: now.Add(-9 * time.Minute), wantStale: false},
		{name: "11 minutes", updatedAt: now.Add(-11 * time.Minute), wantStale: true},
		{name: "20 minutes", updatedAt: now.Add(-20 * time.Minute), wantStale: true},
		{name: "1 hour", updatedAt: now.Add(-1 * time.Hour), wantStale: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStalePaid(tt.updatedAt, now)
			if got != tt.wantStale {
				t.Errorf("isStalePaid(%v, %v) = %v, want %v", tt.updatedAt, now, got, tt.wantStale)
			}
		})
	}
}

func TestReconcilePaidOrdersStaleDetection(t *testing.T) {
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: "fresh", Status: OrderStatusPaid, UpdatedAt: time.Now()},
		{Id: 2, OrderNo: "truly-stale", Status: OrderStatusPaid, UpdatedAt: time.Now().Add(-20 * time.Minute)},
		{Id: 3, OrderNo: "recently-paid-old-creation", Status: OrderStatusPaid, CreatedAt: time.Now().Add(-20 * time.Minute), UpdatedAt: time.Now()},
	})
	logic := NewReconcilePaidOrdersLogic(deps)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	tasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 pending tasks, got %d", len(tasks))
	}
}

func TestReconcilePaidOrdersArchivedRunTaskRace(t *testing.T) {
	orderNo := "race-test-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("first ProcessTask: %v", err)
	}

	var err error
	err = deps.Inspector.ArchiveTask("default", taskID)
	if err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	err = deps.Inspector.RunTask("default", taskID)
	if err != nil {
		t.Fatalf("RunTask (simulating concurrent recovery before reconcile): %v", err)
	}
	taskInfo, err := deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if taskInfo.State != asynq.TaskStatePending {
		t.Fatalf("expected pending after RunTask, got %v", taskInfo.State)
	}

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("recovery ProcessTask: %v", err)
	}

	taskInfo, err = deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if taskInfo.State != asynq.TaskStatePending {
		t.Fatalf("expected task to remain pending after race scenario, got %v", taskInfo.State)
	}

	pendingTasks, err := deps.Inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pendingTasks) != 1 {
		t.Fatalf("expected exactly 1 pending task, got %d", len(pendingTasks))
	}
}

func TestReconcilePaidOrdersHandleArchivedBenignRace(t *testing.T) {
	orderNo := "handle-archived-race-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	payload, _ := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: orderNo})
	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, payload, asynq.MaxRetry(5))
	_, err := deps.Queue.EnqueueContext(context.Background(), task, asynq.TaskID(taskID))
	if err != nil {
		t.Fatalf("EnqueueContext: %v", err)
	}

	info, err := deps.Inspector.GetTaskInfo("default", taskID)
	if err != nil {
		t.Fatalf("GetTaskInfo: %v", err)
	}
	if info.State != asynq.TaskStatePending {
		t.Fatalf("expected pending, got %v", info.State)
	}

	fakeInfo := &asynq.TaskInfo{
		ID:      taskID,
		Queue:   "default",
		Type:    taskqueue.ForthwithActivateOrder,
		Payload: payload,
		State:   asynq.TaskStateArchived,
	}

	action, state, err := logic.handleArchived(context.Background(), orderNo, taskID, fakeInfo)
	if err != nil {
		t.Fatalf("handleArchived: %v", err)
	}
	if action != conflictKept {
		t.Fatalf("expected conflictKept, got %v", action)
	}
	if state != asynq.TaskStatePending {
		t.Fatalf("expected pending state after benign race, got %v", state)
	}
}

func TestReconcilePaidOrdersHandleArchivedTypeMismatchReturnsError(t *testing.T) {
	orderNo := "handle-archived-type-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	payload, _ := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: orderNo})
	fakeInfo := &asynq.TaskInfo{
		ID:      taskID,
		Queue:   "default",
		Type:    "some-other-type",
		Payload: payload,
		State:   asynq.TaskStateArchived,
	}

	_, _, err := logic.handleArchived(context.Background(), orderNo, taskID, fakeInfo)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestReconcilePaidOrdersHandleArchivedOrderNoMismatchReturnsError(t *testing.T) {
	orderNo := "handle-archived-oid-1"
	deps, _ := newReconcileTestContext(t, []*orderEntity.Order{
		{Id: 1, OrderNo: orderNo, Status: OrderStatusPaid},
	})
	logic := NewReconcilePaidOrdersLogic(deps)
	taskID := taskqueue.ActivationTaskID(orderNo)

	wrongPayload, _ := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: "some-other-order"})
	fakeInfo := &asynq.TaskInfo{
		ID:      taskID,
		Queue:   "default",
		Type:    taskqueue.ForthwithActivateOrder,
		Payload: wrongPayload,
		State:   asynq.TaskStateArchived,
	}

	_, _, err := logic.handleArchived(context.Background(), orderNo, taskID, fakeInfo)
	if err == nil {
		t.Fatal("expected error for OrderNo mismatch")
	}
}

func TestReconcilePaidOrdersMultipleBatches(t *testing.T) {
	n := 3 * paidOrderReconcileBatchSize / 2
	orders := make([]*orderEntity.Order, 0, n)
	for i := int64(1); i <= int64(n); i++ {
		orders = append(orders, &orderEntity.Order{
			Id:      i,
			OrderNo: fmt.Sprintf("batch-paid-%d", i),
			Status:  OrderStatusPaid,
		})
	}
	deps, _ := newReconcileTestContext(t, orders)
	logic := NewReconcilePaidOrdersLogic(deps)

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	tasks, err := deps.Inspector.ListPendingTasks("default", asynq.PageSize(n+1))
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(tasks) != n {
		t.Fatalf("expected %d pending tasks, got %d", n, len(tasks))
	}
}
