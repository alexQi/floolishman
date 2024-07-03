//go:generate go run github.com/vektra/mockery/v2 --all --with-expecter --output=../testdata/mocks

package reference

type Exchange interface {
	Broker
	Feeder
}
