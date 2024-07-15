package exchange

import (
	"context"
	"floolishman/utils"
	"floolishman/utils/strutil"
	"fmt"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/common"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/jpillora/backoff"

	"floolishman/model"
)

var ErrNoNeedChangeMarginType int64 = -4046

type ProxyOption struct {
	Status bool
	Url    string
}

type BinanceFuture struct {
	ctx        context.Context
	client     *futures.Client
	assetsInfo map[string]model.AssetInfo
	HeikinAshi bool
	Testnet    bool
	DebugMode  bool

	APIKeyType string
	APIKey     string
	APISecret  string

	ProxyOption ProxyOption

	MetadataFetchers []MetadataFetchers
	PairOptions      []model.PairOption
}

type BinanceFutureOption func(*BinanceFuture)

func WithBinanceFutureTestnet() BinanceFutureOption {
	return func(b *BinanceFuture) {
		b.Testnet = true
	}
}

// WithBinanceFuturesHeikinAshiCandle will use Heikin Ashi candle instead of regular candle
func WithBinanceFuturesHeikinAshiCandle() BinanceFutureOption {
	return func(b *BinanceFuture) {
		b.HeikinAshi = true
	}
}

func WithBinanceFuturesDebugMode() BinanceFutureOption {
	return func(b *BinanceFuture) {
		b.DebugMode = true
	}
}

// WithBinanceFutureCredentials will set the credentials for Binance Futures
func WithBinanceFutureCredentials(key, secret string, keyType string) BinanceFutureOption {
	return func(b *BinanceFuture) {
		b.APIKey = key
		b.APISecret = secret
		b.APIKeyType = keyType
	}
}

// WithBinanceFutureCredentials will set the credentials for Binance Futures
func WithBinanceFutureProxy(proxyUrl string) BinanceFutureOption {
	return func(b *BinanceFuture) {
		b.ProxyOption = ProxyOption{
			Status: true,
			Url:    proxyUrl,
		}
	}
}

// NewBinanceFuture will create a new BinanceFuture instance
func NewBinanceFuture(ctx context.Context, options ...BinanceFutureOption) (*BinanceFuture, error) {
	binance.WebsocketKeepalive = true
	exchange := &BinanceFuture{ctx: ctx}
	for _, option := range options {
		option(exchange)
	}

	futures.UseTestnet = exchange.Testnet

	if exchange.ProxyOption.Status {
		exchange.client = futures.NewProxiedClient(exchange.APIKey, exchange.APISecret, exchange.ProxyOption.Url)
	} else {
		exchange.client = futures.NewClient(exchange.APIKey, exchange.APISecret)
	}

	exchange.client.KeyType = exchange.APIKeyType
	exchange.client.Debug = exchange.DebugMode

	err := exchange.client.NewPingService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("binance ping fail: %w", err)
	}

	results, err := exchange.client.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize with orders precision and assets limits
	exchange.assetsInfo = make(map[string]model.AssetInfo)
	for _, info := range results.Symbols {
		tradeLimits := model.AssetInfo{
			BaseAsset:          info.BaseAsset,
			QuoteAsset:         info.QuoteAsset,
			BaseAssetPrecision: info.BaseAssetPrecision,
			QuotePrecision:     info.QuotePrecision,
		}
		for _, filter := range info.Filters {
			if typ, ok := filter["filterType"]; ok {
				if typ == string(binance.SymbolFilterTypeLotSize) {
					tradeLimits.MinQuantity, _ = strconv.ParseFloat(filter["minQty"].(string), 64)
					tradeLimits.MaxQuantity, _ = strconv.ParseFloat(filter["maxQty"].(string), 64)
					tradeLimits.StepSize, _ = strconv.ParseFloat(filter["stepSize"].(string), 64)
				}

				if typ == string(binance.SymbolFilterTypePriceFilter) {
					tradeLimits.MinPrice, _ = strconv.ParseFloat(filter["minPrice"].(string), 64)
					tradeLimits.MaxPrice, _ = strconv.ParseFloat(filter["maxPrice"].(string), 64)
					tradeLimits.TickSize, _ = strconv.ParseFloat(filter["tickSize"].(string), 64)
				}
			}
		}
		exchange.assetsInfo[info.Symbol] = tradeLimits
	}

	utils.Log.Info("[SETUP] Using Binance Futures exchange")

	return exchange, nil
}

