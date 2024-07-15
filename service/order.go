package service

import (
	"context"
	"floolishman/reference"
	"floolishman/utils"
	"fmt"
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
		{"% Win", fmt.Sprintf("%.1f", s.WinPercentage())},
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
	ProfitPercent float64
	ProfitValue   float64
	MatchStrategy map[string]int
	Side          model.SideType
	Duration      time.Duration
	CreatedAt     time.Time
}

type Position struct {
	Side          model.SideType
	PositionSide  model.PositionSideType
	MatchStrategy map[string]int
	AvgPrice      float64
	Quantity      float64
	CreatedAt     time.Time
}

func (p *Position) Update(order *model.Order) (result *Result, finished bool) {
	price := order.Price
	// 多单
	if p.PositionSide == order.PositionSide && p.PositionSide == model.PositionSideTypeLong {
		// 平仓
		if p.Side != order.Side && p.Side == model.SideTypeBuy {
			if p.Quantity == order.Quantity {
				finished = true
			} else {
				p.Quantity -= order.Quantity
			}

			quantity := math.Min(p.Quantity, order.Quantity)
			order.Profit = (price - p.AvgPrice) / p.AvgPrice
			order.ProfitValue = (price - p.AvgPrice) * quantity

			result = &Result{
				CreatedAt:     order.CreatedAt,
				Pair:          order.Pair,
				Duration:      order.CreatedAt.Sub(p.CreatedAt),
				ProfitPercent: order.Profit,
				ProfitValue:   order.ProfitValue,
				Side:          p.Side,
				MatchStrategy: order.MatchStrategy,
			}

			return result, finished
		}
	}
	// 空单
	if p.PositionSide == order.PositionSide && p.PositionSide == model.PositionSideTypeShort {
		// 开仓
		if p.Side != order.Side && p.Side == model.SideTypeSell {
			// 平仓
			if p.Quantity == order.Quantity {
				finished = true
			} else {
				p.Quantity -= order.Quantity
			}

			quantity := math.Min(p.Quantity, order.Quantity)
			order.Profit = (p.AvgPrice - price) / p.AvgPrice
			order.ProfitValue = (p.AvgPrice - price) * quantity

			result = &Result{
				CreatedAt:     order.CreatedAt,
				Pair:          order.Pair,
				Duration:      order.CreatedAt.Sub(p.CreatedAt),
				ProfitPercent: order.Profit,
				ProfitValue:   order.ProfitValue,
				Side:          p.Side,
				MatchStrategy: order.MatchStrategy,
			}

			return result, finished
		}
	}

	return nil, false
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

	position map[string]*Position
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
		position:       make(map[string]*Position),
	}
}

func (c *ServiceOrder) SetNotifier(notifier reference.Notifier) {
	c.notifier = notifier
}

func (c *ServiceOrder) OnCandle(candle model.Candle) {
	c.lastPrice[candle.Pair] = candle.Close
}

func (c *ServiceOrder) GetCurrentPositionOrders(pair string) ([]*model.Order, error) {
	orders := []*model.Order{}
	orders, err := c.storage.Orders(
		storage.WithPair(pair),
		storage.WithTradingStatus(0), // 交易状态未完成
		storage.WithStatusIn(
			model.OrderStatusTypeNew,    // 未成交订单
			model.OrderStatusTypeFilled, // 已成交订单
		),
	)
	if err != nil {
		return orders, err
	}
	return orders, nil
}

func (c *ServiceOrder) updatePosition(o *model.Order) {
	// get filled orders before the current order
	position, ok := c.position[o.Pair]
	if !ok {
		c.position[o.Pair] = &Position{
			AvgPrice:      o.Price,
			Quantity:      o.Quantity,
			CreatedAt:     o.CreatedAt,
			Side:          o.Side,
			PositionSide:  o.PositionSide,
			MatchStrategy: o.MatchStrategy,
		}
		return
	}

	result, closed := position.Update(o)
	if closed {
		delete(c.position, o.Pair)
	}

	if result != nil {
		// TODO: replace by a slice of Result
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
			"[SUMMARY] %f %s (%f %%) `%s`",
			result.ProfitValue,
			quote,
			result.ProfitPercent*100,
			c.Results[o.Pair].String(),
		))
	}
}

