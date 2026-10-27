package reservation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	Reserved  = "RESERVED"
	Confirmed = "CONFIRMED"
	Released  = "RELEASED"
)

var (
	ErrConflict     = errors.New("reservation identity conflict")
	ErrInvalid      = errors.New("invalid reservation request")
	ErrInsufficient = errors.New("insufficient stock")
	ErrTransition   = errors.New("invalid reservation transition")
	ErrNotFound     = errors.New("reservation not found")
)

type Item struct {
	SKU      string `json:"sku"`
	Quantity int32  `json:"quantity"`
}

type Request struct {
	ID         string
	CustomerID string
	Items      []Item
	ExpiresAt  time.Time
}

type Reservation struct {
	ID         string
	CustomerID string
	Items      []Item
	Status     string
	ExpiresAt  time.Time
}

type record struct {
	ID         string    `gorm:"primaryKey;type:varbinary(191)"`
	CustomerID string    `gorm:"type:varbinary(191);not null"`
	Digest     string    `gorm:"size:64;not null"`
	ItemsJSON  string    `gorm:"type:json;not null"`
	Status     string    `gorm:"size:16;not null;index:idx_reservation_due,priority:1"`
	ExpiresAt  time.Time `gorm:"not null;index:idx_reservation_due,priority:2"`
}

func (record) TableName() string { return "inventory_reservations" }

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// Migrate requires one row per product in the existing stock table. It never
// rewrites inventory or collapses duplicate products.
func (s *Store) Migrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).Exec(`CREATE TABLE IF NOT EXISTS inventory_reservations (
		id VARBINARY(191) NOT NULL PRIMARY KEY,
		customer_id VARBINARY(191) NOT NULL,
		digest CHAR(64) NOT NULL,
		items_json JSON NOT NULL,
		status VARCHAR(16) NOT NULL,
		expires_at DATETIME(6) NOT NULL,
		INDEX idx_reservation_due (status, expires_at)
	) ENGINE=InnoDB`).Error; err != nil {
		return err
	}
	var idType, ownerType string
	if err := s.db.WithContext(ctx).Raw(`SELECT DATA_TYPE FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'inventory_reservations' AND column_name = 'id'`).Scan(&idType).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT DATA_TYPE FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'inventory_reservations' AND column_name = 'customer_id'`).Scan(&ownerType).Error; err != nil {
		return err
	}
	if idType != "varbinary" || ownerType != "varbinary" {
		return fmt.Errorf("inventory_reservations identity columns require VARBINARY(191); migrate the existing table explicitly")
	}
	var duplicates int64
	if err := s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM (SELECT product_id FROM o_stock GROUP BY product_id HAVING COUNT(*) > 1) AS duplicate_skus").Scan(&duplicates).Error; err != nil {
		return err
	}
	if duplicates != 0 {
		return fmt.Errorf("o_stock contains %d duplicate product ids; resolve before enabling reservations", duplicates)
	}
	var uniqueIndexes int64
	if err := s.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM (
		SELECT index_name FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'o_stock' AND non_unique = 0
		GROUP BY index_name HAVING COUNT(*) = 1 AND MAX(column_name) = 'product_id'
	) AS unique_product_indexes`).Scan(&uniqueIndexes).Error; err != nil {
		return err
	}
	if uniqueIndexes == 0 {
		if err := s.db.WithContext(ctx).Exec("ALTER TABLE o_stock ADD UNIQUE INDEX uq_o_stock_product_id (product_id)").Error; err != nil {
			return fmt.Errorf("enforce unique stock product id: %w", err)
		}
	}
	return nil
}

func normalize(req Request) (Request, string, error) {
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.CustomerID) == "" || len(req.Items) == 0 || req.ExpiresAt.IsZero() {
		return Request{}, "", fmt.Errorf("%w: id, customer, items and expiry are required", ErrInvalid)
	}
	if len(req.ID) > 191 || len(req.CustomerID) > 191 {
		return Request{}, "", fmt.Errorf("%w: id or customer is too long", ErrInvalid)
	}
	counts := make(map[string]int64, len(req.Items))
	for _, item := range req.Items {
		if item.SKU == "" || strings.TrimSpace(item.SKU) != item.SKU || len(item.SKU) > 191 || item.Quantity <= 0 {
			return Request{}, "", fmt.Errorf("%w: item SKU or quantity", ErrInvalid)
		}
		counts[item.SKU] += int64(item.Quantity)
		if counts[item.SKU] > math.MaxInt32 {
			return Request{}, "", fmt.Errorf("%w: quantity overflows int32", ErrInvalid)
		}
	}
	items := make([]Item, 0, len(counts))
	for sku, quantity := range counts {
		items = append(items, Item{SKU: sku, Quantity: int32(quantity)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SKU < items[j].SKU })
	req.Items = items
	req.ExpiresAt = req.ExpiresAt.UTC().Truncate(time.Microsecond)
	bytes, err := json.Marshal(struct {
		CustomerID string
		Items      []Item
		ExpiresAt  time.Time
	}{req.CustomerID, req.Items, req.ExpiresAt})
	if err != nil {
		return Request{}, "", err
	}
	digest := sha256.Sum256(bytes)
	return req, hex.EncodeToString(digest[:]), nil
}

