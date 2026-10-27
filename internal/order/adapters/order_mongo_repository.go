package adapters

import (
	"context"
	"errors"
	"fmt"
	_ "github.com/jiahuipaung/gorder/common/config"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

var (
	dbName   = viper.Sub("mongo").GetString("db-name")
	collName = viper.Sub("mongo").GetString("coll-name")
)

type OrderRepositoryMongo struct {
	db *mongo.Client
}

func NewOrderRepositoryMongo(db *mongo.Client) *OrderRepositoryMongo {
	return &OrderRepositoryMongo{db: db}
}

func (r *OrderRepositoryMongo) collection() *mongo.Collection {
	return r.db.Database(dbName).Collection(collName)
}

type orderModel struct {
	MongoID         primitive.ObjectID `bson:"_id"`
	ID              string             `bson:"id"`
	CustomerID      string             `bson:"customer_id"`
	Status          string             `bson:"status"`
	PaymentLink     string             `bson:"payment_link"`
	Items           []*entity.Item     `bson:"items"`
	ExpiresAt       time.Time          `bson:"expires_at"`
	CreatePublished bool               `bson:"create_published"`
}

func (r *OrderRepositoryMongo) Create(ctx context.Context, order *domain.Order) (created *domain.Order, err error) {
	defer r.logWithTag("create", err, order, created)
	write := r.marshalToModel(order)
	res, err := r.collection().InsertOne(ctx, write)
	if err != nil {
		return nil, err
	}
	created = order
	created.ID = res.InsertedID.(primitive.ObjectID).Hex()
	return
}

func (r *OrderRepositoryMongo) Get(ctx context.Context, id, customerID string) (got *domain.Order, err error) {
	defer r.logWithTag("get", err, nil, got)
	read := &orderModel{}
	mongoID, parseErr := primitive.ObjectIDFromHex(id)
	if parseErr != nil {
		return nil, domain.NotFoundError{OrderID: id}
	}
	condition := bson.M{"_id": mongoID, "customer_id": customerID}
	if err = r.collection().FindOne(ctx, condition).Decode(read); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.NotFoundError{OrderID: id}
		}
		return
	}
	got = r.unmarshal(read)
	return
}

// ListPending pages through durable order intents that still need reservation
// or confirmed publication. Expired intents require payment reconciliation.
func (r *OrderRepositoryMongo) ListPending(ctx context.Context, after string, limit int) ([]*domain.Order, error) {
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("invalid pending page limit")
	}
	filter := bson.M{"status": domain.StatusPending, "expires_at": bson.M{"$gt": time.Now().UTC()}, "create_published": bson.M{"$ne": true}}
	if after != "" {
		id, err := primitive.ObjectIDFromHex(after)
		if err != nil {
			return nil, err
		}
		filter["_id"] = bson.M{"$gt": id}
	}
	cursor, err := r.collection().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var result []*domain.Order
	for cursor.Next(ctx) {
		var row orderModel
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result = append(result, r.unmarshal(&row))
	}
	return result, cursor.Err()
}

func (r *OrderRepositoryMongo) MarkCreatePublished(ctx context.Context, id, customerID string) error {
	mongoID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.NotFoundError{OrderID: id}
	}
	result, err := r.collection().UpdateOne(ctx, bson.M{"_id": mongoID, "customer_id": customerID}, bson.M{"$set": bson.M{"create_published": true}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return domain.NotFoundError{OrderID: id}
	}
	return nil
}

