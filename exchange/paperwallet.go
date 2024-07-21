package exchange

import (
	"context"
	"errors"
	"floolishman/reference"
	"floolishman/utils"
	"floolishman/utils/calc"
	"floolishman/utils/strutil"
	"fmt"
	"github.com/adshao/go-binance/v2/common"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"floolishman/model"
)

type assetInfo struct {
	Free float64
	Lock float64
}

type AssetValue struct {
	Time  time.Time
	Value float64
}

type PaperWallet struct {
	sync.Mutex
	ctx           context.Context
	baseCoin      string
	counter       int64
	takerFee      float64
	makerFee      float64
	initialValue  float64
	feeder        reference.Feeder
	orders        []model.Order
	assets        map[string]*assetInfo
	avgShortPrice map[string]float64
	avgLongPrice  map[string]float64
	volume        map[string]float64
	lastCandle    map[string]model.Candle
	fistCandle    map[string]model.Candle
	assetValues   map[string][]AssetValue
	equityValues  []AssetValue
	PairOptions   map[string]model.PairOption
}

func (p *PaperWallet) ListenOrders() {
	//TODO implement me
	panic("implement me")
}

func (p *PaperWallet) AssetsInfo(pair string) model.AssetInfo {
	asset, quote := SplitAssetQuote(pair)
	return model.AssetInfo{
		BaseAsset:          asset,
		QuoteAsset:         quote,
		MaxPrice:           math.MaxFloat64,
		MaxQuantity:        math.MaxFloat64,
		StepSize:           0.00000001,
		TickSize:           0.00000001,
		QuotePrecision:     8,
		BaseAssetPrecision: 8,
	}
}

type PaperWalletOption func(*PaperWallet)

func WithPaperAsset(pair string, amount float64) PaperWalletOption {
	return func(wallet *PaperWallet) {
		wallet.assets[pair] = &assetInfo{
			Free: amount,
			Lock: 0,
		}
	}
}

func WithPaperFee(maker, taker float64) PaperWalletOption {
	return func(wallet *PaperWallet) {
		wallet.makerFee = maker
		wallet.takerFee = taker
	}
}

func WithDataFeed(feeder reference.Feeder) PaperWalletOption {
	return func(wallet *PaperWallet) {
		wallet.feeder = feeder
	}
}

func NewPaperWallet(ctx context.Context, baseCoin string, options ...PaperWalletOption) *PaperWallet {
	wallet := PaperWallet{
		ctx:           ctx,
		baseCoin:      baseCoin,
		orders:        make([]model.Order, 0),
		assets:        make(map[string]*assetInfo),
		fistCandle:    make(map[string]model.Candle),
		lastCandle:    make(map[string]model.Candle),
		avgShortPrice: make(map[string]float64),
		avgLongPrice:  make(map[string]float64),
		volume:        make(map[string]float64),
		assetValues:   make(map[string][]AssetValue),
		equityValues:  make([]AssetValue, 0),
		PairOptions:   make(map[string]model.PairOption),
	}

	for _, option := range options {
		option(&wallet)
	}

	wallet.initialValue = wallet.assets[wallet.baseCoin].Free
	utils.Log.Info("[SETUP] Using paper wallet")
	utils.Log.Infof("[SETUP] Initial Portfolio = %f %s", wallet.initialValue, wallet.baseCoin)

	return &wallet
}

func (b *PaperWallet) SetPairOption(_ context.Context, option model.PairOption) error {
	b.PairOptions[option.Pair] = option
	return nil
}

func (p *PaperWallet) ID() int64 {
	p.counter++
	return p.counter
}

func (p *PaperWallet) Pairs() []string {
	pairs := make([]string, 0)
	for pair := range p.assets {
		pairs = append(pairs, pair)
	}
	return pairs
}

func (p *PaperWallet) LastQuote(ctx context.Context, pair string) (float64, error) {
	return p.feeder.LastQuote(ctx, pair)
}

