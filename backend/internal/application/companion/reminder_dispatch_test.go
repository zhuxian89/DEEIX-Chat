package companion

import (
	"context"
	"fmt"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"sync"
	"testing"
	"time"
)

type dispatchFixture struct {
	Store
	mu                         sync.Mutex
	remaining, claimed         int
	claimCalls, skippedBatches int
	finished                   map[string]string
}

func (f *dispatchFixture) ReminderSnapshot(context.Context, uint, string, time.Time) (domain.ReminderSnapshot, error) {
	return domain.ReminderSnapshot{}, nil
}
func (f *dispatchFixture) ExecuteReminder(context.Context, uint, domain.ReminderCommand, time.Time) (domain.ReminderResult, error) {
	return domain.ReminderResult{}, nil
}
func (f *dispatchFixture) TickReminders(context.Context, time.Time, string) error { return nil }
func (f *dispatchFixture) ClaimReminderNotification(ctx context.Context, _ time.Time, _ string) (*domain.NotificationDelivery, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if f.skippedBatches > 0 {
		f.skippedBatches--
		return nil, true, nil
	}
	if f.remaining == 0 {
		return nil, false, nil
	}
	f.remaining--
	f.claimed++
	return &domain.NotificationDelivery{ID: fmt.Sprintf("delivery-%d", f.claimed), UserID: uint(f.claimed)}, true, nil
}
func (f *dispatchFixture) FinishReminderNotification(_ context.Context, id, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.finished[id]; ok {
		return fmt.Errorf("duplicate finish")
	}
	f.finished[id] = status
	return nil
}

type capacitySender struct {
	mu          sync.Mutex
	active, max int
	ids         map[string]bool
	overtook    bool
}

func (s *capacitySender) Send(ctx context.Context, d domain.NotificationDelivery) error {
	s.mu.Lock()
	if s.ids[d.ID] {
		s.mu.Unlock()
		return fmt.Errorf("duplicate send")
	}
	s.ids[d.ID] = true
	s.active++
	if s.active > s.max {
		s.max = s.active
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock() }()
	delay := 5 * time.Millisecond
	if d.UserID == 1 {
		delay = 100 * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	s.mu.Lock()
	if d.UserID > 1 && s.active > 1 {
		s.overtook = true
	}
	s.mu.Unlock()
	return nil
}
func TestReminderDispatch500BoundedConcurrentAndSlowSender(t *testing.T) {
	f := &dispatchFixture{remaining: 500, finished: map[string]string{}}
	sender := &capacitySender{ids: map[string]bool{}}
	service := &Service{Store: f, Notifier: sender, NotificationTemplateID: "template"}
	start := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := service.dispatchReminders(ctx, 8, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(f.finished) != 500 || len(sender.ids) != 500 || sender.max > 8 || sender.max < 2 || !sender.overtook {
		t.Fatalf("finished=%d sent=%d max=%d overtook=%v", len(f.finished), len(sender.ids), sender.max, sender.overtook)
	}
	t.Logf("500 simulated sends, max concurrency %d, accelerated 1ms pacing: %s", sender.max, time.Since(start))
}

func TestReminderDispatchSharesEmptyQueueResultAndSkipsIneligibleBatches(t *testing.T) {
	for _, deliveries := range []int{0, 1} {
		t.Run(fmt.Sprintf("deliveries-%d", deliveries), func(t *testing.T) {
			f := &dispatchFixture{remaining: deliveries, skippedBatches: deliveries * 2, finished: map[string]string{}}
			sender := &capacitySender{ids: map[string]bool{}}
			service := &Service{Store: f, Notifier: sender, NotificationTemplateID: "template"}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if err := service.dispatchReminders(ctx, 8, time.Millisecond); err != nil {
				t.Fatal(err)
			}
			if f.claimCalls != deliveries*3+1 || len(f.finished) != deliveries || len(sender.ids) != deliveries {
				t.Fatalf("claims=%d finished=%d sent=%d", f.claimCalls, len(f.finished), len(sender.ids))
			}
		})
	}
}
