package model

import (
	"floolishman/constants"
	"fmt"
	"github.com/adshao/go-binance/v2/futures"
	"log"
	"strings"
)

type PairOption struct {
	Status              bool
	Pair                string
	Leverage            int
	MarginType          futures.MarginType
	MarginMode          constants.MarginMode
	MarginSize          float64
	MaxGridStep         float64
	MinGridStep         float64
	UndulatePriceLimit  float64
	UndulateVolumeLimit float64
	WindowPeriod        float64
	MaxAddPosition      int64
	MinAddPosition      int64
	ProfitableScale     float64
	ProfitableTrigger   float64
	PullMarginLossRatio float64
	MaxMarginRatio      float64
	MaxMarginLossRatio  float64
}

func (o PairOption) String() string {
	return fmt.Sprintf("[STRATEGY] Loading Pair: %s, Leverage: %d, MarginType: %s", o.Pair, o.Leverage, o.MarginType)
}

func BuildPairOption(pair string, valMap map[string]interface{}) PairOption {
	// 检查并处理 status
	status, ok := valMap["status"].(bool)
	if !ok {
		log.Fatalf("Invalid status format for pair %s: %v", pair, valMap["status"])
	}
	// 检查并处理 leverage
	leverageFloat, ok := valMap["leverage"].(float64)
	if !ok {
		log.Fatalf("Invalid leverage format for pair %s: %v", pair, valMap["leverage"])
	}
	// 网格窗口期
	windowPeriod, ok := valMap["windowperiod"].(float64)
	if !ok {
		log.Fatalf("Invalid windowPeriod format for pair %s: %v", pair, valMap["windowPeriod"])
	}
	// 最大网格间隔
	maxGridStep, ok := valMap["maxgridstep"].(float64)
	if !ok {
		log.Fatalf("Invalid maxGridStep format for pair %s: %v", pair, valMap["maxGridStep"])
	}
	// 最小网格间隔
	minGridStep, ok := valMap["mingridstep"].(float64)
	if !ok {
		log.Fatalf("Invalid minGridStep format for pair %s: %v", pair, valMap["minGridStep"])
	}
	// 价格波动限制
	undulatePriceLimit, ok := valMap["undulatepricelimit"].(float64)
	if !ok {
		log.Fatalf("Invalid undulatePriceLimit format for pair %s: %v", pair, valMap["undulatePriceLimit"])
	}
	// 量能波动限制
	undulateVolumeLimit, ok := valMap["undulatevolumelimit"].(float64)
	if !ok {
		log.Fatalf("Invalid undulateVolumeLimit format for pair %s: %v", pair, valMap["undulateVolumeLimit"])
	}
	// 保证金类型
	marginType, ok := valMap["margintype"].(string)
	if !ok {
		log.Fatalf("Invalid marginType format for pair %s", pair)
	}
	// 保证金模式
	marginMode, ok := valMap["marginmode"].(string)
	if !ok {
		log.Fatalf("Invalid marginMode format for pair %s", pair)
	}
	// 保证金大小
	marginSize, ok := valMap["marginsize"].(float64)
	if !ok {
		log.Fatalf("Invalid marginSize format for pair %s: %v", pair, valMap["marginSize"])
	}
	// 最大加仓次数
	maxAddPosition, ok := valMap["maxaddposition"].(int)
	if !ok {
		log.Fatalf("Invalid maxAddPosition format for pair %s: %v", pair, valMap["maxAddPosition"])
	}
	// 最小加仓次数
	minAddPosition, ok := valMap["minaddposition"].(int)
	if !ok {
		log.Fatalf("Invalid minAddPosition format for pair %s: %v", pair, valMap["minAddPosition"])
	}
	// 止盈触发比例
	profitableTrigger, ok := valMap["profitabletrigger"].(float64)
	if !ok {
		log.Fatalf("Invalid profitableTrigger format for pair %s: %v", pair, valMap["profitableTrigger"])
	}
	// 止盈触发回撤比例
	profitableScale, ok := valMap["profitablescale"].(float64)
	if !ok {
		log.Fatalf("Invalid profitableScale format for pair %s: %v", pair, valMap["profitableScale"])
	}
	// 亏损拉回比例
	pullMarginLossRatio, ok := valMap["pullmarginlossratio"].(float64)
	if !ok {
		log.Fatalf("Invalid pullMarginLossRatio format for pair %s: %v", pair, valMap["pullMarginLossRatio"])
	}
	// 止盈触发回撤比例
	maxMarginRatio, ok := valMap["maxmarginratio"].(float64)
	if !ok {
		log.Fatalf("Invalid maxMarginRatio format for pair %s: %v", pair, valMap["maxMarginRatio"])
	}
	// 止盈触发回撤比例
	maxMarginLossRatio, ok := valMap["maxmarginlossratio"].(float64)
	if !ok {
		log.Fatalf("Invalid maxMarginLossRatio format for pair %s: %v", pair, valMap["maxMarginLossRatio"])
	}

	return PairOption{
		Status:              status,
		Pair:                strings.ToUpper(pair),
		Leverage:            int(leverageFloat),
		WindowPeriod:        windowPeriod,
		MaxGridStep:         maxGridStep,
		MinGridStep:         minGridStep,
		UndulatePriceLimit:  undulatePriceLimit,
		UndulateVolumeLimit: undulateVolumeLimit,
		MarginType:          futures.MarginType(strings.ToUpper(marginType)),
		MarginMode:          constants.MarginMode(strings.ToUpper(marginMode)),
		MarginSize:          marginSize,
		MaxAddPosition:      int64(maxAddPosition),
		MinAddPosition:      int64(minAddPosition),
		ProfitableScale:     profitableScale,
		ProfitableTrigger:   profitableTrigger,
		PullMarginLossRatio: pullMarginLossRatio,
		MaxMarginRatio:      maxMarginRatio,
		MaxMarginLossRatio:  maxMarginLossRatio,
	}
}