func toReservation(row record) (*Reservation, error) {
	var items []Item
	if err := json.Unmarshal([]byte(row.ItemsJSON), &items); err != nil {
		return nil, fmt.Errorf("decode reservation items: %w", err)
	}
	return &Reservation{ID: row.ID, CustomerID: row.CustomerID, Items: items, Status: row.Status, ExpiresAt: row.ExpiresAt}, nil
}

func (s *Store) Reserve(ctx context.Context, input Request) (*Reservation, error) {
	req, digest, err := normalize(input)
	if err != nil {
		return nil, err
	}
	itemsJSON, err := json.Marshal(req.Items)
	if err != nil {
		return nil, err
	}
	var result *Reservation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidate := record{ID: req.ID, CustomerID: req.CustomerID, Digest: digest, ItemsJSON: string(itemsJSON), Status: "NEW", ExpiresAt: req.ExpiresAt}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		var row record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", req.ID).Error; err != nil {
			return err
		}
		if row.Digest != digest || row.CustomerID != req.CustomerID {
			return ErrConflict
		}
		if row.Status != "NEW" {
			result, err = toReservation(row)
			return err
		}
		if !req.ExpiresAt.After(time.Now().UTC()) {
			return fmt.Errorf("%w: reservation already expired", ErrInvalid)
		}
		for _, item := range req.Items {
			updated := tx.Exec("UPDATE o_stock SET quantity = quantity - ? WHERE product_id = ? AND quantity >= ?", item.Quantity, item.SKU, item.Quantity)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return fmt.Errorf("%w: %s", ErrInsufficient, item.SKU)
			}
		}
		if err := tx.Model(&record{}).Where("id = ?", req.ID).Update("status", Reserved).Error; err != nil {
			return err
		}
		row.Status = Reserved
		result, err = toReservation(row)
		return err
	})
	return result, err
}

func (s *Store) transition(ctx context.Context, id, customer, target string) (*Reservation, error) {
	if id == "" || customer == "" {
		return nil, ErrNotFound
	}
	var result *Reservation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ? AND customer_id = ?", id, customer).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.Status == target {
			var err error
			result, err = toReservation(row)
			return err
		}
		if row.Status != Reserved {
			return ErrTransition
		}
		if target == Released {
			reservation, err := toReservation(row)
			if err != nil {
				return err
			}
			for _, item := range reservation.Items {
				updated := tx.Exec("UPDATE o_stock SET quantity = quantity + ? WHERE product_id = ?", item.Quantity, item.SKU)
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return fmt.Errorf("stock row missing for %s", item.SKU)
				}
			}
		}
		if err := tx.Model(&record{}).Where("id = ?", id).Update("status", target).Error; err != nil {
			return err
		}
		row.Status = target
		var err error
		result, err = toReservation(row)
		return err
	})
	return result, err
}

func (s *Store) Confirm(ctx context.Context, id, customer string) (*Reservation, error) {
	return s.transition(ctx, id, customer, Confirmed)
}

func (s *Store) Release(ctx context.Context, id, customer string) (*Reservation, error) {
	return s.transition(ctx, id, customer, Released)
}

func (s *Store) Get(ctx context.Context, id, customer string) (*Reservation, error) {
	var row record
	err := s.db.WithContext(ctx).First(&row, "id = ? AND customer_id = ?", id, customer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toReservation(row)
}

// Due is advisory. A caller must prove the payment cannot succeed before Release.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Reservation, error) {
	if limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("limit must be between 1 and 1000")
	}
	var rows []record
	if err := s.db.WithContext(ctx).Where("status = ? AND expires_at <= ?", Reserved, now.UTC()).Order("expires_at, id").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Reservation, 0, len(rows))
	for _, row := range rows {
		r, err := toReservation(row)
		if err != nil {
			return nil, err
		}
		result = append(result, *r)
	}
	return result, nil
}
