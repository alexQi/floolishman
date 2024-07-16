package types

type OrderCloseSignal struct {
	Pair      string
	OrderFlag string
}

var OrderCloseChan = make(chan OrderCloseSignal, 100)