func (p *PaperWallet) AssetValues(pair string) []AssetValue {
	return p.assetValues[pair]
}

func (p *PaperWallet) EquityValues() []AssetValue {
	return p.equityValues
}

func (p *PaperWallet) MaxDrawdown() (float64, time.Time, time.Time) {
	if len(p.equityValues) < 1 {
		return 0, time.Time{}, time.Time{}
	}
	localMin := math.MaxFloat64
	localMinBase := p.equityValues[0].Value
	localMinStart := p.equityValues[0].Time
	localMinEnd := p.equityValues[0].Time

	globalMin := localMin
	globalMinBase := localMinBase
	globalMinStart := localMinStart
	globalMinEnd := localMinEnd

	for i := 1; i < len(p.equityValues); i++ {
		diff := p.equityValues[i].Value - p.equityValues[i-1].Value

		if localMin > 0 {
			localMin = diff
			localMinBase = p.equityValues[i-1].Value
			localMinStart = p.equityValues[i-1].Time
			localMinEnd = p.equityValues[i].Time
		} else {
			localMin += diff
			localMinEnd = p.equityValues[i].Time
		}

		if localMin < globalMin {
			globalMin = localMin
			globalMinBase = localMinBase
			globalMinStart = localMinStart
			globalMinEnd = localMinEnd
		}
	}

	return globalMin / globalMinBase, globalMinStart, globalMinEnd
}

func (p *PaperWallet) Summary() {
	var (
		total        float64
		marketChange float64
		volume       float64
	)

	baseCoinValue := p.assets[p.baseCoin].Free + p.assets[p.baseCoin].Lock
	fmt.Println("----- FINAL WALLET -----")
	fmt.Printf("FINAL QUOTE = %.4f %s (Free: %.4f,Lock: %.4f )\n", baseCoinValue, p.baseCoin, p.assets[p.baseCoin].Free, p.assets[p.baseCoin].Lock)

	fmt.Println()
	fmt.Println("----- FINAL POSITION -----")
	for pair := range p.lastCandle {
		asset, quote := SplitAssetQuote(pair)
		assetInfo, ok := p.assets[asset]
		if !ok {
			continue
		}

		quantity := assetInfo.Free + assetInfo.Lock
		value := quantity * p.lastCandle[pair].Close
		if quantity < 0 {
			totalShort := 2.0*p.avgShortPrice[pair]*quantity - p.lastCandle[pair].Close*quantity
			value = math.Abs(totalShort)
		}
		total += value
		marketChange += (p.lastCandle[pair].Close - p.fistCandle[pair].Close) / p.fistCandle[pair].Close
		fmt.Printf("%.4f %s = %.4f %s\n", quantity, asset, total, quote)
	}

	avgMarketChange := marketChange / float64(len(p.lastCandle))
	profit := baseCoinValue - p.initialValue

	fmt.Println()
	maxDrawDown, _, _ := p.MaxDrawdown()
	fmt.Println("----- RETURNS -----")
	fmt.Printf("START PORTFOLIO     = %.2f %s\n", p.initialValue, p.baseCoin)
	fmt.Printf("FINAL PORTFOLIO     = %.2f %s\n", baseCoinValue, p.baseCoin)
	fmt.Printf("GROSS PROFIT        =  %f %s (%.2f%%)\n", profit, p.baseCoin, profit/p.initialValue*100)
	fmt.Printf("MARKET CHANGE (B&H) =  %.2f%%\n", avgMarketChange*100)
	fmt.Println()
	fmt.Println("------ RISK -------")
	fmt.Printf("MAX DRAWDOWN = %.2f %%\n", maxDrawDown*100)
	fmt.Println()
	fmt.Println("------ VOLUME -----")
	for pair, vol := range p.volume {
		volume += vol
		fmt.Printf("%s         = %.2f %s\n", pair, vol, p.baseCoin)
	}
	fmt.Printf("TOTAL           = %.2f %s\n", volume, p.baseCoin)
	fmt.Println("-------------------")
}