func (b *BinanceFuture) SetPairOption(ctx context.Context, option model.PairOption) error {
	_, err := b.client.NewChangeLeverageService().Symbol(option.Pair).Leverage(option.Leverage).Do(ctx)
	if err != nil {
		return err
	}

	err = b.client.NewChangeMarginTypeService().Symbol(option.Pair).MarginType(option.MarginType).Do(ctx)
	if err != nil {
		if apiError, ok := err.(*common.APIError); !ok || apiError.Code != ErrNoNeedChangeMarginType {
			return err
		}
	}
	return nil
}

func (b *BinanceFuture) LastQuote(ctx context.Context, pair string) (float64, error) {
	candles, err := b.CandlesByLimit(ctx, pair, "1m", 1)
	if err != nil || len(candles) < 1 {
		return 0, err
	}
	return candles[0].Close, nil
}

func (b *BinanceFuture) AssetsInfo(pair string) model.AssetInfo {
	return b.assetsInfo[pair]
}

func (b *BinanceFuture) validate(pair string, quantity float64) error {
	info, ok := b.assetsInfo[pair]
	if !ok {
		return ErrInvalidAsset
	}

	if quantity > info.MaxQuantity || quantity < info.MinQuantity {
		return &OrderError{
			Err:      fmt.Errorf("%w: min: %f max: %f ,current:%f", ErrInvalidQuantity, info.MinQuantity, info.MaxQuantity, quantity),
			Pair:     pair,
			Quantity: quantity,
		}
	}

	return nil
}