func (c *ServiceOrder) notify(message string) {
	utils.Log.Info(message)
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

func (c *ServiceOrder) updateOrders() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	//pending orders
	orders, err := c.storage.Orders(
		storage.WithStatusIn(
			model.OrderStatusTypeNew,
			model.OrderStatusTypeFilled,
			model.OrderStatusTypePartiallyFilled,
			model.OrderStatusTypePendingCancel,
		),
		storage.WithTradingStatus(0),
	)
	if err != nil {
		c.notifyError(err)
		c.mtx.Unlock()
		return
	}

	processedPositionOrders := map[string]*model.Order{}
	// For each pending order, check for updates
	var updatedOrders []model.Order
	for _, order := range orders {
		if _, ok := processedPositionOrders[order.ClientOrderId]; ok {
			continue
		}
		if (order.Type == model.OrderTypeLimit || order.Type == model.OrderTypeMarket) && order.Status == model.OrderStatusTypeFilled {
			continue
		}
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
		excOrder.LongShortRatio = order.LongShortRatio
		excOrder.MatchStrategy = order.MatchStrategy

		// 判断交易状态,如果已完成，关闭仓位 及止盈止损仓位
		if excOrder.Status == model.OrderStatusTypeFilled && (excOrder.Type == model.OrderTypeStop || excOrder.Type == model.OrderTypeStopMarket) {
			// 修改当前止损止盈单状态为已交易完成
			excOrder.TradingStatus = 1
			// 修改当前止损止盈单关联的仓位为已交易完成
			positionOrders, err := c.storage.Orders(
				storage.WithOrderTypeIn(model.OrderTypeLimit),
				storage.WithStatusIn(model.OrderStatusTypeFilled),
				storage.WithOrderFlag(excOrder.OrderFlag),
				storage.WithTradingStatus(0),
			)
			if err != nil {
				c.notifyError(err)
				c.mtx.Unlock()
				return
			}
			if len(positionOrders) > 0 {
				for _, positionOrder := range positionOrders {
					positionOrder.TradingStatus = 1
					err = c.storage.UpdateOrder(positionOrder)
					if err != nil {
						c.notifyError(err)
						continue
					}
					processedPositionOrders[positionOrder.ClientOrderId] = positionOrder
				}
			}
		}

		err = c.storage.UpdateOrder(&excOrder)
		if err != nil {
			c.notifyError(err)
			continue
		}

		utils.Log.Infof("[ORDER %s] %s", excOrder.Status, excOrder)
		updatedOrders = append(updatedOrders, excOrder)
	}

	for _, processOrder := range updatedOrders {
		c.processTrade(&processOrder)
		c.orderFeed.Publish(processOrder, false)
	}
}

func (c *ServiceOrder) Status() Status {
	return c.status
}

func (c *ServiceOrder) ListenUpdateOrders() {
	c.updateOrders()
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
					c.updateOrders()
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
		c.updateOrders()
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

func (c *ServiceOrder) CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string, size, limit float64, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating LIMIT %s order for %s", side, pair)
	order, err := c.exchange.CreateOrderLimit(side, positionSide, pair, size, limit, longShortRatio, matchStrategy)
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

func (c *ServiceOrder) CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, size float64, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating MARKET %s order for %s", side, pair)
	order, err := c.exchange.CreateOrderMarket(side, positionSide, pair, size, longShortRatio, matchStrategy)
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

func (c *ServiceOrder) CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string, size, limit float64, stopPrice float64, orderFlag string, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating STOP LIMIT %s order for %s", side, pair)
	order, err := c.exchange.CreateOrderStopLimit(side, positionSide, pair, size, limit, stopPrice, orderFlag, longShortRatio, matchStrategy)
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

func (c *ServiceOrder) CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string, size, stopPrice float64, orderFlag string, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	utils.Log.Infof("[ORDER] Creating STOP MARKET %s order for %s", side, pair)
	order, err := c.exchange.CreateOrderStopMarket(side, positionSide, pair, size, stopPrice, orderFlag, longShortRatio, matchStrategy)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}

	err = c.storage.CreateOrder(&order)
	if err != nil {
		c.notifyError(err)
		return model.Order{}, err
	}
	if order.Status == model.OrderStatusTypeFilled {
		c.processTrade(&order)
		go c.orderFeed.Publish(order, true)
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
