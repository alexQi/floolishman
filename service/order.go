package service

import (
	"context"
	"floolishman/reference"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"github.com/samber/lo"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"floolishman/exchange"
	"floolishman/model"
	"floolishman/storage"

	"github.com/olekukonko/tablewriter"
)

type summary struct {
	Pair              string
	WinLong           []float64
	WinLongPercent    []float64
	WinLongStrateis   map[string]int
	WinShort          []float64
	WinShortPercent   []float64
	WinShortStrateis  map[string]int
	LoseLong          []float64
	LoseLongPercent   []float64
	LoseLongStrateis  map[string]int
	LoseShort         []float64
	LoseShortPercent  []float64
	LoseShortStrateis map[string]int
	Volume            float64
}

func (s summary) Win() []float64 {
	return append(s.WinLong, s.WinShort...)
}

func (s summary) WinPercent() []float64 {
	return append(s.WinLongPercent, s.WinShortPercent...)
}

func (s summary) Lose() []float64 {
	return append(s.LoseLong, s.LoseShort...)
}

func (s summary) LosePercent() []float64 {
	return append(s.LoseLongPercent, s.LoseShortPercent...)
}

func (s summary) Profit() float64 {
	profit := 0.0
	for _, value := range append(s.Win(), s.Lose()...) {
		profit += value
	}
	return profit
}

func (s summary) SQN() float64 {
	total := float64(len(s.Win()) + len(s.Lose()))
	avgProfit := s.Profit() / total
	stdDev := 0.0
	for _, profit := range append(s.Win(), s.Lose()...) {
		stdDev += math.Pow(profit-avgProfit, 2)
	}
	stdDev = math.Sqrt(stdDev / total)
	return math.Sqrt(total) * (s.Profit() / total) / stdDev
}

func (s summary) Payoff() float64 {
	avgWin := 0.0
	avgLose := 0.0

	for _, value := range s.WinPercent() {
		avgWin += value
	}

	for _, value := range s.LosePercent() {
		avgLose += value
	}

	if len(s.Win()) == 0 || len(s.Lose()) == 0 || avgLose == 0 {
		return 0
	}

	return (avgWin / float64(len(s.Win()))) / math.Abs(avgLose/float64(len(s.Lose())))
}

func (s summary) ProfitFactor() float64 {
	if len(s.Lose()) == 0 {
		return 0
	}
	profit := 0.0
	for _, value := range s.WinPercent() {
		profit += value
	}

	loss := 0.0
	for _, value := range s.LosePercent() {
		loss += value
	}
	return profit / math.Abs(loss)
}

func (s summary) WinPercentage() float64 {
	if len(s.Win())+len(s.Lose()) == 0 {
		return 0
	}

	return float64(len(s.Win())) / float64(len(s.Win())+len(s.Lose())) * 100
}

func (s summary) String() string {
	tableString := &strings.Builder{}
	table := tablewriter.NewWriter(tableString)
	_, quote := exchange.SplitAssetQuote(s.Pair)
	data := [][]string{
		{"Coin", s.Pair},
		{"Trades", strconv.Itoa(len(s.Lose()) + len(s.Win()))},
		{"Win", strconv.Itoa(len(s.Win()))},
		{"Loss", strconv.Itoa(len(s.Lose()))},
		{"Win.Percent", fmt.Sprintf("%.1f", s.WinPercentage())},
		{"Payoff", fmt.Sprintf("%.1f", s.Payoff()*100)},
		{"Pr.Fact", fmt.Sprintf("%.1f", s.ProfitFactor()*100)},
		{"Profit", fmt.Sprintf("%.4f %s", s.Profit(), quote)},
		{"Volume", fmt.Sprintf("%.4f %s", s.Volume, quote)},
	}
	table.AppendBulk(data)
	table.SetColumnAlignment([]int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_RIGHT})
	table.Render()
	return tableString.String()
}

func (s summary) StdoutString() string {
	_, quote := exchange.SplitAssetQuote(s.Pair)
	return fmt.Sprintf("Coin: %s |  Trades: %d | Win %d | Loss: %d | Win Percent: %.1f | Payoff: %.1f | Pr.Fact: %.1f | Profit: %.4f %s | Volume: %.4f %s",
		s.Pair,
		len(s.Lose())+len(s.Win()),
		len(s.Win()),
		len(s.Lose()),
		s.WinPercentage(),
		s.Payoff()*100,
		s.ProfitFactor()*100,
		s.Profit(),
		quote,
		s.Volume,
		quote,
	)
}

