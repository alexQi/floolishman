package caller

import (
	"context"
	"errors"
	"floolishman/constants"
	"floolishman/grpc/service"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"reflect"
	"sync"
	"time"
)

var Loc *time.Location

type SeasonType string

var (
	SeasonTypeProfitBack SeasonType = "PROFIT-BACK"
	SeasonTypeLossMax    SeasonType = "LOSS-MAX"
	SeasonTypeReverse    SeasonType = "REVERSE"
	SeasonTypeTimeout    SeasonType = "TIMEOUT"
)

var (
	MaxPairPositions     = 5
	AvgVolumeLimitRatio  = 1.6
	ChangeRingCount      = 5
	ChangeDiffInterval   = 2
	TendencyAngleLimit   = 8.0
	AmplitudeMinRatio    = 1.0
	AmplitudeMaxRatio    = 3.0
	StepMoreRatio        = 0.08
	ProfitMoreHedgeRatio = 0.04
)

var (
	CancelLimitDuration        time.Duration = 180
	CheckCloseInterval         time.Duration = 500
	CheckLeverageInterval      time.Duration = 1000
	CheckTimeoutInterval       time.Duration = 500
	CheckStrategyInterval      time.Duration = 500
	CHeckPriceUndulateInterval time.Duration = 500
)

func init() {
	Loc, _ = time.LoadLocation("Asia/Shanghai")
}

var ConstCallers = map[string]reference.Caller{
	"candle":    &Candle{},
	"scoop":     &Scoop{},
	"frequency": &Frequency{},
	"watchdog":  &Watchdog{},
	"dual":      &Dual{},
	"grid":      &Grid{},
}

type Base struct {
	ctx             context.Context
	mu              map[string]*sync.Mutex
	strategy        model.CompositesStrategy
	setting         types.CallerSetting
	broker          reference.Broker
	exchange        reference.Exchange
	guider          *service.ServiceGuider
	samples         map[string]map[string]map[string]*model.Dataframe
	positionJudgers map[string]*PositionJudger

	pairOptions map[string]*model.PairOption

	// todo 需要处理为线程安全map
	pairTubeOpen      *model.ThreadSafeMap[string, bool]
	pairGridMap       *model.ThreadSafeMap[string, *model.PositionGrid]
	pairGridMapIndex  *model.ThreadSafeMap[string, int]
	pairHedgeMode     *model.ThreadSafeMap[string, constants.PositionMode]
	pairGirdStatus    *model.ThreadSafeMap[string, constants.GridStatus]
	pairProfitLevels  *model.ThreadSafeMap[string, []*model.StopProfitLevel]
	pairCurrentProfit *model.ThreadSafeMap[string, *model.PairProfit]

	pairPrices            *model.ThreadSafeMap[string, float64]
	pairVolumes           *model.ThreadSafeMap[string, float64]
	lastAvgVolume         *model.ThreadSafeMap[string, float64]
	pairOriginVolumes     *model.ThreadSafeMap[string, *model.RingBuffer]
	pairOriginPrices      *model.ThreadSafeMap[string, *model.RingBuffer]
	pairPriceChangeRatio  *model.ThreadSafeMap[string, float64]
	pairVolumeChangeRatio *model.ThreadSafeMap[string, float64]
	pairVolumeGrowRatio   *model.ThreadSafeMap[string, float64]
	lastUpdate            *model.ThreadSafeMap[string, time.Time]
	lossLimitTimes        *model.ThreadSafeMap[string, time.Time]
}

func NewCaller(
	ctx context.Context,
	strategy model.CompositesStrategy,
	broker reference.Broker,
	exchange reference.Exchange,
	setting types.CallerSetting,
) reference.Caller {
	realCaller := ConstCallers[setting.CheckMode]
	realCaller.Init(ctx, strategy, broker, exchange, setting)
	return realCaller
}