func (p *PaperWallet) updateAveragePrice(side model.SideType, pair string, amount, value float64) {
	actualQty := 0.0
	asset, quote := SplitAssetQuote(pair)

	if p.assets[asset] != nil {
		actualQty = p.assets[asset].Free
	}

	// without previous position
	if actualQty == 0 {
		if side == model.SideTypeBuy {
			p.avgLongPrice[pair] = value
		} else {
			p.avgShortPrice[pair] = value
		}
		return
	}

	// actual long + order buy
	if actualQty > 0 && side == model.SideTypeBuy {
		positionValue := p.avgLongPrice[pair] * actualQty
		p.avgLongPrice[pair] = (positionValue + amount*value) / (actualQty + amount)
		return
	}

	// actual long + order sell
	if actualQty > 0 && side == model.SideTypeSell {
		profitValue := amount*value - math.Min(amount, actualQty)*p.avgLongPrice[pair]
		percentage := profitValue / (amount * p.avgLongPrice[pair])
		utils.Log.Infof("PROFIT = %.4f %s (%.2f %%)", profitValue, quote, percentage*100.0) // TODO: store profits

		if amount <= actualQty { // not enough quantity to close the position
			return
		}

		p.avgShortPrice[pair] = value

		return
	}

	// actual short + order sell
	if actualQty < 0 && side == model.SideTypeSell {
		positionValue := p.avgShortPrice[pair] * -actualQty
		p.avgShortPrice[pair] = (positionValue + amount*value) / (-actualQty + amount)

		return
	}

	// actual short + order buy
	if actualQty < 0 && side == model.SideTypeBuy {
		profitValue := math.Min(amount, -actualQty)*p.avgShortPrice[pair] - amount*value
		percentage := profitValue / (amount * p.avgShortPrice[pair])
		utils.Log.Infof("PROFIT = %.4f %s (%.2f %%)", profitValue, quote, percentage*100.0) // TODO: store profits

		if amount <= -actualQty { // not enough quantity to close the position
			return
		}

		p.avgLongPrice[pair] = value
	}
}

// ERROR BUG 考虑是订单关联更新导致已无新订单
func (p *PaperWallet) validateFunds(side model.SideType, positionSide model.PositionSideType, pair string, amount, value float64) error {
	asset, quote := SplitAssetQuote(pair)
	if _, ok := p.assets[asset]; !ok {
		p.assets[asset] = &assetInfo{}
	}

	if _, ok := p.assets[quote]; !ok {
		p.assets[quote] = &assetInfo{}
	}

	leverage := float64(p.PairOptions[pair].Leverage) // 获取合约杠杆倍数
	funds := p.assets[quote].Free * leverage
	if side == model.SideTypeSell {
		// 开空单
		if positionSide == model.PositionSideTypeShort {
			if funds < amount*value {
				return &OrderError{
					Err:      ErrInsufficientFunds,
					Pair:     pair,
					Quantity: amount,
				}
			}
		}
	} else { // SideTypeBuy
		// 开多单
		if positionSide == model.PositionSideTypeLong {
			if funds < amount*value {
				return &OrderError{
					Err:      ErrInsufficientFunds,
					Pair:     pair,
					Quantity: amount,
				}
			}
		}
	}

	return nil
}