func (s summary) SaveReturns(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, value := range s.WinPercent() {
		_, err = file.WriteString(fmt.Sprintf("%.4f\n", value))
		if err != nil {
			return err
		}
	}

	for _, value := range s.LosePercent() {
		_, err = file.WriteString(fmt.Sprintf("%.4f\n", value))
		if err != nil {
			return err
		}
	}
	return nil
}

type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusError   Status = "error"
)

type Result struct {
	Pair          string
	OrderFlag     string // 当前方向仓位标识
	ProfitPercent float64
	ProfitValue   float64
	MatchStrategy map[string]int
	Side          model.SideType
	Duration      time.Duration
	CreatedAt     time.Time
}
type ServiceOrder struct {
	mtx            sync.Mutex
	ctx            context.Context
	exchange       reference.Exchange
	storage        storage.Storage
	orderFeed      *model.Feed
	notifier       reference.Notifier
	Results        map[string]*summary
	lastPrice      map[string]float64
	tickerInterval time.Duration
	finish         chan bool
	status         Status

	postionMap map[string]map[string]*model.Position
}

func NewServiceOrder(ctx context.Context, exchange reference.Exchange, storage storage.Storage,
	orderFeed *model.Feed) *ServiceOrder {

	return &ServiceOrder{
		ctx:            ctx,
		storage:        storage,
		exchange:       exchange,
		orderFeed:      orderFeed,
		lastPrice:      make(map[string]float64),
		Results:        make(map[string]*summary),
		tickerInterval: time.Second,
		finish:         make(chan bool),
		postionMap:     make(map[string]map[string]*model.Position),
	}
}

func (c *ServiceOrder) SetNotifier(notifier reference.Notifier) {
	c.notifier = notifier
}

func (c *ServiceOrder) OnCandle(candle model.Candle) {
	c.lastPrice[candle.Pair] = candle.Close
}

func (c *ServiceOrder) GetPositionsForPair(pair string) ([]*model.Position, error) {
	positions := []*model.Position{}
	if _, ok := c.postionMap[pair]; ok {
		for _, position := range c.postionMap[pair] {
			positions = append(positions, position)
		}
	}
	// 内存缓存中没有查询到，去数据库查询
	if len(positions) == 0 {
		positions, err := c.storage.Positions(
			storage.PositionFilterParams{
				Pair:   pair,
				Status: 0,
			},
		)
		if err != nil {
			return positions, err
		}
		// 重新缓存到内存
		if len(positions) > 0 {
			go func() {
				// 交易对已知，提前初始化
				if _, ok := c.postionMap[pair]; !ok {
					c.postionMap[pair] = make(map[string]*model.Position)
				}
				for _, position := range positions {
					c.postionMap[position.Pair][position.PositionSide] = position
				}
			}()
		}
	}

	return positions, nil
}

func (c *ServiceOrder) GetPositionsForOpened() ([]*model.Position, error) {
	positions := []*model.Position{}
	if len(c.postionMap) > 0 {
		for _, pairPositions := range c.postionMap {
			for _, position := range pairPositions {
				positions = append(positions, position)
			}
		}
	}
	// 内存缓存中没有查询到，去数据库查询
	if len(positions) == 0 {
		positions, err := c.storage.Positions(
			storage.PositionFilterParams{
				Status: 0,
			},
		)
		if err != nil {
			return positions, err
		}
		// 重新缓存到内存
		if len(positions) > 0 {
			go func() {
				for _, position := range positions {
					if _, ok := c.postionMap[position.Pair]; !ok {
						c.postionMap[position.Pair] = make(map[string]*model.Position)
					}
					c.postionMap[position.Pair][position.PositionSide] = position
				}
			}()
		}
	}

	return positions, nil
}

func (c *ServiceOrder) GetOrdersForPostionLossUnfilled(orderFlag string) ([]*model.Order, error) {
	orders := []*model.Order{}
	// todo 改为单独查询指定开仓还是平仓单
	orders, err := c.storage.Orders(
		storage.OrderFilterParams{
			OrderFlag: orderFlag,
			Statuses: []model.OrderStatusType{
				model.OrderStatusTypeNew, // 未成交订单
			},
		},
	)
	if err != nil {
		return orders, err
	}
	return lo.Filter(orders, func(order *model.Order, _ int) bool {
		if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeShort) ||
			(order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeLong) {
			return false
		}
		return true
	}), nil
}

