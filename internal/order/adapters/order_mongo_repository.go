package adapters

import (
	"context"
	_ "github.com/jiahuipaung/gorder/common/config"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	MongoID     primitive.ObjectID `bson:"_id"`
	ID          string             `bson:"id"`
	CustomerID  string             `bson:"customer_id"`
	Status      string             `bson:"status"`
	PaymentLink string             `bson:"payment_link"`
	Items       []*entity.Item     `bson:"items"`
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
	mongoID, _ := primitive.ObjectIDFromHex(id)
	condition := bson.M{"_id": mongoID}
	if err = r.collection().FindOne(ctx, condition).Decode(read); err != nil {
		return
	}
	if read == nil {
		return nil, domain.NotFoundError{OrderID: id}
	}
	got = r.unmarshal(read)
	return
}

// Update 先查找对应order，然后apply updateFn，再写入回去
func (r *OrderRepositoryMongo) Update(
	ctx context.Context,
	o *domain.Order,
	updateFn func(context.Context, *domain.Order) (*domain.Order, error)) (err error) {
	defer r.logWithTag("update", err, o, nil)
	if o == nil {
		panic("order is nil")
	}
	// 事务
	session, err := r.db.StartSession()
	if err != nil {
		return
	}
	defer session.EndSession(ctx)

	if err = session.StartTransaction(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = session.AbortTransaction(ctx)
		} else {
			_ = session.CommitTransaction(ctx)
		}
	}()

	// inside transaction
	oldOrder, err := r.Get(ctx, o.ID, o.CustomerID)
	if err != nil {
		return err
	}
	updated, err := updateFn(ctx, o)
	if err != nil {
		return
	}
	logrus.Infof("update||oldOrder=%v||updated=%v", oldOrder, updated)
	mongoID, _ := primitive.ObjectIDFromHex(oldOrder.ID)
	res, err := r.collection().UpdateOne(
		ctx,
		bson.M{"_id": mongoID, "customer_id": oldOrder.CustomerID},
		bson.M{"$set": bson.M{
			"status":       updated.Status,
			"payment_link": updated.PaymentLink,
		}},
	)
	if err != nil {
		return
	}
	r.logWithTag("finish_update", err, o, res)
	return
}

func (r *OrderRepositoryMongo) marshalToModel(order *domain.Order) *orderModel {
	return &orderModel{
		MongoID:     primitive.NewObjectID(),
		ID:          order.ID,
		CustomerID:  order.CustomerID,
		Status:      order.Status,
		PaymentLink: order.PaymentLink,
		Items:       order.Items,
	}
}

func (r *OrderRepositoryMongo) unmarshal(model *orderModel) *domain.Order {
	return &domain.Order{
		ID:          model.MongoID.Hex(),
		CustomerID:  model.CustomerID,
		Status:      model.Status,
		PaymentLink: model.PaymentLink,
		Items:       model.Items,
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
