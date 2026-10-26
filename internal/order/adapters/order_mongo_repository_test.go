package adapters

import (
	"context"
	"errors"
	"testing"

	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestOrderRepositoryMongoMarkPaidUsesConditionalUpdate(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("one waiting order is changed", func(mt *mtest.T) {
		previousDBName, previousCollName := dbName, collName
		dbName, collName = mt.DB.Name(), mt.Coll.Name()
		defer func() {
			dbName, collName = previousDBName, previousCollName
		}()

		mt.AddMockResponses(mtest.CreateSuccessResponse(
			bson.E{Key: "n", Value: int32(1)},
			bson.E{Key: "nModified", Value: int32(1)},
		))

		orderID := primitive.NewObjectID()
		repo := NewOrderRepositoryMongo(mt.Client)
		changed, err := repo.MarkPaid(
			context.Background(),
			orderID.Hex(),
			"customer-1",
			"payment-link-1",
		)
		require.NoError(t, err)
		require.True(t, changed)

		started := mt.GetStartedEvent()
		require.NotNil(t, started)
		require.Equal(t, "update", started.CommandName)
		updates, err := started.Command.Lookup("updates").Array().Values()
		require.NoError(t, err)
		require.Len(t, updates, 1)
		filter := updates[0].Document().Lookup("q").Document()
		require.Equal(t, orderID, filter.Lookup("_id").ObjectID())
		require.Equal(t, "customer-1", filter.Lookup("customer_id").StringValue())
		require.Equal(t, domain.StatusWaitingForPayment, filter.Lookup("status").StringValue())
	})
}

func TestOrderRepositoryMongoMarkPaidDoesNotOverwriteLinkWithEmptyValue(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty event link is omitted from update", func(mt *mtest.T) {
		previousDBName, previousCollName := dbName, collName
		dbName, collName = mt.DB.Name(), mt.Coll.Name()
		defer func() {
			dbName, collName = previousDBName, previousCollName
		}()

		mt.AddMockResponses(mtest.CreateSuccessResponse(
			bson.E{Key: "n", Value: int32(1)},
			bson.E{Key: "nModified", Value: int32(1)},
		))

		repo := NewOrderRepositoryMongo(mt.Client)
		changed, err := repo.MarkPaid(
			context.Background(),
			primitive.NewObjectID().Hex(),
			"customer-1",
			"",
		)
		require.NoError(t, err)
		require.True(t, changed)

		started := mt.GetStartedEvent()
		require.NotNil(t, started)
		updates, err := started.Command.Lookup("updates").Array().Values()
		require.NoError(t, err)
		set := updates[0].Document().Lookup("u").Document().Lookup("$set").Document()
		require.Equal(t, domain.StatusPaid, set.Lookup("status").StringValue())
		_, err = set.LookupErr("payment_link")
		require.Error(t, err)
	})
}

func TestOrderRepositoryMongoMarkPaidClassifiesNoMatch(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	orderID := primitive.NewObjectID()

	tests := []struct {
		name              string
		lookupDocuments   []bson.D
		wantDuplicate     bool
		wantNotFound      bool
		wantInvalidStatus bool
	}{
		{
			name: "already paid is a successful duplicate",
			lookupDocuments: []bson.D{{
				{Key: "_id", Value: orderID},
				{Key: "customer_id", Value: "customer-1"},
				{Key: "status", Value: domain.StatusPaid},
				{Key: "payment_link", Value: "first-link"},
			}},
			wantDuplicate: true,
		},
		{
			name:         "missing or wrong customer remains hidden",
			wantNotFound: true,
		},
		{
			name: "other state is rejected",
			lookupDocuments: []bson.D{{
				{Key: "_id", Value: orderID},
				{Key: "customer_id", Value: "customer-1"},
				{Key: "status", Value: "cancelled"},
			}},
			wantInvalidStatus: true,
		},
	}

	for _, tt := range tests {
		mt.Run(tt.name, func(mt *mtest.T) {
			previousDBName, previousCollName := dbName, collName
			dbName, collName = mt.DB.Name(), mt.Coll.Name()
			defer func() {
				dbName, collName = previousDBName, previousCollName
			}()

			responses := []bson.D{mtest.CreateSuccessResponse(
				bson.E{Key: "n", Value: int32(0)},
				bson.E{Key: "nModified", Value: int32(0)},
			)}
			responses = append(responses, mtest.CreateCursorResponse(
				0,
				mt.DB.Name()+"."+mt.Coll.Name(),
				mtest.FirstBatch,
				tt.lookupDocuments...,
			))
			mt.AddMockResponses(responses...)

			repo := NewOrderRepositoryMongo(mt.Client)
			changed, err := repo.MarkPaid(
				context.Background(),
				orderID.Hex(),
				"customer-1",
				"replayed-link",
			)
			require.False(t, changed)
			switch {
			case tt.wantDuplicate:
				require.NoError(t, err)
			case tt.wantNotFound:
				var target domain.NotFoundError
				require.True(t, errors.As(err, &target))
			case tt.wantInvalidStatus:
				var target domain.InvalidStatusTransitionError
				require.True(t, errors.As(err, &target))
			}

			updateEvent := mt.GetStartedEvent()
			require.NotNil(t, updateEvent)
			findEvent := mt.GetStartedEvent()
			require.NotNil(t, findEvent)
			require.Equal(t, "find", findEvent.CommandName)
			filter := findEvent.Command.Lookup("filter").Document()
			require.Equal(t, orderID, filter.Lookup("_id").ObjectID())
			require.Equal(t, "customer-1", filter.Lookup("customer_id").StringValue())
		})
	}
}