func (r *OrderRepositoryMongo) RejectStock(ctx context.Context, id, customerID string) error {
	mongoID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return domain.NotFoundError{OrderID: id}
	}
	result, err := r.collection().UpdateOne(ctx, bson.M{"_id": mongoID, "customer_id": customerID, "status": domain.StatusPending}, bson.M{"$set": bson.M{"status": domain.StatusStockRejected}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 1 {
		return nil
	}
	current, err := r.Get(ctx, id, customerID)
	if err != nil {
		return err
	}
	if current.Status == domain.StatusStockRejected {
		return nil
	}
	return domain.InvalidStatusTransitionError{OrderID: id, CurrentStatus: current.Status, TargetStatus: domain.StatusStockRejected}
}

func (r *OrderRepositoryMongo) AttachPaymentSession(
	ctx context.Context,
	orderID, customerID, paymentLink string,
) (bool, error) {
	return r.transitionStatus(
		ctx,
		orderID,
		customerID,
		domain.StatusPending,
		domain.StatusWaitingForPayment,
		bson.M{
			"status":       domain.StatusWaitingForPayment,
			"payment_link": paymentLink,
		},
		func(current *domain.Order) bool {
			return current.Status == domain.StatusWaitingForPayment &&
				current.PaymentLink == paymentLink
		},
	)
}

func (r *OrderRepositoryMongo) MarkPaid(
	ctx context.Context,
	orderID, customerID, paymentLink string,
) (bool, error) {
	updates := bson.M{"status": domain.StatusPaid}
	if paymentLink != "" {
		updates["payment_link"] = paymentLink
	}
	return r.transitionStatus(
		ctx,
		orderID,
		customerID,
		domain.StatusWaitingForPayment,
		domain.StatusPaid,
		updates,
		func(current *domain.Order) bool {
			return current.Status == domain.StatusPaid || current.Status == domain.StatusReady
		},
	)
}

func (r *OrderRepositoryMongo) MarkReady(
	ctx context.Context,
	orderID, customerID string,
) (bool, error) {
	return r.transitionStatus(
		ctx,
		orderID,
		customerID,
		domain.StatusPaid,
		domain.StatusReady,
		bson.M{"status": domain.StatusReady},
		func(current *domain.Order) bool { return current.Status == domain.StatusReady },
	)
}

func (r *OrderRepositoryMongo) transitionStatus(
	ctx context.Context,
	orderID, customerID, expectedStatus, targetStatus string,
	updates bson.M,
	isReplay func(*domain.Order) bool,
) (bool, error) {
	mongoID, err := primitive.ObjectIDFromHex(orderID)
	if err != nil {
		return false, domain.NotFoundError{OrderID: orderID}
	}
	result, err := r.collection().UpdateOne(
		ctx,
		bson.M{
			"_id":         mongoID,
			"customer_id": customerID,
			"status":      expectedStatus,
		},
		bson.M{"$set": updates},
	)
	if err != nil {
		return false, err
	}
	if result.ModifiedCount == 1 {
		return true, nil
	}

	current, err := r.Get(ctx, orderID, customerID)
	if err != nil {
		return false, err
	}
	if isReplay(current) {
		return false, nil
	}
	return false, domain.InvalidStatusTransitionError{
		OrderID:       orderID,
		CurrentStatus: current.Status,
		TargetStatus:  targetStatus,
	}
}

func (r *OrderRepositoryMongo) marshalToModel(order *domain.Order) *orderModel {
	return &orderModel{
		MongoID:     primitive.NewObjectID(),
		ID:          order.ID,
		CustomerID:  order.CustomerID,
		Status:      order.Status,
		PaymentLink: order.PaymentLink,
		Items:       order.Items,
		ExpiresAt:   order.ExpiresAt,
	}
}

func (r *OrderRepositoryMongo) unmarshal(model *orderModel) *domain.Order {
	return &domain.Order{
		ID:          model.MongoID.Hex(),
		CustomerID:  model.CustomerID,
		Status:      model.Status,
		PaymentLink: model.PaymentLink,
		Items:       model.Items,
		ExpiresAt:   model.ExpiresAt,
	}
}

func (r *OrderRepositoryMongo) logWithTag(tag string, err error, input *domain.Order, result interface{}) {
	l := logrus.WithFields(logrus.Fields{
		"tag":         "order_repository_mongo",
		"input_order": input,
		"exec_time":   time.Now().Unix(),
		"err":         err,
		"result":      result,
	})
	if err != nil {
		l.Infof("%s fail", tag)
	} else {
		l.Infof("%s success", tag)
	}
}