func (c *ServiceOrder) GetOrdersForUnfilled() ([]*model.Order, error) {
	orders := []*model.Order{}
	orders, err := c.storage.Orders(
		storage.OrderFilterParams{
			Statuses: []model.OrderStatusType{
				model.OrderStatusTypeNew, // 未成交订单
			},
		},
	)
	if err != nil {
		return orders, err
	}
	return orders, nil
}

func (c *ServiceOrder) Update(p *model.Position, order *model.Order) (result *Result) {
	if p.PositionSide != string(order.PositionSide) {
		return nil
	}
	price := order.Price
	if p.Side == string(order.Side) {
		p.AvgPrice = (p.AvgPrice*p.Quantity + price*order.Quantity) / (p.Quantity + order.Quantity)
		p.Quantity += order.Quantity
	} else {
		if p.PositionSide == string(model.PositionSideTypeLong) {
			// 平多单
			if p.Quantity == order.Quantity {
				p.Status = 1
			} else {
				p.Quantity -= order.Quantity
			}
			quantity := calc.Abs(order.Quantity)
			order.Profit = (price - p.AvgPrice) / p.AvgPrice
			order.ProfitValue = (price - p.AvgPrice) * quantity

			result = &Result{
				CreatedAt:     order.CreatedAt,
				Pair:          order.Pair,
				Duration:      order.CreatedAt.Sub(p.CreatedAt),
				ProfitPercent: order.Profit,
				ProfitValue:   order.ProfitValue,
				Side:          model.SideType(p.Side),
				MatchStrategy: p.MatchStrategy,
			}
		} else {
			// 平空单
			if p.Quantity == order.Quantity {
				p.Status = 1
			} else {
				p.Quantity -= order.Quantity
			}
			quantity := calc.Abs(order.Quantity)
			order.Profit = (p.AvgPrice - price) / p.AvgPrice
			order.ProfitValue = (p.AvgPrice - price) * quantity

			result = &Result{
				CreatedAt:     order.CreatedAt,
				Pair:          order.Pair,
				Duration:      order.CreatedAt.Sub(p.CreatedAt),
				ProfitPercent: order.Profit,
				ProfitValue:   order.ProfitValue,
				Side:          model.SideType(p.Side),
				MatchStrategy: p.MatchStrategy,
			}
		}
		return result
	}

	return nil
}

