package caller

import (
	"context"
	"floolishman/grpc/service"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"reflect"
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

var ConstCallers = map[string]reference.Caller{
	"candle":    &CallerCandle{},
	"interval":  &CallerInterval{},
	"frequency": &CallerFrequency{},
	"watchdog":  &CallerWatchdog{},
}

type CallerBase struct {
	ctx              context.Context
	mu               sync.Mutex
	strategy         types.CompositesStrategy
	setting          types.CallerSetting
	broker           reference.Broker
	exchange         reference.Exchange
	guider           *service.ServiceGuider
	pairOptions      map[string]model.PairOption
	samples          map[string]map[string]map[string]*model.Dataframe
	profitRatioLimit map[string]float64
	pairPrices       map[string]float64
	lastUpdate       map[string]time.Time
	lossLimitTimes   map[string]time.Time
	positionJudgers  map[string]*PositionJudger
}

func NewCaller(
	ctx context.Context,
	strategy types.CompositesStrategy,
	broker reference.Broker,
	exchange reference.Exchange,
	setting types.CallerSetting,
) reference.Caller {
	realCaller := ConstCallers[setting.CheckMode]
	realCaller.Init(ctx, strategy, broker, exchange, setting)
	return realCaller
}

func (c *CallerBase) Init(
	ctx context.Context,
	strategy types.CompositesStrategy,
	broker reference.Broker,
	exchange reference.Exchange,
	setting types.CallerSetting,
) {
	c.ctx = ctx
	c.strategy = strategy
	c.broker = broker
	c.exchange = exchange
	c.setting = setting
	c.pairOptions = make(map[string]model.PairOption)
	c.pairPrices = make(map[string]float64)
	c.lastUpdate = make(map[string]time.Time)
	c.lossLimitTimes = make(map[string]time.Time)
	c.profitRatioLimit = make(map[string]float64)
	c.samples = make(map[string]map[string]map[string]*model.Dataframe)
	c.positionJudgers = make(map[string]*PositionJudger)
	c.guider = service.NewServiceGuider(ctx, setting.GuiderHost)
}

func (c *CallerBase) SetPair(option model.PairOption) {
	c.pairOptions[option.Pair] = option
	c.pairPrices[option.Pair] = 0
	c.profitRatioLimit[option.Pair] = 0
	if c.samples[option.Pair] == nil {
		c.samples[option.Pair] = make(map[string]map[string]*model.Dataframe)
	}
	// 初始化不同时间周期的dataframe 及 samples
	for _, strategy := range c.strategy.Strategies {
		if _, ok := c.samples[option.Pair][strategy.Timeframe()]; !ok {
			c.samples[option.Pair][strategy.Timeframe()] = make(map[string]*model.Dataframe)
		}
		c.samples[option.Pair][strategy.Timeframe()][reflect.TypeOf(strategy).Elem().Name()] = &model.Dataframe{
			Pair:     option.Pair,
			Metadata: make(map[string]model.Series[float64]),
		}
	}
}

func (c *CallerBase) SetSample(pair string, timeframe string, strategyName string, dataframe *model.Dataframe) {
	c.samples[pair][timeframe][strategyName] = dataframe
}

func (c *CallerBase) UpdatePairInfo(pair string, price float64, updatedAt time.Time) {
	c.pairPrices[pair] = price
	c.lastUpdate[pair] = updatedAt
}

func (c *CallerBase) tickCheckOrderTimeout() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			c.CheckOrderTimeout()
		}
	}
}

func (c *CallerBase) CheckOrderTimeout() {
	c.mu.Lock()         // 加锁
	defer c.mu.Unlock() // 解锁
	existOrderMap, err := c.broker.GetOrdersForUnfilled()
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
			if c.setting.CheckMode == "candle" {
				currentTime = c.lastUpdate[positionOrder.Pair]
			}
			// 获取挂单时间是否超长
			cancelLimitTime := positionOrder.UpdatedAt.Add(CancelLimitDuration * time.Second)
			// 判断当前时间是否在cancelLimitTime之前,在取消时间之前则不取消,防止挂单后被立马取消
			if currentTime.Before(cancelLimitTime) {
				continue
			}
			// 取消之前的未成交的限价单
			err = c.broker.Cancel(*positionOrder)
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
				err = c.broker.Cancel(*lossLimitOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
	}
}
