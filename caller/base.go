package caller

import (
	"context"
	"errors"
	"floolishman/grpc/service"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"github.com/jpillora/backoff"
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
	StepMoreRatio        = 0.08
	ProfitMoreHedgeRatio = 0.04
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
	"dual":      &CallerDual{},
	"grid":      &CallerGrid{},
}

type CallerBase struct {
	ctx               context.Context
	mu                sync.Mutex
	strategy          types.CompositesStrategy
	setting           types.CallerSetting
	ba                *backoff.Backoff
	broker            reference.Broker
	exchange          reference.Exchange
	guider            *service.ServiceGuider
	pairOptions       map[string]model.PairOption
	samples           map[string]map[string]map[string]*model.Dataframe
	profitRatioLimit  map[string]float64
	pairPrices        map[string]float64
	lastUpdate        map[string]time.Time
	lossLimitTimes    map[string]time.Time
	positionJudgers   map[string]*PositionJudger
	positionGridMap   map[string]*model.PositionGrid
	positionGridIndex map[string]int
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
	c.positionGridMap = make(map[string]*model.PositionGrid)
	c.positionGridIndex = make(map[string]int)
	c.guider = service.NewServiceGuider(ctx, setting.GuiderHost)
	c.ba = &backoff.Backoff{
		Min: 100 * time.Millisecond,
		Max: 1 * time.Second,
	}
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

func (c *CallerBase) CheckHasUnfilledPositionOrders(pair string, side model.SideType, positionSide model.PositionSideType) (bool, error) {
	// 判断当前是否已有同向挂单未成交，有则不在开单
	existUnfilledOrderMap, err := c.broker.GetOrdersForPairUnfilled(pair)
	if err != nil {
		return false, err
	}
	var exsitOrder *model.Order
	for _, existUnfilledOrder := range existUnfilledOrderMap {
		positionOrders, ok := existUnfilledOrder["position"]
		if !ok {
			continue
		}
		for _, positionOrder := range positionOrders {
			// 判断当前是否有同向挂单
			if positionOrder.Side == side && positionOrder.PositionSide == positionSide {
				exsitOrder = positionOrder
				break
			}
		}
	}
	// 判断当前是否已有同向挂单
	if exsitOrder != nil {
		utils.Log.Infof(
			"[POSITION - EXSIT] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v | (UNFILLED)",
			exsitOrder.OrderFlag,
			exsitOrder.Pair,
			exsitOrder.PositionSide,
			exsitOrder.Quantity,
			exsitOrder.Price,
			c.pairPrices[exsitOrder.Pair],
		)
		return true, nil
	}
	return false, nil
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

			// 取消订单时将该订单锁定的网格重置回去
			if _, ok := c.positionGridMap[positionOrder.Pair]; ok {
				c.positionGridMap[positionOrder.Pair].GridItems[c.positionGridIndex[positionOrder.Pair]].Lock = false
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

func (c *CallerBase) finishAllPosition(mainPosition *model.Position, subPosition *model.Position) {
	// 批量下单
	orderParams := []*model.OrderParam{}
	// 平掉副仓位
	if subPosition.Quantity > 0 {
		orderParams = append(orderParams, &model.OrderParam{
			Side:         model.SideType(mainPosition.Side),
			PositionSide: model.PositionSideType(subPosition.PositionSide),
			Pair:         subPosition.Pair,
			Quantity:     subPosition.Quantity,
			Extra: model.OrderExtra{
				Leverage:  subPosition.Leverage,
				OrderFlag: subPosition.OrderFlag,
			},
		})
	}
	// 平掉主仓位
	if mainPosition.Quantity > 0 {
		orderParams = append(orderParams, &model.OrderParam{
			Side:         model.SideType(subPosition.Side),
			PositionSide: model.PositionSideType(mainPosition.PositionSide),
			Pair:         mainPosition.Pair,
			Quantity:     mainPosition.Quantity,
			Extra: model.OrderExtra{
				Leverage:  mainPosition.Leverage,
				OrderFlag: mainPosition.OrderFlag,
			},
		})
	}
	_, err := c.broker.BatchCreateOrderMarket(orderParams)
	if err != nil {
		utils.Log.Error(err)
	}
}

func (c *CallerBase) BuildGird(pair string, timeframe string, isForce bool) {
	if isForce == false {
		if _, ok := c.positionGridMap[pair]; ok {
			return
		}
	}
	// 获取当前已存在的仓位,保持原有网格
	openedPositions, err := c.broker.GetPositionsForPair(pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) > 0 {
		utils.Log.Infof("[GRID] Build - Position has exsit, wating...")
		return
	}

	dataframe := c.samples[pair][timeframe]["Grid1h"]
	if len(dataframe.Close) == 0 {
		return
	}

	// 获取当前价格区间的中间位置作为网格基线
	//midPrice := c.pairPrices[pair]
	//midPrice := dataframe.Metadata["bb_middle"].Last(1)
	//midPrice := dataframe.Close.Last(1)
	midPrice := dataframe.Metadata["ema8"].Last(1)
	bbUpper := dataframe.Metadata["bb_upper"].Last(1)
	bbLower := dataframe.Metadata["bb_lower"].Last(1)
	bbWidth := dataframe.Metadata["bb_width"].Last(1)
	avgVolume := dataframe.Metadata["avgVolume"].Last(0)
	volume := dataframe.Metadata["volume"].Last(0)
	// 上一根蜡烛线已经破线，不在初始化网格
	if c.pairPrices[pair] > bbUpper || c.pairPrices[pair] < bbLower {
		utils.Log.Infof("[Grid] Build - Bolling has cross limit, wating...")
		delete(c.positionGridMap, pair)
		return
	}
	if (volume / avgVolume) > 1.6 {
		utils.Log.Infof("[GRID] Build - Volume bigger than avgVolume, wating...")
		delete(c.positionGridMap, pair)
		return
	}
	// 计算网格数量
	numGrids := bbWidth / c.pairOptions[pair].GridStep
	if numGrids <= 0 {
		utils.Log.Infof("[GRID] Build - Grid spacing too large for the given price width, wating...")
		return
	}

	if _, ok := c.positionGridMap[pair]; ok {
		if c.positionGridMap[pair].BasePrice == midPrice {
			utils.Log.Infof("[GRID] Build - Grid has no change, wating...")
			return
		}
	}

	// 初始化网格
	grid := model.PositionGrid{
		BasePrice:     midPrice,
		CreatedAt:     time.Now(),
		CountGrid:     int64(numGrids),
		BoundaryUpper: bbUpper,
		BoundaryLower: bbLower,
		GridItems:     []model.PositionGridItem{},
	}

	var longPrice, shortPrice float64
	halfGridStep := c.pairOptions[pair].GridStep / 2
	// 计算网格上下限
	for i := 1; i <= int(numGrids/2); i++ {
		if i == 1 {
			continue
		}
		longPrice = midPrice + float64(i)*c.pairOptions[pair].GridStep + halfGridStep
		if longPrice > grid.BoundaryUpper {
			break
		}
		grid.GridItems = append(grid.GridItems, model.PositionGridItem{
			Side:         model.SideTypeSell,
			PositionSide: model.PositionSideTypeShort,
			Price:        longPrice,
			Lock:         false,
		})
	}
	for i := 1; i <= int(numGrids/2); i++ {
		if i == 1 {
			continue
		}
		// 下方网格
		shortPrice = midPrice - float64(i)*c.pairOptions[pair].GridStep - halfGridStep
		if shortPrice < grid.BoundaryLower {
			break
		}
		grid.GridItems = append(grid.GridItems, model.PositionGridItem{
			Side:         model.SideTypeBuy,
			PositionSide: model.PositionSideTypeLong,
			Price:        shortPrice,
			Lock:         false,
		})
	}
	utils.Log.Infof(
		"[GRID] Build - BasePrice: %v | Upper: %v | Lower: %v | Count: %v | CreatedAt: %s",
		grid.BasePrice,
		grid.BoundaryUpper,
		grid.BoundaryLower,
		grid.CountGrid,
		grid.CreatedAt.In(Loc).Format("2006-01-02 15:04:05"),
	)
	grid.SortGridItemsByPrice(true)
	// 将网格添加到网格映射中
	c.positionGridMap[pair] = &grid
}

func (c *CallerBase) ResetGrid(pair string) {
	if _, ok := c.positionGridMap[pair]; !ok {
		return
	}
	if len(c.positionGridMap[pair].GridItems) == 0 {
		return
	}
	// 修改网格锁定状态
	for i := range c.positionGridMap[pair].GridItems {
		c.positionGridMap[pair].GridItems[i].Lock = false
	}
}

func (c *CallerBase) getOpenGrid(pair string, currentPrice float64) (int, error) {
	openGridIndex := -1
	if currentPrice == c.positionGridMap[pair].BasePrice {
		return openGridIndex, errors.New("Pair price at base line")
	}
	if currentPrice > c.positionGridMap[pair].BasePrice {
		for i, item := range c.positionGridMap[pair].GridItems {
			if item.PositionSide == model.PositionSideTypeLong {
				continue
			}
			if currentPrice < item.Price && item.Lock == false {
				openGridIndex = i
				break
			}
		}
	} else {
		for i := len(c.positionGridMap[pair].GridItems) - 1; i >= 0; i-- {
			item := c.positionGridMap[pair].GridItems[i]
			if item.PositionSide == model.PositionSideTypeShort {
				continue
			}
			if currentPrice > item.Price && item.Lock == false {
				openGridIndex = i
				break
			}
		}
	}
	return openGridIndex, nil
}
