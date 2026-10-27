package adapters

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestUnsupportedGetItemsReturnsUnimplemented(t *testing.T) {
	r := NewMySQLStockRepository(nil)
	_, err := r.GetItems(context.Background(), []string{"a"})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("got %v", err)
	}
}
