package convertor

import "sync"

var (
	orderConvertor *OrderConverter
	orderOnce      sync.Once
)

var (
	itemConvertor *ItemConverter
	itemOnce      sync.Once
)

var (
	itemWithQuantityConvertor *ItemWithQuantityConverter
	itemWithQuantityOnce      sync.Once
)

func NewOrderConverter() *OrderConverter {
	orderOnce.Do(func() {
		orderConvertor = new(OrderConverter)
	})
	return orderConvertor
}

func NewItemConvertor() *ItemConverter {
	itemOnce.Do(func() {
		itemConvertor = new(ItemConverter)
	})
	return itemConvertor
}

func NewItemWithQuantityConverter() *ItemWithQuantityConverter {
	itemWithQuantityOnce.Do(func() {
		itemWithQuantityConvertor = new(ItemWithQuantityConverter)
	})
	return itemWithQuantityConvertor
}