func (p *PaperWallet) updateFunds(order *model.Order) error {
	asset, quote := SplitAssetQuote(order.Pair)
	if order.Status != model.OrderStatusTypeFilled {
		return nil
	}
	leverage := float64(p.PairOptions[order.Pair].Leverage) // 获取合约杠杆倍数
	// 平多单
	if order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeLong {
		if order.Status != model.OrderStatusTypeFilled {
			return nil
		}
		// 查询对应的仓位
		positonOrder, err := p.findPositonOrder(order.Pair, order.OrderFlag, model.OrderTypeLimit)
		if err != nil {
			utils.Log.Error(err)
		}
		if positonOrder.ExchangeID == 0 {
			return nil
		}
		if p.assets[asset].Lock < order.Quantity {
			return &OrderError{
				Err:      ErrInvalidAsset,
				Pair:     order.Pair,
				Quantity: order.Quantity,
			}
		}

		p.updateAveragePrice(order.Side, order.Pair, order.Quantity, order.Price)

		// 开多单资产
		p.assets[asset].Free = 0
		p.assets[asset].Lock -= order.Quantity

		lockQuote := positonOrder.Price * order.Quantity / leverage
		// 修改基本资产
		p.assets[quote].Lock -= lockQuote
		p.assets[quote].Free += lockQuote + (order.Price-positonOrder.Price)*order.Quantity

		utils.Log.Debugf("%s -> LOCK = %f / FREE %f", asset, p.assets[asset].Lock, p.assets[asset].Free)
	}
	// 平空单
	if order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeShort {
		if order.Status != model.OrderStatusTypeFilled {
			return nil
		}
		positonOrder, err := p.findPositonOrder(order.Pair, order.OrderFlag, model.OrderTypeLimit)
		if err != nil {
			utils.Log.Error(err)
		}
		if positonOrder.ExchangeID == 0 {
			return nil
		}
		if calc.Abs(p.assets[asset].Lock) < order.Quantity {
			return &OrderError{
				Err:      ErrInvalidAsset,
				Pair:     order.Pair,
				Quantity: order.Quantity,
			}
		}

		p.updateAveragePrice(order.Side, order.Pair, order.Quantity, order.Price)
		// 查询对应的仓位

		// 开多单资产
		p.assets[asset].Free = 0
		p.assets[asset].Lock += order.Quantity

		lockQuote := positonOrder.Price * order.Quantity / leverage
		// 修改基本资产
		p.assets[quote].Lock -= lockQuote
		p.assets[quote].Free += lockQuote + (positonOrder.Price-order.Price)*order.Quantity

		utils.Log.Debugf("%s -> LOCK = %f / FREE %f", asset, p.assets[asset].Lock, p.assets[asset].Free)
	}

	return nil
}