func (c *Base) Init(
	ctx context.Context,
	strategy model.CompositesStrategy,
	broker reference.Broker,
	exchange reference.Exchange,
	setting types.CallerSetting,
) {
	c.ctx = ctx
	c.strategy = strategy
	c.broker = broker
	c.exchange = exchange
	c.setting = setting
	c.pairOptions = make(map[string]*model.PairOption)

	c.samples = make(map[string]map[string]map[string]*model.Dataframe)
	c.positionJudgers = make(map[string]*PositionJudger)
	c.mu = make(map[string]*sync.Mutex)
	// build tsmap
	c.pairTubeOpen = model.NewThreadSafeMap[string, bool]()
	c.lastUpdate = model.NewThreadSafeMap[string, time.Time]()
	c.lossLimitTimes = model.NewThreadSafeMap[string, time.Time]()

	c.pairGridMap = model.NewThreadSafeMap[string, *model.PositionGrid]()
	c.pairGridMapIndex = model.NewThreadSafeMap[string, int]()
	c.pairProfitLevels = model.NewThreadSafeMap[string, []*model.StopProfitLevel]()
	c.pairHedgeMode = model.NewThreadSafeMap[string, constants.PositionMode]()
	c.pairGirdStatus = model.NewThreadSafeMap[string, constants.GridStatus]()
	c.pairCurrentProfit = model.NewThreadSafeMap[string, *model.PairProfit]()

	c.pairPrices = model.NewThreadSafeMap[string, float64]()
	c.pairVolumes = model.NewThreadSafeMap[string, float64]()
	c.pairOriginPrices = model.NewThreadSafeMap[string, *model.RingBuffer]()
	c.pairOriginVolumes = model.NewThreadSafeMap[string, *model.RingBuffer]()
	c.pairPriceChangeRatio = model.NewThreadSafeMap[string, float64]()
	c.pairVolumeChangeRatio = model.NewThreadSafeMap[string, float64]()
	c.pairVolumeGrowRatio = model.NewThreadSafeMap[string, float64]()
	c.lastAvgVolume = model.NewThreadSafeMap[string, float64]()

	if c.setting.CheckMode == "watchdog" {
		c.guider = service.NewServiceGuider(ctx, setting.GuiderHost)
	}

	if c.setting.Backtest == false {
		go c.RegisterPairOption()
		go c.RegisterPairGridBuilder()
		go c.RegisterPairPauser()
	}
}

func (c *Base) OpenTube(pair string) {
	c.pairTubeOpen.Set(pair, true)
}