func (c *ServiceOrder) updatePosition(o *model.Order) {
	_, ok := c.postionMap[o.Pair]
	if !ok {
		c.postionMap[o.Pair] = make(map[string]*model.Position)
	}
	position, ok := c.postionMap[o.Pair][o.OrderFlag]
	// 查询当前内存缓存是否存在仓位，不存在则创建仓位
	if !ok {
		// 判断当前订单在数据库中是否有仓位
		position, err := c.storage.GetPosition(storage.PositionFilterParams{
			Pair:      o.Pair,
			OrderFlag: o.OrderFlag,
			Status:    0,
		})
		if err != nil {
			utils.Log.Error(err)
			return
		}
		// 不存在则创建仓位
		if position.ID == 0 {
			position = &model.Position{
				OrderFlag:      o.OrderFlag,
				AvgPrice:       o.Price,
				Quantity:       o.Quantity,
				Leverage:       o.Leverage,
				LongShortRatio: o.LongShortRatio,
				CreatedAt:      o.CreatedAt,
				Side:           string(o.Side),
				PositionSide:   string(o.PositionSide),
				MatchStrategy:  o.MatchStrategy,
			}
			c.postionMap[o.Pair][o.OrderFlag] = position
			// 插入数据
			err := c.storage.CreatePosition(position)
			if err != nil {
				utils.Log.Error(err)
			}
			return
		} else {
			c.postionMap[o.Pair][o.OrderFlag] = position
		}
	}

	result := c.Update(position, o)
	if position.Status > 0 {
		// 更新数据库仓位记录
		err := c.storage.UpdatePosition(position)
		if err != nil {
			return
		}
		// 删除内存缓存
		delete(c.postionMap[o.Pair], o.OrderFlag)
	}

	if result != nil {
		if result.ProfitPercent >= 0 {
			if result.Side == model.SideTypeBuy {
				c.Results[o.Pair].WinLong = append(c.Results[o.Pair].WinLong, result.ProfitValue)
				c.Results[o.Pair].WinLongPercent = append(c.Results[o.Pair].WinLongPercent, result.ProfitPercent)

				for s, i := range result.MatchStrategy {
					c.Results[o.Pair].WinLongStrateis[s] += i
				}
			} else {
				c.Results[o.Pair].WinShort = append(c.Results[o.Pair].WinShort, result.ProfitValue)
				c.Results[o.Pair].WinShortPercent = append(c.Results[o.Pair].WinShortPercent, result.ProfitPercent)

				for s, i := range result.MatchStrategy {
					c.Results[o.Pair].WinShortStrateis[s] += i
				}
			}
		} else {
			if result.Side == model.SideTypeBuy {
				c.Results[o.Pair].LoseLong = append(c.Results[o.Pair].LoseLong, result.ProfitValue)
				c.Results[o.Pair].LoseLongPercent = append(c.Results[o.Pair].LoseLongPercent, result.ProfitPercent)

				for s, i := range result.MatchStrategy {
					c.Results[o.Pair].LoseLongStrateis[s] += i
				}
			} else {
				c.Results[o.Pair].LoseShort = append(c.Results[o.Pair].LoseShort, result.ProfitValue)
				c.Results[o.Pair].LoseShortPercent = append(c.Results[o.Pair].LoseShortPercent, result.ProfitPercent)

				for s, i := range result.MatchStrategy {
					c.Results[o.Pair].LoseShortStrateis[s] += i
				}
			}
		}
		_, quote := exchange.SplitAssetQuote(o.Pair)
		c.notify(fmt.Sprintf(
			"[SUMMARY] %f %s (%f %%) \n`%s`",
			result.ProfitValue,
			quote,
			result.ProfitPercent*100,
			c.Results[o.Pair].String(),
		))
	}
}

func (c *ServiceOrder) notify(message string) {
	utils.Log.Tracef(message)
	if c.notifier != nil {
		c.notifier.Notify(message)
	}
}

func (c *ServiceOrder) notifyError(err error) {
	utils.Log.Error(err)
	if c.notifier != nil {
		c.notifier.OnError(err)
	}
}

func (c *ServiceOrder) processTrade(order *model.Order) {
	if order.Status != model.OrderStatusTypeFilled {
		return
	}

	// initializer results map if needed
	if _, ok := c.Results[order.Pair]; !ok {
		c.Results[order.Pair] = &summary{
			Pair:              order.Pair,
			WinLongStrateis:   make(map[string]int),
			WinShortStrateis:  make(map[string]int),
			LoseLongStrateis:  make(map[string]int),
			LoseShortStrateis: make(map[string]int),
		}
	}

	// register order volume
	c.Results[order.Pair].Volume += order.Price * order.Quantity

	// update position size / avg price
	c.updatePosition(order)
}
func (c *ServiceOrder) Status() Status {
	return c.status
}

func (c *ServiceOrder) ListenOrders() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	//pending orders
	orders, err := c.storage.Orders(
		storage.OrderFilterParams{
			Statuses: []model.OrderStatusType{
				model.OrderStatusTypeNew,
				model.OrderStatusTypePartiallyFilled,
				model.OrderStatusTypePendingCancel,
			},
		},
	)
	if err != nil {
		c.notifyError(err)
		c.mtx.Unlock()
		return
	}
	// For each pending order, check for updates
	var updatedOrders []model.Order
	for _, order := range orders {
		excOrder, err := c.exchange.Order(order.Pair, order.ExchangeID)
		if err != nil {
			utils.Log.WithField("id", order.ExchangeID).Error("orderControler/get: ", err)
			continue
		}
		// 排出限价单已成交的部分 查询 限价单未成交 及 止损单 no status change
		if excOrder.Status == order.Status {
			continue
		}

		excOrder.ID = order.ID
		excOrder.OrderFlag = order.OrderFlag
		excOrder.Type = order.Type
		excOrder.Amount = order.Amount
		excOrder.LongShortRatio = order.LongShortRatio
		excOrder.Leverage = order.Leverage
		excOrder.GuiderPositionRate = order.GuiderPositionRate

		err = c.storage.UpdateOrder(&excOrder)
		if err != nil {
			c.notifyError(err)
			continue
		}
		// 重新放入策略统计数据
		excOrder.MatchStrategy = order.MatchStrategy
		// 放入更新数组
		utils.Log.Infof("[ORDER %s] %s", excOrder.Status, excOrder)
		updatedOrders = append(updatedOrders, excOrder)
	}

	for _, processOrder := range updatedOrders {
		c.processTrade(&processOrder)
		c.orderFeed.Publish(processOrder, false)
	}
}

