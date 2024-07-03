package process

import (
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"time"
)

var (
	CheckOpenInterval  = 30
	CheckCloseInterval = 10
)

var ChanOnCandle = make(chan model.Candle, 10000)

func CheckOpenPoistion(borker reference.Broker, callback types.OpenPositionFunc) {
	for {
		select {
		case candle := <-ChanOnCandle:
			callback(candle, borker)
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

func CheckClosePoistion(options map[string]model.PairOption, borker reference.Broker, callback types.ClosePositionFunc) {
	for {
		select {
		// 检查一次仓位
		case <-time.After(time.Duration(CheckCloseInterval) * time.Second):
			for _, option := range options {
				callback(option, borker)
			}
		}
	}
}
