//go:build integration

package reservation

import (
	"context"
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, *gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("GORDER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("GORDER_TEST_MYSQL_DSN required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil || !strings.HasSuffix(name, "_test") {
		t.Fatal("test database must end in _test")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Exec("CREATE TABLE IF NOT EXISTS o_stock (product_id VARCHAR(191) COLLATE utf8mb4_bin PRIMARY KEY, quantity BIGINT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("test-%d", time.Now().UnixNano())
	for _, sku := range []string{id + "a", id + "b"} {
		if err := db.Exec("INSERT INTO o_stock(product_id,quantity) VALUES (?, 5)", sku).Error; err != nil {
			t.Fatal(err)
		}
	}
	return s, db, id
}
func available(t *testing.T, db *gorm.DB, sku string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT quantity FROM o_stock WHERE product_id=?", sku).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}
func request(id string, n int32) Request {
	return Request{ID: id, CustomerID: "customer", Items: []Item{{SKU: id + "a", Quantity: n}}, ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)}
}

func TestReserveReplayConflictAndTerminalStates(t *testing.T) {
	s, db, id := testStore(t)
	ctx := context.Background()
	req := request(id, 2)
	for i := 0; i < 2; i++ {
		r, err := s.Reserve(ctx, req)
		if err != nil || r.Status != Reserved {
			t.Fatalf("reserve %v %v", r, err)
		}
	}
	if got := available(t, db, id+"a"); got != 3 {
		t.Fatalf("replay reduced stock: %d", got)
	}
	conflict := req
	conflict.Items = []Item{{SKU: id + "a", Quantity: 3}}
	if _, err := s.Reserve(ctx, conflict); err == nil {
		t.Fatal("conflicting reuse accepted")
	}
	if _, err := s.Confirm(ctx, id, "other-customer"); err == nil {
		t.Fatal("wrong owner confirmed")
	}
	for i := 0; i < 2; i++ {
		if _, err := s.Confirm(ctx, id, "customer"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Release(ctx, id, "customer"); err == nil {
		t.Fatal("confirmed reservation released")
	}
	if available(t, db, id+"a") != 3 {
		t.Fatal("confirm double deducted")
	}
	// A reconstructed store must replay the same durable terminal state.
	r, err := NewStore(db).Reserve(ctx, req)
	if err != nil || r.Status != Confirmed {
		t.Fatalf("lost terminal state: %v %v", r, err)
	}
}

func TestReleaseReturnsStockOnlyOnce(t *testing.T) {
	s, db, id := testStore(t)
	ctx := context.Background()
	req := request(id, 3)
	if _, err := s.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.Release(ctx, id, "customer"); err != nil {
			t.Fatal(err)
		}
	}
	if available(t, db, id+"a") != 5 {
		t.Fatal("release changed total stock")
	}
	if _, err := s.Confirm(ctx, id, "customer"); err == nil {
		t.Fatal("released reservation confirmed")
	}
	r, err := s.Reserve(ctx, req)
	if err != nil || r.Status != Released {
		t.Fatal("replay reacquired released stock")
	}
}

func TestMultiSKURollback(t *testing.T) {
	s, db, id := testStore(t)
	req := request(id, 2)
	req.Items = append(req.Items, Item{SKU: id + "b", Quantity: 6})
	if _, err := s.Reserve(context.Background(), req); err == nil {
		t.Fatal("oversubscribed cart accepted")
	}
	if available(t, db, id+"a") != 5 || available(t, db, id+"b") != 5 {
		t.Fatal("partial reservation committed")
	}
}

func TestConcurrentReservationsCannotOversell(t *testing.T) {
	s, db, id := testStore(t)
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := request(fmt.Sprintf("%s-%d", id, i), 1)
			req.Items = []Item{{SKU: id + "a", Quantity: 1}}
			if _, err := s.Reserve(context.Background(), req); err == nil {
				success.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if success.Load() != 5 || available(t, db, id+"a") != 0 {
		t.Fatalf("success=%d stock=%d", success.Load(), available(t, db, id+"a"))
	}
}

func TestConcurrentConfirmAndReleaseHaveOneWinner(t *testing.T) {
	s, db, id := testStore(t)
	ctx := context.Background()
	if _, err := s.Reserve(ctx, request(id, 2)); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, f := range []func(context.Context, string, string) (*Reservation, error){s.Confirm, s.Release} {
		wg.Add(1)
		go func(f func(context.Context, string, string) (*Reservation, error)) {
			defer wg.Done()
			if _, err := f(ctx, id, "customer"); err == nil {
				successes.Add(1)
			}
		}(f)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("terminal winners=%d", successes.Load())
	}
	r, err := s.Get(ctx, id, "customer")
	if err != nil {
		t.Fatal(err)
	}
	want := int64(3)
	if r.Status == Released {
		want = 5
	}
	if available(t, db, id+"a") != want {
		t.Fatal("inventory does not match terminal state")
	}
}

func TestDueReservationsRemainHeldUntilExplicitRelease(t *testing.T) {
	s, db, id := testStore(t)
	ctx := context.Background()
	req := request(id, 1)
	if _, err := s.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	due, err := s.Due(ctx, req.ExpiresAt.Add(time.Second), 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range due {
		if r.ID == id {
			found = true
		}
	}
	if !found || available(t, db, id+"a") != 4 {
		t.Fatal("due scan must not release stock")
	}
}

func TestReservationIdentityIsCaseSensitive(t *testing.T) {
	s, db, id := testStore(t)
	first := request(id, 1)
	if _, err := s.Reserve(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = strings.ToUpper(id)
	if _, err := s.Reserve(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if got := available(t, db, id+"a"); got != 3 {
		t.Fatalf("distinct reservation IDs collapsed: available=%d", got)
	}
	if _, err := s.Get(context.Background(), id, "CUSTOMER"); err == nil {
		t.Fatal("wrong-case customer matched reservation")
	}
}
