package caller

import (
	"context"
	"floolishman/grpc/service"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"sync"
	"time"
)

var Loc *time.Location

type SeasonType string

var (
	SeasonTypeReverse SeasonType = "REVERSE"
	SeasonTypeTimeout SeasonType = "TIMEOUT"
)

var (
	CancelLimitDuration   time.Duration = 60
	CheckCloseInterval    time.Duration = 500
	CheckLeverageInterval time.Duration = 1000
	CheckTimeoutInterval  time.Duration = 500
	CheckStrategyInterval time.Duration = 2
)

func init() {
	Loc, _ = time.LoadLocation("Asia/Shanghai")
}

type CallerSetting struct {
	checkMode    string
	followSymbol bool
	backtest     bool
}

type CallerBaseInterface interface {
	Start(map[string]model.PairOption, CallerSetting)
	Listen()
}

type CallerBase struct {
	ctx         context.Context
	mu          sync.Mutex
	strategy    types.CompositesStrategy
	broker      reference.Broker
	exchange    reference.Exchange
	guider      *service.ServiceGuider
	pairOptions map[string]model.PairOption
	pairPrices  map[string]float64
	lastUpdate  time.Time
	setting     CallerSetting
}

func NewCaller(ctx context.Context, strategy types.CompositesStrategy, broker reference.Broker) {

}

func (s *CallerBase) tickCheckOrderTimeout() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			s.checkOrderTimeout()
		}
	}
}

func (bc *CallerBase) checkOrderTimeout() {
	bc.mu.Lock()         // 加锁
	defer bc.mu.Unlock() // 解锁
	existOrderMap, err := bc.broker.GetOrdersForUnfilled()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for orderFlag, existOrders := range existOrderMap {
		positionOrders, ok := existOrders["position"]
		if !ok {
			continue
		}
		for _, positionOrder := range positionOrders {
			// 获取当前时间使用
			currentTime := time.Now()
			if bc.setting.checkMode == "candle" {
				currentTime = bc.lastUpdate
			}
			// 获取挂单时间是否超长
			cancelLimitTime := positionOrder.UpdatedAt.Add(CancelLimitDuration * time.Second)
			// 判断当前时间是否在cancelLimitTime之前,在取消时间之前则不取消,防止挂单后被立马取消
			if currentTime.Before(cancelLimitTime) {
				continue
			}
			// 取消之前的未成交的限价单
			err = bc.broker.Cancel(*positionOrder)
			if err != nil {
				utils.Log.Error(err)
				continue
			}
			utils.Log.Infof(
				"[ORDER - %s] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v | Create: %s",
				SeasonTypeTimeout,
				positionOrder.OrderFlag,
				positionOrder.Pair,
				positionOrder.PositionSide,
				positionOrder.Quantity,
				positionOrder.Price,
				positionOrder.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
			)
			// 取消之前的止损单
			lossLimitOrders, ok := existOrderMap[orderFlag]["lossLimit"]
			if !ok {
				continue
			}
			for _, lossLimitOrder := range lossLimitOrders {
				// 取消之前的止损单
				err = bc.broker.Cancel(*lossLimitOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
	}
}