func (b *BinanceFuture) formatPrice(pair string, value float64) string {
	if info, ok := b.assetsInfo[pair]; ok {
		value = common.AmountToLotSize(info.TickSize, info.QuotePrecision, value)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (b *BinanceFuture) formatQuantity(pair string, value float64, toLot bool) string {
	if toLot {
		if info, ok := b.assetsInfo[pair]; ok {
			value = common.AmountToLotSize(info.StepSize, info.BaseAssetPrecision, value)
		}
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (b *BinanceFuture) CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string,
	quantity float64, limit float64, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {

	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	clientOrderId := strutil.RandomString(12)
	orderFlag := strutil.RandomString(6)
	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		NewClientOrderID(clientOrderId).
		Type(futures.OrderTypeLimit).
		TimeInForce(futures.TimeInForceTypeGTC).
		Side(futures.SideType(side)).
		PositionSide(futures.PositionSideType(positionSide)).
		Quantity(b.formatQuantity(pair, quantity, true)).
		Price(b.formatPrice(pair, limit)).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	price, err := strconv.ParseFloat(order.Price, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.OrigQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID:     order.OrderID,
		ClientOrderId:  clientOrderId,
		OrderFlag:      orderFlag,
		CreatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		UpdatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		Pair:           pair,
		Side:           model.SideType(order.Side),
		PositionSide:   model.PositionSideType(order.PositionSide),
		Type:           model.OrderType(order.Type),
		Status:         model.OrderStatusType(order.Status),
		Price:          price,
		Quantity:       quantity,
		LongShortRatio: longShortRatio,
		MatchStrategy:  matchStrategy,
	}, nil
}

func (b *BinanceFuture) CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {
	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}
	clientOrderId := strutil.RandomString(12)
	orderFlag := strutil.RandomString(6)
	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		NewClientOrderID(clientOrderId).
		Type(futures.OrderTypeMarket).
		Side(futures.SideType(side)).
		PositionSide(futures.PositionSideType(positionSide)).
		Quantity(b.formatQuantity(pair, quantity, true)).
		NewOrderResponseType(futures.NewOrderRespTypeRESULT).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	cost, err := strconv.ParseFloat(order.CumQuote, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.ExecutedQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID:     order.OrderID,
		ClientOrderId:  clientOrderId,
		OrderFlag:      orderFlag,
		CreatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		UpdatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		Pair:           order.Symbol,
		Side:           model.SideType(order.Side),
		PositionSide:   model.PositionSideType(order.PositionSide),
		Type:           model.OrderType(order.Type),
		Status:         model.OrderStatusType(order.Status),
		Price:          cost / quantity,
		Quantity:       quantity,
		LongShortRatio: longShortRatio,
		MatchStrategy:  matchStrategy,
	}, nil
}

func (b *BinanceFuture) CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string,
	quantity float64, limit float64, stopPrice float64, orderFlag string, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {

	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	clientOrderId := strutil.RandomString(12)
	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		NewClientOrderID(clientOrderId).
		Type(futures.OrderTypeStop).
		TimeInForce(futures.TimeInForceTypeGTC).
		Side(futures.SideType(side)).
		PositionSide(futures.PositionSideType(positionSide)).
		Quantity(b.formatQuantity(pair, quantity, false)).
		WorkingType(futures.WorkingTypeMarkPrice).
		StopPrice(b.formatPrice(pair, stopPrice)).
		Price(b.formatPrice(pair, limit)).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	price, err := strconv.ParseFloat(order.Price, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.OrigQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID:     order.OrderID,
		ClientOrderId:  clientOrderId,
		OrderFlag:      orderFlag,
		CreatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		UpdatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		Pair:           pair,
		Side:           model.SideType(order.Side),
		PositionSide:   model.PositionSideType(order.PositionSide),
		Type:           model.OrderType(order.Type),
		Status:         model.OrderStatusType(order.Status),
		Price:          price,
		Quantity:       quantity,
		LongShortRatio: longShortRatio,
		MatchStrategy:  matchStrategy,
	}, nil
}

func (b *BinanceFuture) CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string,
	quantity float64, stopPrice float64, orderFlag string, longShortRatio float64, matchStrategy map[string]int) (model.Order, error) {

	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	clientOrderId := strutil.RandomString(12)
	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		NewClientOrderID(clientOrderId).
		Type(futures.OrderTypeStopMarket).
		TimeInForce(futures.TimeInForceTypeGTC).
		Side(futures.SideType(side)).
		PositionSide(futures.PositionSideType(positionSide)).
		Quantity(b.formatQuantity(pair, quantity, false)).
		WorkingType(futures.WorkingTypeMarkPrice).
		StopPrice(b.formatPrice(pair, stopPrice)).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	price, err := strconv.ParseFloat(order.Price, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.OrigQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID:     order.OrderID,
		ClientOrderId:  clientOrderId,
		OrderFlag:      orderFlag,
		CreatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		UpdatedAt:      time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		Pair:           pair,
		Side:           model.SideType(order.Side),
		PositionSide:   model.PositionSideType(order.PositionSide),
		Type:           model.OrderType(order.Type),
		Status:         model.OrderStatusType(order.Status),
		Price:          price,
		Quantity:       quantity,
		LongShortRatio: longShortRatio,
		MatchStrategy:  matchStrategy,
	}, nil
}

func (b *BinanceFuture) Cancel(order model.Order) error {
	_, err := b.client.NewCancelOrderService().
		Symbol(order.Pair).
		OrderID(order.ExchangeID).
		Do(b.ctx)
	return err
}

func (b *BinanceFuture) Orders(pair string, limit int) ([]model.Order, error) {
	result, err := b.client.NewListOrdersService().
		Symbol(pair).
		Limit(limit).
		Do(b.ctx)

	if err != nil {
		return nil, err
	}

	orders := make([]model.Order, 0)
	for _, order := range result {
		orders = append(orders, b.newFutureOrder(order))
	}
	return orders, nil
}

func (b *BinanceFuture) Order(pair string, id int64) (model.Order, error) {
	order, err := b.client.NewGetOrderService().
		Symbol(pair).
		OrderID(id).
		Do(b.ctx)

	if err != nil {
		return model.Order{}, err
	}

	return b.newFutureOrder(order), nil
}

func (p *BinanceFuture) ListenUpdateOrders() {
	//TODO implement me
	panic("implement me")
}

func (b *BinanceFuture) GetCurrentPositionOrders(pair string) ([]*model.Order, error) {
	//TODO implement me
	panic("implement me")
}

func (b *BinanceFuture) newFutureOrder(order *futures.Order) model.Order {
	var (
		price float64
		err   error
	)
	cost, _ := strconv.ParseFloat(order.CumQuote, 64)
	quantity, _ := strconv.ParseFloat(order.ExecutedQuantity, 64)
	if cost > 0 && quantity > 0 {
		price = cost / quantity
	} else {
		price, err = strconv.ParseFloat(order.Price, 64)
		if err != nil {
			utils.Log.Warn(err)
		}
		quantity, err = strconv.ParseFloat(order.OrigQuantity, 64)
		if err != nil {
			utils.Log.Warn(err)
		}
	}

	return model.Order{
		ExchangeID:    order.OrderID,
		ClientOrderId: order.ClientOrderID,
		Pair:          order.Symbol,
		CreatedAt:     time.Unix(0, order.Time*int64(time.Millisecond)),
		UpdatedAt:     time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		Side:          model.SideType(order.Side),
		PositionSide:  model.PositionSideType(order.PositionSide),
		Type:          model.OrderType(order.Type),
		Status:        model.OrderStatusType(order.Status),
		Price:         price,
		Quantity:      quantity,
	}
}

func (b *BinanceFuture) Account() (model.Account, error) {
	acc, err := b.client.NewGetAccountService().Do(b.ctx)
	if err != nil {
		return model.Account{}, err
	}

	balances := make([]model.Balance, 0)
	for _, position := range acc.Positions {
		free, err := strconv.ParseFloat(position.PositionAmt, 64)
		if err != nil {
			return model.Account{}, err
		}

		if free == 0 {
			continue
		}

		leverage, err := strconv.ParseFloat(position.Leverage, 64)
		if err != nil {
			return model.Account{}, err
		}

		if position.PositionSide == futures.PositionSideTypeShort {
			free = -free
		}

		asset, _ := SplitAssetQuote(position.Symbol)

		balances = append(balances, model.Balance{
			Asset:    asset,
			Free:     free,
			Leverage: leverage,
		})
	}

	for _, asset := range acc.Assets {
		free, err := strconv.ParseFloat(asset.WalletBalance, 64)
		if err != nil {
			return model.Account{}, err
		}

		if free == 0 {
			continue
		}

		balances = append(balances, model.Balance{
			Asset: asset.Asset,
			Free:  free,
		})
	}

	return model.Account{
		Balances: balances,
	}, nil
}

func (b *BinanceFuture) Position(pair string) (asset, quote float64, err error) {
	assetTick, quoteTick := SplitAssetQuote(pair)
	acc, err := b.Account()
	if err != nil {
		return 0, 0, err
	}

	assetBalance, quoteBalance := acc.Balance(assetTick, quoteTick)

	return assetBalance.Free + assetBalance.Lock, quoteBalance.Free + quoteBalance.Lock, nil
}

func (b *BinanceFuture) CandlesSubscription(ctx context.Context, pair, period string) (chan model.Candle, chan error) {
	ccandle := make(chan model.Candle)
	cerr := make(chan error)
	ha := model.NewHeikinAshi()

	go func() {
		ba := &backoff.Backoff{
			Min: 100 * time.Millisecond,
			Max: 1 * time.Second,
		}

		for {
			if b.ProxyOption.Status {
				futures.SetWsProxyUrl(b.ProxyOption.Url)
			}
			done, _, err := futures.WsKlineServe(pair, period, func(event *futures.WsKlineEvent) {
				ba.Reset()
				candle := FutureCandleFromWsKline(pair, event.Kline)

				if candle.Complete && b.HeikinAshi {
					candle = candle.ToHeikinAshi(ha)
				}

				if candle.Complete {
					// fetch aditional data if needed
					for _, fetcher := range b.MetadataFetchers {
						key, value := fetcher(pair, candle.Time)
						candle.Metadata[key] = value
					}
				}
				ccandle <- candle
			}, func(err error) {
				cerr <- err
			})
			if err != nil {
				cerr <- err
				close(cerr)
				close(ccandle)
				return
			}

			select {
			case <-ctx.Done():
				close(cerr)
				close(ccandle)
				return
			case <-done:
				time.Sleep(ba.Duration())
			}
		}
	}()

	return ccandle, cerr
}

func (b *BinanceFuture) CandlesByLimit(ctx context.Context, pair, period string, limit int) ([]model.Candle, error) {
	candles := make([]model.Candle, 0)
	klineService := b.client.NewKlinesService()
	ha := model.NewHeikinAshi()

	data, err := klineService.Symbol(pair).
		Interval(period).
		Limit(limit + 1).
		Do(ctx)

	if err != nil {
		return nil, err
	}

	for _, d := range data {
		candle := FutureCandleFromKline(pair, *d)

		if b.HeikinAshi {
			candle = candle.ToHeikinAshi(ha)
		}

		candles = append(candles, candle)
	}

	// discard last candle, because it is incomplete
	return candles[:len(candles)-1], nil
}

func (b *BinanceFuture) CandlesByPeriod(ctx context.Context, pair, period string,
	start, end time.Time) ([]model.Candle, error) {

	candles := make([]model.Candle, 0)
	klineService := b.client.NewKlinesService()
	ha := model.NewHeikinAshi()

	data, err := klineService.Symbol(pair).
		Interval(period).
		StartTime(start.UnixNano() / int64(time.Millisecond)).
		EndTime(end.UnixNano() / int64(time.Millisecond)).
		Do(ctx)

	if err != nil {
		return nil, err
	}

	for _, d := range data {
		candle := FutureCandleFromKline(pair, *d)

		if b.HeikinAshi {
			candle = candle.ToHeikinAshi(ha)
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

func FutureCandleFromKline(pair string, k futures.Kline) model.Candle {
	var err error
	t := time.Unix(0, k.OpenTime*int64(time.Millisecond))
	candle := model.Candle{Pair: pair, Time: t, UpdatedAt: time.Now()}
	candle.Open, err = strconv.ParseFloat(k.Open, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Close, err = strconv.ParseFloat(k.Close, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.High, err = strconv.ParseFloat(k.High, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Low, err = strconv.ParseFloat(k.Low, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Volume, err = strconv.ParseFloat(k.Volume, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Complete = true
	candle.Metadata = make(map[string]float64)
	return candle
}

func FutureCandleFromWsKline(pair string, k futures.WsKline) model.Candle {
	var err error
	t := time.Unix(0, k.StartTime*int64(time.Millisecond))
	candle := model.Candle{Pair: pair, Time: t, UpdatedAt: time.Now()}
	candle.Open, err = strconv.ParseFloat(k.Open, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Close, err = strconv.ParseFloat(k.Close, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.High, err = strconv.ParseFloat(k.High, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Low, err = strconv.ParseFloat(k.Low, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Volume, err = strconv.ParseFloat(k.Volume, 64)
	if err != nil {
		utils.Log.Warn(err)
	}
	candle.Complete = k.IsFinal
	candle.Metadata = make(map[string]float64)
	return candle
}