func (p *PaperWallet) OnCandle(candle model.Candle) {
	p.Lock()
	defer p.Unlock()

	p.lastCandle[candle.Pair] = candle
	if _, ok := p.fistCandle[candle.Pair]; !ok {
		p.fistCandle[candle.Pair] = candle
	}

	leverage := float64(p.PairOptions[candle.Pair].Leverage) // 获取合约杠杆倍数

	// 存储限价挂单
	limitOrders := map[string]model.Order{}
	for _, order := range p.orders {
		if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeLong) || (order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeShort) {
			continue
		}
		limitOrders[order.OrderFlag] = order
	}
	for i, order := range p.orders {
		if order.Pair != candle.Pair || order.Status != model.OrderStatusTypeNew {
			continue
		}

		if _, ok := p.volume[candle.Pair]; !ok {
			p.volume[candle.Pair] = 0
		}

		asset, quote := SplitAssetQuote(order.Pair)

		if order.Side == model.SideTypeBuy {
			// 开多单
			if order.PositionSide == model.PositionSideTypeLong && order.Price >= candle.Low {
				if _, ok := p.assets[quote]; !ok {
					p.assets[quote] = &assetInfo{}
				}
				var orderPrice float64
				if order.Type == model.OrderTypeLimit {
					orderPrice = order.Price
				} else {
					continue
				}
				volume := orderPrice * order.Quantity
				// 锁定的资产
				lockQuote := volume / leverage
				if p.assets[quote].Free < lockQuote {
					utils.Log.Warn(ErrInsufficientFunds)
					continue
				}

				p.volume[candle.Pair] += volume
				p.orders[i].UpdatedAt = candle.Time
				p.orders[i].Status = model.OrderStatusTypeFilled

				limitOrders[p.orders[i].OrderFlag] = p.orders[i]
				// update assets size
				p.updateAveragePrice(order.Side, order.Pair, order.Quantity, orderPrice)
				p.assets[asset].Free = 0
				p.assets[asset].Lock += order.Quantity

				p.assets[quote].Lock += lockQuote
				p.assets[quote].Free -= lockQuote
			}
			// 平空单
			if order.PositionSide == model.PositionSideTypeShort {
				limitOrder, ok := limitOrders[order.OrderFlag]
				if !ok {
					continue
				}
				if limitOrder.Status == model.OrderStatusTypeFilled {
					continue
				}
				if _, ok := p.assets[asset]; !ok {
					p.assets[asset] = &assetInfo{}
				}
				var orderPrice float64
				if (order.Type == model.OrderTypeStop || order.Type == model.OrderTypeStopMarket) && order.Price <= candle.High {
					orderPrice = order.Price
				} else if order.Type == model.OrderTypeMarket {
					orderPrice = order.Price
				} else {
					continue
				}
				// 查询对应的仓位,当前无仓位时不需要平仓
				positonOrder, err := p.findPositonOrder(order.Pair, order.OrderFlag, model.OrderTypeLimit)
				if err != nil {
					continue
				}

				p.volume[candle.Pair] += orderPrice * order.Quantity
				p.orders[i].UpdatedAt = candle.Time
				p.orders[i].Status = model.OrderStatusTypeFilled

				// update assets size
				p.updateAveragePrice(order.Side, order.Pair, order.Quantity, orderPrice)
				p.assets[asset].Free = 0
				p.assets[asset].Lock += order.Quantity

				// 释放锁定的基本资产
				lockQuote := positonOrder.Price * order.Quantity / leverage
				p.assets[quote].Lock -= lockQuote
				p.assets[quote].Free += lockQuote + (positonOrder.Price-orderPrice)*order.Quantity
			}
		}

		if order.Side == model.SideTypeSell {
			// 平多单
			if order.PositionSide == model.PositionSideTypeLong {
				limitOrder, ok := limitOrders[order.OrderFlag]
				if !ok {
					continue
				}
				if limitOrder.Status == model.OrderStatusTypeFilled {
					continue
				}
				if _, ok := p.assets[asset]; !ok {
					p.assets[asset] = &assetInfo{}
				}
				var orderPrice float64
				if (order.Type == model.OrderTypeStop || order.Type == model.OrderTypeStopMarket) && order.Price >= candle.Low {
					orderPrice = order.Price
				} else if order.Type == model.OrderTypeMarket {
					orderPrice = order.Price
				} else {
					continue
				}
				// 查询对应的仓位 当前无仓位时不需要平仓
				positonOrder, err := p.findPositonOrder(order.Pair, order.OrderFlag, model.OrderTypeLimit)
				if err != nil {
					continue
				}
				p.volume[candle.Pair] += orderPrice * order.Quantity
				p.orders[i].UpdatedAt = candle.Time
				p.orders[i].Status = model.OrderStatusTypeFilled

				// update assets size
				p.updateAveragePrice(order.Side, order.Pair, order.Quantity, orderPrice)
				p.assets[asset].Free = 0
				p.assets[asset].Lock -= order.Quantity

				// 释放锁定的基本资产
				lockQuote := positonOrder.Price * order.Quantity / leverage
				p.assets[quote].Lock -= lockQuote
				p.assets[quote].Free += lockQuote + (orderPrice-positonOrder.Price)*order.Quantity
			}
			// 开空单
			if order.PositionSide == model.PositionSideTypeShort && order.Price <= candle.High {
				if _, ok := p.assets[quote]; !ok {
					p.assets[quote] = &assetInfo{}
				}
				var orderPrice float64
				if order.Type == model.OrderTypeLimit {
					orderPrice = order.Price
				} else {
					continue
				}
				volume := orderPrice * order.Quantity

				// 锁定的资产
				lockQuote := volume / leverage
				if p.assets[quote].Free < lockQuote {
					utils.Log.Warn(ErrInsufficientFunds)
					continue
				}

				p.volume[candle.Pair] += volume
				p.orders[i].UpdatedAt = candle.Time
				p.orders[i].Status = model.OrderStatusTypeFilled

				limitOrders[p.orders[i].OrderFlag] = p.orders[i]

				// update assets size
				p.updateAveragePrice(order.Side, order.Pair, order.Quantity, orderPrice)
				p.assets[asset].Free = 0
				p.assets[asset].Lock -= order.Quantity

				p.assets[quote].Lock += lockQuote
				p.assets[quote].Free -= lockQuote
			}
		}
	}

	if candle.Complete {
		var total float64
		for asset, info := range p.assets {
			amount := info.Free + info.Lock
			pair := strings.ToUpper(asset + p.baseCoin)
			if amount < 0 {
				v := math.Abs(amount)
				liquid := 2*v*p.avgShortPrice[pair] - v*p.lastCandle[pair].Close
				total += liquid
			} else {
				total += amount * p.lastCandle[pair].Close
			}

			p.assetValues[asset] = append(p.assetValues[asset], AssetValue{
				Time:  candle.Time,
				Value: amount * p.lastCandle[pair].Close,
			})
		}

		baseCoinInfo := p.assets[p.baseCoin]
		p.equityValues = append(p.equityValues, AssetValue{
			Time:  candle.Time,
			Value: total + baseCoinInfo.Lock + baseCoinInfo.Free,
		})
	}
}