func (c *Base) SetPair(option model.PairOption) {
	option.ProfitableScale = option.ProfitableScale * float64(option.Leverage)
	option.ProfitableScaleDecrStep = option.ProfitableScaleDecrStep * float64(option.Leverage)
	option.ProfitableTrigger = option.ProfitableTrigger * float64(option.Leverage)
	option.ProfitableTriggerIncrStep = option.ProfitableTriggerIncrStep * float64(option.Leverage)
	option.PullMarginLossRatio = option.PullMarginLossRatio * float64(option.Leverage)
	option.MaxMarginRatio = option.MaxMarginRatio * float64(option.Leverage)
	option.MaxMarginLossRatio = option.MaxMarginLossRatio * float64(option.Leverage)
	c.pairOptions[option.Pair] = &option
	c.mu[option.Pair] = &sync.Mutex{}

	c.pairTubeOpen.Set(option.Pair, false)
	c.pairOriginVolumes.Set(option.Pair, model.NewRingBuffer(ChangeRingCount))
	c.pairOriginPrices.Set(option.Pair, model.NewRingBuffer(ChangeRingCount))
	c.pairPrices.Set(option.Pair, 0)
	c.pairVolumes.Set(option.Pair, 0)
	c.pairPriceChangeRatio.Set(option.Pair, 0)
	c.pairVolumeChangeRatio.Set(option.Pair, 0)
	c.pairVolumeGrowRatio.Set(option.Pair, 0)

	c.pairGridMap.Set(option.Pair, &model.PositionGrid{})
	c.pairGridMapIndex.Set(option.Pair, -1)
	c.pairHedgeMode.Set(option.Pair, constants.PositionModeNormal)
	c.pairGirdStatus.Set(option.Pair, constants.GridStatusInside)
	c.pairCurrentProfit.Set(option.Pair, &model.PairProfit{})
	c.pairProfitLevels.Set(option.Pair, c.generateTriggerSequence(
		option.ProfitableTrigger,
		option.ProfitableTriggerIncrStep,
		option.ProfitableScale,
		option.ProfitableScaleDecrStep,
	))

	c.resetPairProfit(option.Pair)

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

func (c *Base) RegisterPairOption() {
	for {
		select {
		case pairStatus := <-types.PairStatusChan:
			c.pairOptions[pairStatus.Pair].Status = pairStatus.Status
			utils.Log.Infof(
				"[CALLER - SWITCH：%s] Caller Status Changed, new Status: %v",
				pairStatus.Pair,
				pairStatus.Status,
			)
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (c *Base) RegisterPairGridBuilder() {
	for {
		select {
		case buildPairGrid := <-types.PairGridBuilderParamChan:
			c.gridBuilder(buildPairGrid.Pair, buildPairGrid.Timeframe, buildPairGrid.IsForce)
		}
	}
}

func (c *Base) RegisterPairPauser() {
	for {
		select {
		case pair := <-types.PairPauserChan:
			c.PausePairCall(pair, time.Duration(c.pairOptions[pair].PauseCaller))
		}
	}
}

func (c *Base) PausePairCall(pair string, minutes time.Duration) {
	if c.pairOptions[pair].Status == false {
		return
	}
	utils.Log.Infof(
		"[CALLER - PAUSE：%s] Caller paused, will be resume at %v mins",
		pair,
		minutes,
	)
	c.pairOptions[pair].Status = false
	time.AfterFunc(minutes*time.Minute, func() {
		c.pairOptions[pair].Status = true
	})
}

func (c *Base) SetSample(pair string, timeframe string, strategyName string, dataframe *model.Dataframe) {
	c.samples[pair][timeframe][strategyName] = dataframe
}

func (c *Base) UpdatePairInfo(pair string, price float64, volume float64, updatedAt time.Time) {
	c.pairPrices.Set(pair, price)
	c.pairVolumes.Set(pair, volume)
	c.lastUpdate.Set(pair, updatedAt)
}

func (c *Base) tickCheckOrderTimeout() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			c.CloseOrder(true)
		}
	}
}

func (c *Base) ListenIndicator() {
	for {
		select {
		case <-time.After(CHeckPriceUndulateInterval * time.Millisecond):
			for _, option := range c.pairOptions {
				pairPrice, _ := c.pairPrices.Get(option.Pair)
				pairVolume, _ := c.pairVolumes.Get(option.Pair)
				pairOriginPrices, _ := c.pairOriginPrices.Get(option.Pair)
				pairOriginVolumes, _ := c.pairOriginVolumes.Get(option.Pair)
				// 记录循环价格数组
				pairOriginPrices.Add(pairPrice)
				pairOriginVolumes.Add(pairVolume)
				// 重设数据
				c.pairOriginPrices.Set(option.Pair, pairOriginPrices)
				c.pairOriginVolumes.Set(option.Pair, pairOriginVolumes)
				// 判断数据是否足够
				if pairOriginPrices.Count() < ChangeRingCount || pairOriginVolumes.Count() < ChangeRingCount {
					continue
				}
				// 本次量能小于上次量能，处理蜡烛收线时量能倍重置
				if pairOriginVolumes.Last(0) < pairOriginVolumes.Last(1) {
					pairOriginVolumes.Clear()
					c.pairOriginVolumes.Set(option.Pair, pairOriginVolumes)
					continue
				}
				// 计算量能异动诧异
				currDiffVolume := pairOriginVolumes.Last(0) - pairOriginVolumes.Last(ChangeDiffInterval)
				prevDiffVolume := pairOriginVolumes.Last(ChangeDiffInterval) - pairOriginVolumes.Last(2*ChangeDiffInterval)
				// 计算价格差异
				currDiffPrice := pairOriginPrices.Last(0) - pairOriginPrices.Last(ChangeDiffInterval)
				// 处理价格变化率
				c.pairPriceChangeRatio.Set(option.Pair, currDiffPrice/(option.UndulatePriceLimit*float64(ChangeDiffInterval)))
				// 处理量能变化率
				c.pairVolumeChangeRatio.Set(option.Pair, currDiffVolume/(option.UndulateVolumeLimit*float64(ChangeDiffInterval)))
				// 判断当前量能差有没有达到最小限制
				if currDiffVolume > (option.UndulateVolumeLimit * float64(ChangeDiffInterval)) {
					// 处理量能每秒增长率
					c.pairVolumeGrowRatio.Set(option.Pair, currDiffVolume/prevDiffVolume)
				} else {
					c.pairVolumeGrowRatio.Set(option.Pair, 0)
				}
			}
		}
	}
}

func (c *Base) CheckHasUnfilledPositionOrders(pair string, side model.SideType, positionSide model.PositionSideType) (bool, error) {
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
	pairPrice, _ := c.pairPrices.Get(pair)
	// 判断当前是否已有同向挂单
	if exsitOrder != nil {
		utils.Log.Infof(
			"[POSITION - EXSIT] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v | (UNFILLED)",
			exsitOrder.OrderFlag,
			exsitOrder.Pair,
			exsitOrder.PositionSide,
			exsitOrder.Quantity,
			exsitOrder.Price,
			pairPrice,
		)
		return true, nil
	}
	return false, nil
}

func (c *Base) CloseOrder(checkTimeout bool) {
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
			// 检查时间超时
			if checkTimeout {
				// 获取当前时间使用
				currentTime := time.Now()
				if c.setting.CheckMode == "candle" {
					currentTime, _ = c.lastUpdate.Get(positionOrder.Pair)
				}
				// 获取挂单时间是否超长
				cancelLimitTime := positionOrder.UpdatedAt.Add(CancelLimitDuration * time.Second)
				// 判断当前时间是否在cancelLimitTime之前,在取消时间之前则不取消,防止挂单后被立马取消
				if currentTime.Before(cancelLimitTime) {
					continue
				}
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
			// 取消订单时将该订单锁定的网格重置回去
			if c.pairGridMap.Exists(positionOrder.Pair) {
				currentGrid, currentExsit := c.pairGridMap.Get(positionOrder.Pair)
				gridIndex, indexExsit := c.pairGridMapIndex.Get(positionOrder.Pair)
				if indexExsit && gridIndex >= 0 && currentExsit {
					currentGrid.GridItems[gridIndex].Lock = false
					c.pairGridMap.Set(positionOrder.Pair, currentGrid)
					c.pairGridMapIndex.Set(positionOrder.Pair, -1)
				}
			}

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

func (c *Base) finishAllPosition(mainPosition *model.Position, subPosition *model.Position) {
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
	if len(orderParams) == 0 {
		return
	}
	_, err := c.broker.BatchCreateOrderMarket(orderParams)
	if err != nil {
		utils.Log.Error(err)
	}
}

func (c *Base) gridBuilder(pair string, timeframe string, isForce bool) {
	currentGrid, gridExsit := c.pairGridMap.Get(pair)
	pairTubeOpen, _ := c.pairTubeOpen.Get(pair)
	if isForce == true && pairTubeOpen == false {
		utils.Log.Infof("[GRID: %s] Build - Living data has no exsit, wating...", pair)
		return
	}
	dataframe := c.samples[pair][timeframe]["Grid1h"]
	if len(dataframe.Close) == 0 {
		return
	}
	var dataIndex int
	var gridStep, emptySize float64
	// 判断是否是强制更新
	if isForce == false {
		dataIndex = 0
	} else {
		dataIndex = 1
	}
	openPrice := dataframe.Open.Last(dataIndex)
	lowPrice := dataframe.Low.Last(dataIndex)
	highPrice := dataframe.High.Last(dataIndex)
	closePrice := dataframe.Close.Last(dataIndex)
	bbUpper := dataframe.Metadata["bbUpper"].Last(dataIndex)
	bbMiddle := dataframe.Metadata["bbMiddle"].Last(dataIndex)
	bbLower := dataframe.Metadata["bbLower"].Last(dataIndex)
	bbWidth := dataframe.Metadata["bbWidth"].Last(dataIndex)
	avgVolume := dataframe.Metadata["avgVolume"].Last(dataIndex)
	tendency := dataframe.Metadata["tendency"].Last(dataIndex)
	discrepancy := dataframe.Metadata["discrepancy"].Last(dataIndex)
	volume := dataframe.Metadata["volume"].Last(dataIndex)
	midPrice := dataframe.Metadata["basePrice"].Last(dataIndex)
	// 更新平均量能
	c.lastAvgVolume.Set(pair, avgVolume)
	// 获取当前已存在的仓位,保持原有网格
	openedPositions, err := c.broker.GetPositionsForPair(pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) > 0 && gridExsit == true {
		utils.Log.Infof("[GRID: %s] Build - Position has exsit, wating...", pair)
		return
	}
	// 当前趋势角度大于限制角度时
	if calc.Abs(tendency) > TendencyAngleLimit {
		// 使用最大网格
		gridStep = c.pairOptions[pair].MaxGridStep
		// 上升趋势时，需要更高的空单，角度每增加一度空窗网格增加一格
		emptySize = gridStep * calc.FormatAmountToSize(calc.Abs(tendency)-TendencyAngleLimit, 0.1)
		if tendency > 0 {
			midPrice = calc.AccurateAdd(bbMiddle, discrepancy)
		} else {
			midPrice = calc.AccurateSub(bbMiddle, discrepancy)
		}
	} else {
		// 根据振幅动态计算网格大小
		amplitude := indicator.AMP(openPrice, highPrice, lowPrice)
		if amplitude <= AmplitudeMinRatio {
			gridStep = c.pairOptions[pair].MaxGridStep
			emptySize = gridStep * 1.8
		} else if amplitude >= AmplitudeMaxRatio {
			gridStep = c.pairOptions[pair].MinGridStep
			emptySize = gridStep * 1.2
		} else {
			gridStep = float64(c.pairOptions[pair].MinGridStep) + (amplitude-AmplitudeMinRatio)*(float64(c.pairOptions[pair].MaxGridStep-c.pairOptions[pair].MinGridStep)/(AmplitudeMaxRatio-AmplitudeMinRatio))
			emptySize = gridStep * 1.5
		}
		// 判断基准线
		if midPrice > bbMiddle {
			// 如果上轨与均线的距离大于均线与中轨的距离，则使用中轨作为基准线
			if (bbUpper-midPrice) > (midPrice-bbMiddle) && openPrice > closePrice {
				midPrice = bbMiddle
			}
		} else {
			// 如果下轨与均线的距离大于中轨与均线的距离，则使用中轨作为基准线
			if (midPrice-bbLower) > (bbMiddle-midPrice) && closePrice > openPrice {
				midPrice = bbMiddle
			}
		}
	}

	if gridExsit && currentGrid.BasePrice == midPrice {
		return
	}
	// 上一根蜡烛线已经破线，不在初始化网格
	if closePrice > bbUpper || closePrice < bbLower {
		utils.Log.Infof("[Grid: %s] Build - Bolling has cross limit, wating...", pair)
		c.pairGridMap.Delete(pair)
		return
	}
	// 上根线量能是否过大，过大时不在初始化网格
	if (volume / avgVolume) > AvgVolumeLimitRatio {
		utils.Log.Infof("[GRID: %s] Build - Volume bigger than avgVolume, wating...", pair)
		c.pairGridMap.Delete(pair)
		return
	}

	// 计算网格数量
	numGrids := bbWidth / gridStep
	if numGrids <= 0 {
		utils.Log.Infof("[GRID: %s] Build - Grid spacing too large for the given price width, wating...", pair)
		return
	}
	// 改为固定
	//sideGridNum := numGrids / 2
	//sideGridNum := c.pairOptions[pair].MaxAddPosition

	// 初始化网格
	grid := model.PositionGrid{
		BasePrice:     midPrice,
		GridStep:      gridStep,
		CountLong:     c.pairOptions[pair].MaxAddPosition,
		CountShort:    c.pairOptions[pair].MaxAddPosition,
		CreatedAt:     dataframe.Time[len(dataframe.Time)-dataIndex-1],
		GridItems:     []model.PositionGridItem{},
		BoundaryUpper: bbUpper,
		BoundaryLower: bbLower,
	}

	var longPrice, shortPrice float64
	var longGridItems, shortGridItems []model.PositionGridItem
	// 获取网格固定数量
	// 计算网格上下限
	for i := 0; i <= int(grid.CountShort); i++ {
		if dataframe.Open.Last(1) < closePrice {
			shortPrice = midPrice + float64(i)*gridStep + emptySize
		} else {
			shortPrice = midPrice + float64(i)*gridStep + c.pairOptions[pair].WindowPeriod
		}
		shortGridItems = append(shortGridItems, model.PositionGridItem{
			Side:         model.SideTypeSell,
			PositionSide: model.PositionSideTypeShort,
			Price:        shortPrice,
			Lock:         false,
		})
		// 突破线后多给一个网格
		// todo 不限制布林带网格
		//if longPrice > bbUpper {
		//	break
		//}
	}
	for i := 0; i <= int(grid.CountLong); i++ {
		if dataframe.Open.Last(1) > closePrice {
			longPrice = midPrice - float64(i)*gridStep - emptySize
		} else {
			longPrice = midPrice - float64(i)*gridStep - c.pairOptions[pair].WindowPeriod
		}
		longGridItems = append(longGridItems, model.PositionGridItem{
			Side:         model.SideTypeBuy,
			PositionSide: model.PositionSideTypeLong,
			Price:        longPrice,
			Lock:         false,
		})
		// 突破线后多给一个网格
		// todo 不限制布林带网格
		//if shortPrice < bbLower {
		//	break
		//}
	}
	if len(longGridItems) < int(c.pairOptions[pair].MinAddPosition) || len(shortGridItems) < int(c.pairOptions[pair].MinAddPosition) {
		utils.Log.Infof(
			"[GRID: %s] Build - Too few grids (Min Add Position: %v ),Long:%v, Short:%v, wating...",
			pair,
			c.pairOptions[pair].MinAddPosition,
			len(longGridItems),
			len(shortGridItems),
		)
		c.pairGridMap.Delete(pair)
		return
	}
	grid.GridItems = append(grid.GridItems, longGridItems...)
	grid.GridItems = append(grid.GridItems, shortGridItems...)
	grid.SortGridItemsByPrice(true)
	grid.CountGrid = int64(len(grid.GridItems))
	grid.CountLong = int64(len(longGridItems))
	grid.CountShort = int64(len(shortGridItems))
	utils.Log.Infof(
		"[GRID: %s] Build - BasePrice: %v | Upper: %v | Lower: %v | Count: %v | CreatedAt: %s (Index: %v, Force: %v, Tube: %v)",
		pair,
		grid.BasePrice,
		grid.GridItems[len(grid.GridItems)-1].Price,
		grid.GridItems[0].Price,
		grid.CountGrid,
		grid.CreatedAt.In(Loc).Format("2006-01-02 15:04:05"),
		dataIndex,
		isForce,
		pairTubeOpen,
	)
	// 网格被重新生成时取消所有挂单
	go c.CloseOrder(false)
	// 将网格添加到网格映射中
	c.pairGridMap.Set(pair, &grid)
}

func (c *Base) ResetGrid(pair string) {
	currentGrid, gridExsit := c.pairGridMap.Get(pair)
	if !gridExsit {
		return
	}
	if len(currentGrid.GridItems) == 0 {
		return
	}
	// 修改网格锁定状态
	for i := range currentGrid.GridItems {
		currentGrid.GridItems[i].Lock = false
	}
	c.pairGridMap.Set(pair, currentGrid)
}

func (c *Base) getOpenGrid(pair string, currentPrice float64) (int, error) {
	openGridIndex := -1
	currentGrid, gridExsit := c.pairGridMap.Get(pair)
	if !gridExsit {
		return openGridIndex, errors.New(fmt.Sprintf("[POSITION] Grid for %s not exsit, waiting ....", pair))
	}
	if currentPrice == currentGrid.BasePrice {
		return openGridIndex, errors.New(fmt.Sprintf("[GRID] Pair for %s price at base line, ignore ...", pair))
	}
	if currentPrice > currentGrid.BasePrice {
		for i, item := range currentGrid.GridItems {
			if item.PositionSide == model.PositionSideTypeLong {
				continue
			}
			if currentPrice < item.Price && item.Lock == false {
				openGridIndex = i
				break
			}
		}
	} else {
		for i := len(currentGrid.GridItems) - 1; i >= 0; i-- {
			item := currentGrid.GridItems[i]
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

func (c *Base) generateTriggerSequence(initialTriggerRatio, triggerIncrement, totalDrawdown, drawdownInterval float64) []*model.StopProfitLevel {
	var triggerSequence []*model.StopProfitLevel
	totalSteps := int(totalDrawdown / drawdownInterval)

	for i := 0; i <= totalSteps; i++ {
		triggerRatio := calc.FormatAmountToSize(initialTriggerRatio+float64(i)*triggerIncrement, 0.0001)
		drawdownRatio := calc.FormatAmountToSize(totalDrawdown-float64(i)*drawdownInterval, 0.0001)

		var nextTriggerRatio float64
		if i < totalSteps {
			// 计算下一次的触发比例
			nextTriggerRatio = calc.FormatAmountToSize(initialTriggerRatio+float64(i+1)*triggerIncrement, 0.0001)
		} else {
			// 如果是最后一个步骤，下一次触发比例可以设置为0或其他适当值
			nextTriggerRatio = 0.0
		}

		triggerSequence = append(triggerSequence, &model.StopProfitLevel{
			TriggerRatio:     triggerRatio,
			DrawdownRatio:    drawdownRatio,
			NextTriggerRatio: nextTriggerRatio, // 新增字段
		})
	}
	return triggerSequence
}

func (c *Base) resetPairProfit(pair string) {
	pairProfitLevels, _ := c.pairProfitLevels.Get(pair)
	c.pairCurrentProfit.Set(pair, &model.PairProfit{
		Close:     0,
		MaxProfit: 0,
		Floor:     pairProfitLevels[0].TriggerRatio,
		Decrease:  pairProfitLevels[0].DrawdownRatio,
		IsLock:    false,
	})
}

func (c *Base) findProfitLevel(pair string, profit float64) *model.StopProfitLevel {
	var bestMatch *model.StopProfitLevel
	pairProfitLevels, _ := c.pairProfitLevels.Get(pair)

	for _, level := range pairProfitLevels {
		if profit > level.TriggerRatio {
			bestMatch = level // 更新最佳匹配
		} else {
			break // 一旦找到比 profit 大的 TriggerRatio，退出循环
		}
	}
	return bestMatch
}

func (c *Base) getPositionMargin(quotePosition, currentPrice float64, option *model.PairOption) float64 {
	var amount float64
	switch option.MarginMode {
	case model.MarginModeRoll:
		amount = calc.OpenPositionSize(quotePosition, float64(option.Leverage), currentPrice, option.MarginSize)
		break
	case model.MarginModeMargin:
		amount = calc.PositionSize(option.MarginSize, float64(option.Leverage), currentPrice)
		break
	case model.MarginModeStatic:
		amount = option.MarginSize
		break
	default:
		utils.Log.Errorf("unkown pair: %s margin mode", option.Pair)
	}
	return amount
}
