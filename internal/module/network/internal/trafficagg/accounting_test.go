package trafficagg

import (
	"context"
	"errors"
	"maps"
	"testing"

	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

var errAccounting = errors.New("accounting write failed")

type accountingStore struct {
	repository.Store
	marks            map[string]bool
	upload, download int64
	logs             int
	fail             string
}

func (s *accountingStore) Inbox() repository.InboxRepo { return accountingInbox{s: s} }
func (s *accountingStore) UserSubscription() repository.UserSubscriptionRepo {
	return accountingSubscriptions{s: s}
}
func (s *accountingStore) TrafficLog() repository.TrafficRepo { return accountingLogs{s: s} }

// Model the transaction rollback, including an inbox failure after a domain
// write. The two domain transactions deliberately commit independently.
func (s *accountingStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	beforeUpload, beforeDownload, beforeMarks := s.upload, s.download, maps.Clone(s.marks)
	if err := fn(s); err != nil {
		s.upload, s.download, s.marks = beforeUpload, beforeDownload, beforeMarks
		return err
	}
	return nil
}

func (s *accountingStore) InNetworkTx(_ context.Context, fn func(repository.NetworkStore) error) error {
	beforeLogs, beforeMarks := s.logs, maps.Clone(s.marks)
	if err := fn(s); err != nil {
		s.logs, s.marks = beforeLogs, beforeMarks
		return err
	}
	return nil
}

type accountingInbox struct {
	repository.InboxRepo
	s *accountingStore
}

func (r accountingInbox) Find(_ context.Context, consumer, key string) (*inbox.Record, error) {
	if r.s.marks[consumer+"|"+key] {
		return &inbox.Record{Consumer: consumer, EventKey: key}, nil
	}
	return nil, nil
}

func (r accountingInbox) Insert(_ context.Context, consumer, key, _ string) error {
	if r.s.fail == consumer {
		return errAccounting
	}
	if r.s.marks[consumer+"|"+key] {
		return errors.New("duplicate inbox record")
	}
	r.s.marks[consumer+"|"+key] = true
	return nil
}

type accountingSubscriptions struct {
	repository.UserSubscriptionRepo
	s *accountingStore
}

func (r accountingSubscriptions) FindSubscribesByIds(context.Context, []int64) ([]*usersub.Subscribe, error) {
	return []*usersub.Subscribe{{Id: 2, UserId: 7}}, nil
}

func (r accountingSubscriptions) BatchUpdateUserSubscribeWithTraffic(_ context.Context, deltas []trafficEntity.SubscribeTrafficDelta, _ ...*gorm.DB) error {
	if r.s.fail == "usage" {
		return errAccounting
	}
	for _, delta := range deltas {
		r.s.upload += delta.Upload
		r.s.download += delta.Download
	}
	return nil
}

type accountingLogs struct {
	repository.TrafficRepo
	s *accountingStore
}

func (r accountingLogs) InsertBatch(_ context.Context, logs []*trafficEntity.TrafficLog, _ int, _ ...*gorm.DB) error {
	if r.s.fail == "logs" {
		return errAccounting
	}
	r.s.logs += len(logs)
	return nil
}

func TestTrafficAccountingRetryAcrossDomainCommits(t *testing.T) {
	const bucket = "202609050900"
	for _, failure := range []string{"usage", "subscription.traffic_bucket", "logs", "network.traffic_bucket"} {
		t.Run(failure, func(t *testing.T) {
			store := &accountingStore{marks: map[string]bool{}, fail: failure}
			aggregator := New(Deps{Store: store, Usage: subscription.NewTrafficUsage(store)})
			deltas := []trafficDelta{{ServerId: 1, SubscribeId: 2, Upload: 3, Download: 5}}
			if err := aggregator.persistBucket(context.Background(), bucket, deltas); !errors.Is(err, errAccounting) {
				t.Fatalf("first attempt: %v", err)
			}
			usageCommitted := failure == "logs" || failure == "network.traffic_bucket"
			if usageCommitted {
				if store.upload != 3 || store.download != 5 || !store.marks["subscription.traffic_bucket|"+bucket] {
					t.Fatal("network failure lost the committed subscription transaction")
				}
			} else if store.upload != 0 || store.download != 0 || len(store.marks) != 0 {
				t.Fatal("failed subscription transaction was not rolled back")
			}
			if store.logs != 0 || store.marks["network.traffic_bucket|"+bucket] {
				t.Fatal("failed pipeline committed network logs")
			}
			store.fail = ""
			for range 2 {
				if err := aggregator.persistBucket(context.Background(), bucket, deltas); err != nil {
					t.Fatalf("retry: %v", err)
				}
			}
			if store.upload != 3 || store.download != 5 || store.logs != 1 || len(store.marks) != 2 {
				t.Fatalf("replay duplicated accounting: upload=%d download=%d logs=%d marks=%v", store.upload, store.download, store.logs, store.marks)
			}
		})
	}
}