func (p *PaperWallet) Account() (model.Account, error) {
	balances := make([]model.Balance, 0)
	for pair, info := range p.assets {
		balances = append(balances, model.Balance{
			Asset: pair,
			Free:  info.Free,
			Lock:  info.Lock,
		})
	}

	return model.Account{
		Balances: balances,
	}, nil
}

func (p *PaperWallet) Position(pair string) (asset, quote float64, err error) {
	p.Lock()
	defer p.Unlock()

	assetTick, quoteTick := SplitAssetQuote(pair)
	acc, err := p.Account()
	if err != nil {
		return 0, 0, err
	}

	assetBalance, quoteBalance := acc.Balance(assetTick, quoteTick)

	return assetBalance.Free + assetBalance.Lock, quoteBalance.Free + quoteBalance.Lock, nil
}

func (p *PaperWallet) CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string,
	quantity float64, limit float64, extra model.OrderExtra) (model.Order, error) {

	p.Lock()
	defer p.Unlock()

	if quantity == 0 {
		return model.Order{}, ErrInvalidQuantity
	}
	orderFlag := strutil.RandomString(6)

	currentQuantity := p.FormatQuantity(pair, quantity, true)
	currentPrice := p.FormatPrice(pair, limit)
	err := p.validateFunds(side, positionSide, pair, currentQuantity, currentPrice)
	if err != nil {
		return model.Order{}, err
	}

	clientOrderId := strutil.RandomString(12)

	order := model.Order{
		ExchangeID:     p.ID(),
		ClientOrderId:  clientOrderId,
		OrderFlag:      orderFlag,
		OpenType:       "paperwallet",
		CreatedAt:      p.lastCandle[pair].Time,
		UpdatedAt:      p.lastCandle[pair].Time,
		Pair:           pair,
		Side:           side,
		PositionSide:   positionSide,
		Type:           model.OrderTypeLimit,
		Status:         model.OrderStatusTypeNew,
		Price:          currentPrice,
		Quantity:       currentQuantity,
		Leverage:       extra.Leverage,
		LongShortRatio: extra.LongShortRatio,
		MatchStrategy:  extra.MatchStrategy,
	}
	p.orders = append(p.orders, order)
	return order, nil
}