func (c *ServiceOrder) Start() {
	if c.status != StatusRunning {
		c.status = StatusRunning
		// 监听已有仓位
		go func() {
			ticker := time.NewTicker(c.tickerInterval)
			for {
				select {
				case <-ticker.C:
					c.ListenOrders()
				case <-c.finish:
					ticker.Stop()
					return
				}
			}
		}()
		utils.Log.Info("Bot started.")
	}
}

func (c *ServiceOrder) Stop() {
	if c.status == StatusRunning {
		c.status = StatusStopped
		c.ListenOrders()
		c.finish <- true
		utils.Log.Info("Bot stopped.")
	}
}

func (c *ServiceOrder) Account() (model.Account, error) {
	return c.exchange.Account()
}

func (c *ServiceOrder) Position(pair string) (asset, quote float64, err error) {
	return c.exchange.Position(pair)
}

func (c *ServiceOrder) LastQuote(pair string) (float64, error) {
	return c.exchange.LastQuote(c.ctx, pair)
}

func (c *ServiceOrder) PositionValue(pair string) (float64, error) {
	asset, _, err := c.exchange.Position(pair)
	if err != nil {
		return 0, err
	}
	return asset * c.lastPrice[pair], nil
}

func (c *ServiceOrder) Order(pair string, id int64) (model.Order, error) {
	return c.exchange.Order(pair, id)
}

func (c *ServiceOrder) CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string, size, limit float64, extra model.OrderExtra) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating LIMIT %s order for %s", side, pair)
	order, err := c.exchange.CreateOrderLimit(side, positionSide, pair, size, limit, extra)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}

	err = c.storage.CreateOrder(&order)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}
	go c.orderFeed.Publish(order, true)
	utils.Log.Infof("[ORDER CREATED] %s", order)
	return order, nil
}

func (c *ServiceOrder) CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, size float64, extra model.OrderExtra) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating MARKET %s order for: %s", side, pair, extra.OrderFlag)
	order, err := c.exchange.CreateOrderMarket(side, positionSide, pair, size, extra)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}

	err = c.storage.CreateOrder(&order)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}
	utils.Log.Infof("[ORDER CREATED] %s", order)
	// calculate profit
	c.processTrade(&order)
	go c.orderFeed.Publish(order, true)
	return order, err
}

func (c *ServiceOrder) CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string, size, limit float64, stopPrice float64, extra model.OrderExtra) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating STOP LIMIT %s order for %s: %s", side, pair, extra.OrderFlag)
	order, err := c.exchange.CreateOrderStopLimit(side, positionSide, pair, size, limit, stopPrice, extra)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}

	err = c.storage.CreateOrder(&order)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}
	go c.orderFeed.Publish(order, true)
	utils.Log.Infof("[ORDER CREATED] %s", order)
	return order, nil
}

func (c *ServiceOrder) CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string, size, stopPrice float64, extra model.OrderExtra) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating STOP MARKET %s order for %s: %s", side, pair, extra.OrderFlag)
	order, err := c.exchange.CreateOrderStopMarket(side, positionSide, pair, size, stopPrice, extra)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}

	err = c.storage.CreateOrder(&order)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}
	utils.Log.Infof("[ORDER CREATED] %s", order)
	return order, nil
}

func (c *ServiceOrder) Cancel(order model.Order) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Cancelling order for %s", order.Pair)
	err := c.exchange.Cancel(order)
	if err != nil {
		return err
	}

	order.Status = model.OrderStatusTypePendingCancel
	err = c.storage.UpdateOrder(&order)
	if err != nil {
		c.notifyError(err)
		return err
	}
	utils.Log.Infof("[ORDER CANCELED] %s", order)
	return nil
}
