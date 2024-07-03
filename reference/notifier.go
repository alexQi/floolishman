//go:generate go run github.com/vektra/mockery/v2 --all --with-expecter --output=../testdata/mocks

package reference

import (
	"floolishman/model"
)

type Notifier interface {
	Notify(string)
	OnOrder(order model.Order)
	OnError(err error)
}