func (p *PaperWallet) CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, extra model.OrderExtra) (model.Order, error) {
	p.Lock()
	defer p.Unlock()
	if quantity == 0 {
		return model.Order{}, ErrInvalidQuantity
	}

	orderFlag := extra.OrderFlag
	if orderFlag == "" {
		orderFlag = strutil.RandomString(6)
	}

	currentQuantity := p.FormatQuantity(pair, quantity, true)
	currentPrice := p.FormatPrice(pair, p.lastCandle[pair].Close)
	err := p.validateFunds(side, positionSide, pair, currentQuantity, currentPrice)
	if err != nil {
		return model.Order{}, err
	}

	if _, ok := p.volume[pair]; !ok {
		p.volume[pair] = 0
	}

	p.volume[pair] += currentPrice * currentQuantity

	clientOrderId := strutil.RandomString(12)

	order := model.Order{
		ExchangeID:     p.ID(),
		ClientOrderId:  clientOrderId,
		OrderFlag:      orderFlag,
		OpenType:       "paperwallet",
		CreatedAt:      p.lastCandle[pair].Time,
		UpdatedAt:      p.lastCandle[pair].Time,
		Pair:           pair,
		Side:           side,
		PositionSide:   positionSide,
		Type:           model.OrderTypeMarket,
		Status:         model.OrderStatusTypeFilled,
		Price:          currentPrice,
		Quantity:       currentQuantity,
		Leverage:       extra.Leverage,
		LongShortRatio: extra.LongShortRatio,
		MatchStrategy:  extra.MatchStrategy,
	}
	err = p.updateFunds(&order)
	if err != nil {
		return model.Order{}, err
	}

	p.orders = append(p.orders, order)

	return order, nil
}

func (p *PaperWallet) CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string,
	quantity float64, limit float64, stopPrice float64, extra model.OrderExtra) (model.Order, error) {
	p.Lock()
	defer p.Unlock()

	if quantity == 0 {
		return model.Order{}, ErrInvalidQuantity
	}

	currentQuantity := p.FormatQuantity(pair, quantity, false)
	currentPrice := p.FormatPrice(pair, limit)
	err := p.validateFunds(side, positionSide, pair, currentQuantity, currentPrice)
	if err != nil {
		return model.Order{}, err
	}

	clientOrderId := strutil.RandomString(12)

	order := model.Order{
		ExchangeID:     p.ID(),
		ClientOrderId:  clientOrderId,
		OrderFlag:      extra.OrderFlag,
		OpenType:       "paperwallet",
		CreatedAt:      p.lastCandle[pair].Time,
		UpdatedAt:      p.lastCandle[pair].Time,
		Pair:           pair,
		Side:           side,
		PositionSide:   positionSide,
		Type:           model.OrderTypeStop,
		Status:         model.OrderStatusTypeNew,
		Price:          currentPrice,
		Quantity:       currentQuantity,
		Leverage:       extra.Leverage,
		LongShortRatio: extra.LongShortRatio,
		MatchStrategy:  extra.MatchStrategy,
	}

	p.orders = append(p.orders, order)
	return order, nil
}

func (p *PaperWallet) CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string,
	quantity float64, stopPrice float64, extra model.OrderExtra) (model.Order, error) {
	p.Lock()
	defer p.Unlock()

	if quantity == 0 {
		return model.Order{}, ErrInvalidQuantity
	}

	currentQuantity := p.FormatQuantity(pair, quantity, false)
	currentPrice := p.FormatPrice(pair, stopPrice)
	if positionSide == model.PositionSideTypeLong {
		// 判断触发时机，当前价格小于触发价格时直接平掉
		if stopPrice >= p.lastCandle[pair].Close {
			currentPrice = p.FormatPrice(pair, p.lastCandle[pair].Close)
		}
	} else {
		if stopPrice <= p.lastCandle[pair].Close {
			currentPrice = p.FormatPrice(pair, p.lastCandle[pair].Close)
		}
	}
	err := p.validateFunds(side, positionSide, pair, currentQuantity, currentPrice)
	if err != nil {
		return model.Order{}, err
	}
	clientOrderId := strutil.RandomString(12)

	order := model.Order{
		ExchangeID:     p.ID(),
		ClientOrderId:  clientOrderId,
		OrderFlag:      extra.OrderFlag,
		OpenType:       "paperwallet",
		CreatedAt:      p.lastCandle[pair].Time,
		UpdatedAt:      p.lastCandle[pair].Time,
		Pair:           pair,
		Side:           side,
		PositionSide:   positionSide,
		Type:           model.OrderTypeStopMarket,
		Status:         model.OrderStatusTypeNew,
		Price:          currentPrice,
		Quantity:       currentQuantity,
		Leverage:       extra.Leverage,
		LongShortRatio: extra.LongShortRatio,
		MatchStrategy:  extra.MatchStrategy,
	}

	p.orders = append(p.orders, order)
	return order, nil
}

