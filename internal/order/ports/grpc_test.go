package ports

import (
	"context"
	"errors"
	"testing"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/command"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type attachPaymentSessionHandlerStub struct {
	got     command.AttachPaymentSession
	changed bool
	err     error
	calls   int
}

func (s *attachPaymentSessionHandlerStub) Handler(
	_ context.Context,
	cmd command.AttachPaymentSession,
) (bool, error) {
	s.calls++
	s.got = cmd
	return s.changed, s.err
}

type markOrderReadyHandlerStub struct {
	got     command.MarkOrderReady
	changed bool
	err     error
	calls   int
}

func (s *markOrderReadyHandlerStub) Handler(
	_ context.Context,
	cmd command.MarkOrderReady,
) (bool, error) {
	s.calls++
	s.got = cmd
	return s.changed, s.err
}

func TestGRPCServerAttachPaymentSessionDispatchesIntent(t *testing.T) {
	t.Parallel()

	handler := &attachPaymentSessionHandlerStub{changed: true}
	server := NewGRPCServer(app.Application{Commands: app.Commands{
		AttachPaymentSession: handler,
	}})

	_, err := server.AttachPaymentSession(context.Background(), &orderpb.AttachPaymentSessionRequest{
		OrderID:     "order-1",
		CustomerID:  "customer-1",
		PaymentLink: "checkout-link-1",
	})
	require.NoError(t, err)
	require.Equal(t, 1, handler.calls)
	require.Equal(t, command.AttachPaymentSession{
		OrderID:     "order-1",
		CustomerID:  "customer-1",
		PaymentLink: "checkout-link-1",
	}, handler.got)
}

func TestGRPCServerMarkOrderReadyDispatchesIntent(t *testing.T) {
	t.Parallel()

	handler := &markOrderReadyHandlerStub{changed: true}
	server := NewGRPCServer(app.Application{Commands: app.Commands{
		MarkOrderReady: handler,
	}})

	_, err := server.MarkOrderReady(context.Background(), &orderpb.MarkOrderReadyRequest{
		OrderID:    "order-1",
		CustomerID: "customer-1",
	})
	require.NoError(t, err)
	require.Equal(t, 1, handler.calls)
	require.Equal(t, command.MarkOrderReady{
		OrderID:    "order-1",
		CustomerID: "customer-1",
	}, handler.got)
}

func TestGRPCServerExplicitCommandsRejectInvalidInputBeforeDispatch(t *testing.T) {
	t.Parallel()

	attach := &attachPaymentSessionHandlerStub{}
	ready := &markOrderReadyHandlerStub{}
	server := NewGRPCServer(app.Application{Commands: app.Commands{
		AttachPaymentSession: attach,
		MarkOrderReady:       ready,
	}})

	_, err := server.AttachPaymentSession(context.Background(), &orderpb.AttachPaymentSessionRequest{
		CustomerID:  "customer-1",
		PaymentLink: "checkout-link-1",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Zero(t, attach.calls)

	_, err = server.MarkOrderReady(context.Background(), &orderpb.MarkOrderReadyRequest{
		OrderID: "order-1",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Zero(t, ready.calls)
}

func TestGRPCServerMapsExplicitCommandErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{
			name: "not found",
			err:  domain.NotFoundError{OrderID: "order-1"},
			want: codes.NotFound,
		},
		{
			name: "invalid transition",
			err: domain.InvalidStatusTransitionError{
				OrderID:       "order-1",
				CurrentStatus: domain.StatusPending,
				TargetStatus:  domain.StatusReady,
			},
			want: codes.FailedPrecondition,
		},
		{
			name: "infrastructure",
			err:  errors.New("mongo unavailable"),
			want: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &attachPaymentSessionHandlerStub{err: tt.err}
			server := NewGRPCServer(app.Application{Commands: app.Commands{
				AttachPaymentSession: handler,
			}})

			_, err := server.AttachPaymentSession(context.Background(), &orderpb.AttachPaymentSessionRequest{
				OrderID:     "order-1",
				CustomerID:  "customer-1",
				PaymentLink: "checkout-link-1",
			})
			require.Equal(t, tt.want, status.Code(err))
		})
	}
}