func (p *PaperWallet) Cancel(order model.Order) error {
	p.Lock()
	defer p.Unlock()

	for i, o := range p.orders {
		if o.ExchangeID == order.ExchangeID {
			p.orders[i].Status = model.OrderStatusTypeCanceled
		}
	}
	return nil
}

func (p *PaperWallet) Orders(pair string) ([]model.Order, error) {
	orders := make([]model.Order, 0)
	for _, order := range p.orders {
		if order.Pair == pair {
			orders = append(orders, order)
		}
	}
	return orders, nil
}

func (p *PaperWallet) Order(_ string, id int64) (model.Order, error) {
	for _, order := range p.orders {
		if order.ExchangeID == id {
			return order, nil
		}
	}
	return model.Order{}, errors.New("current order not found")
}

func (p *PaperWallet) findPositonOrder(pair string, orderFlag string, orderType model.OrderType) (model.Order, error) {
	for _, order := range p.orders {
		if order.Pair == pair && order.Status == model.OrderStatusTypeFilled && order.OrderFlag == orderFlag && order.Type == orderType {
			return order, nil
		}
	}
	return model.Order{}, errors.New("order not found")
}

func (p *PaperWallet) GetOrdersForPostionLossUnfilled(_ string) ([]*model.Order, error) {
	//TODO implement me
	panic("implement me")
}
func (b *PaperWallet) GetOrdersForUnfilled() ([]*model.Order, error) {
	//TODO implement me
	panic("implement me")
}
func (b *PaperWallet) GetOrdersForPairUnfilled(pair string) ([]*model.Order, error) {
	//TODO implement me
	panic("implement me")
}
func (b *PaperWallet) GetPositionsForPair(pair string) ([]*model.Position, error) {
	//TODO implement me
	panic("implement me")
}
func (b *PaperWallet) GetPositionsForOpened() ([]*model.Position, error) {
	//TODO implement me
	panic("implement me")
}

func (p *PaperWallet) FormatPrice(pair string, value float64) float64 {
	info := p.AssetsInfo(pair)
	return common.AmountToLotSize(info.TickSize, info.QuotePrecision, value)
}

func (p *PaperWallet) FormatQuantity(pair string, value float64, toLot bool) float64 {
	if toLot {
		//info := p.AssetsInfo(pair)
		//return common.AmountToLotSize(info.StepSize, info.BaseAssetPrecision, value)
		formatNumber := strconv.FormatFloat(value, 'f', -1, 64)
		// 将字符串转换回 float64
		intNumber, err := strconv.ParseFloat(formatNumber, 64)
		if err != nil {
			fmt.Println("Error parsing float:", err)
			return 0
		}
		return intNumber
	}
	return value
}

func (p *PaperWallet) CandlesByPeriod(ctx context.Context, pair, period string,
	start, end time.Time) ([]model.Candle, error) {
	return p.feeder.CandlesByPeriod(ctx, pair, period, start, end)
}

func (p *PaperWallet) CandlesByLimit(ctx context.Context, pair, period string, limit int) ([]model.Candle, error) {
	return p.feeder.CandlesByLimit(ctx, pair, period, limit)
}

func (p *PaperWallet) CandlesSubscription(ctx context.Context, pair, timeframe string) (chan model.Candle, chan error) {
	return p.feeder.CandlesSubscription(ctx, pair, timeframe)
}
