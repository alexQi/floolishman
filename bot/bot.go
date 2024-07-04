package bot

import (
	"context"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/notification"
	"floolishman/reference"
	"floolishman/service"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"fmt"
	"sync"
)

const defaultDatabase = "floolisher.db"

type OrderSubscriber interface {
	OnOrder(model.Order)
}

type CandleSubscriber interface {
	OnCandle(string, model.Candle)
}

type Bot struct {
	storage  storage.Storage
	settings model.Settings
	exchange reference.Exchange
	notifier reference.Notifier
	telegram reference.Telegram
	strategy types.CompositesStrategy

	orderService         *service.OrderService
	strategyService      *service.StrategyService
	priorityQueueCandles map[string]map[string]*model.PriorityQueue // [pair] [] queue
	orderFeed            *model.Feed
	dataFeed             *exchange.DataFeedSubscription
	mu                   sync.Mutex
}

type Option func(*Bot)

func NewBot(ctx context.Context, settings model.Settings, exch reference.Exchange, strategy types.CompositesStrategy,
	options ...Option) (*Bot, error) {
	// 初始化bot参数
	bot := &Bot{
		settings:             settings,
		exchange:             exch,
		strategy:             strategy,
		orderFeed:            model.NewOrderFeed(),
		dataFeed:             exchange.NewDataFeed(exch),
		priorityQueueCandles: map[string]map[string]*model.PriorityQueue{},
	}
	// 加载用户配置
	for _, option := range options {
		option(bot)
	}
	// 加载storage
	var err error
	if bot.storage == nil {
		bot.storage, err = storage.FromFile(defaultDatabase)
		if err != nil {
			return nil, err
		}
	}
	// 加载订单服务
	bot.orderService = service.NewOrderService(ctx, exch, bot.storage, bot.orderFeed)
	// 加载策略服务
	bot.strategyService = service.NewStrategyService(ctx, strategy, bot.orderService)
	// 加载通知服务
	if settings.Telegram.Enabled {
		bot.telegram, err = notification.NewTelegram(bot.orderService, settings)
		if err != nil {
			return nil, err
		}
		// register telegram as notifier
		WithNotifier(bot.telegram)(bot)
	}

	return bot, nil
}

// WithStorage sets the storage for the bot, by default it uses a local file called ninjabot.db
func WithStorage(storage storage.Storage) Option {
	return func(bot *Bot) {
		bot.storage = storage
	}
}

// WithNotifier registers a notifier to the bot, currently only email and telegram are supported
func WithNotifier(notifier reference.Notifier) Option {
	return func(bot *Bot) {
		bot.notifier = notifier
		bot.orderService.SetNotifier(notifier)
		bot.SubscribeOrder(notifier)
	}
}

// WithCandleSubscription subscribes a given struct to the candle feed
func WithCandleSubscription(subscriber CandleSubscriber) Option {
	return func(bot *Bot) {
		bot.SubscribeCandle(subscriber)
	}
}

func (n *Bot) SubscribeCandle(subscriptions ...CandleSubscriber) {
	for _, option := range n.settings.PairOptions {
		for _, subscription := range subscriptions {
			for _, s := range n.strategy.Strategies {
				n.dataFeed.Subscribe(option.Pair, s.Timeframe(), subscription.OnCandle, false)
			}
		}
	}
}

func WithOrderSubscription(subscriber OrderSubscriber) Option {
	return func(bot *Bot) {
		bot.SubscribeOrder(subscriber)
	}
}

func (n *Bot) SubscribeOrder(subscriptions ...OrderSubscriber) {
	for _, option := range n.settings.PairOptions {
		for _, subscription := range subscriptions {
			n.orderFeed.Subscribe(option.Pair, subscription.OnOrder, false)
		}
	}
}

func (n *Bot) OrderService() *service.OrderService {
	return n.orderService
}

func (n Bot) SaveReturns(outputDir string) error {
	for _, summary := range n.orderService.Results {
		outputFile := fmt.Sprintf("%s/%s.csv", outputDir, summary.Pair)
		if err := summary.SaveReturns(outputFile); err != nil {
			return err
		}
	}
	return nil
}

func (n *Bot) onCandle(timeframe string, candle model.Candle) {
	n.priorityQueueCandles[candle.Pair][timeframe].Push(candle)
}

func (n *Bot) processCandle(timeframe string, candle model.Candle) {
	if candle.Complete {
		n.strategyService.OnCandle(timeframe, candle)
		n.orderService.OnCandle(candle)
	} else {
		n.strategyService.OnRealCandle(timeframe, candle)
	}
}

// Process pending candles in buffer
func (n *Bot) processCandles(pair string, timeframe string) {
	for item := range n.priorityQueueCandles[pair][timeframe].PopLock() {
		n.processCandle(timeframe, item.(model.Candle))
	}
}

// Before Ninjabot start, we need to load the necessary data to fill strategy indicators
// Then, we need to get the time frame and warmup period to fetch the necessary candles
func (n *Bot) preload(ctx context.Context, pair string, timeframe string, period int) error {
	candles, err := n.exchange.CandlesByLimit(ctx, pair, timeframe, period)
	if err != nil {
		return err
	}

	for _, candle := range candles {
		n.processCandle(timeframe, candle)
	}
	n.dataFeed.Preload(pair, timeframe, candles)

	return nil
}

func (n *Bot) SettingPairs(ctx context.Context) {
	for _, option := range n.settings.PairOptions {
		err := n.exchange.SetPairOption(ctx, option)
		if err != nil {
			utils.Log.Error(err)
			return
		}
		n.strategyService.SetPairDataframe(option)

		if n.priorityQueueCandles[option.Pair] == nil {
			n.priorityQueueCandles[option.Pair] = make(map[string]*model.PriorityQueue)
		}
		// init loading data by http api
		timeframaMap := n.strategy.TimeWarmupMap()
		for timeframe, period := range timeframaMap {
			// link to ninja bot controller
			n.priorityQueueCandles[option.Pair][timeframe] = model.NewPriorityQueue(nil)
			// preload candles for warmup perio
			err = n.preload(ctx, option.Pair, timeframe, period)
			if err != nil {
				utils.Log.Error(err)
				return
			}
			// link to ninja bot controller
			n.dataFeed.Subscribe(option.Pair, timeframe, n.onCandle, false)
		}

		// link to ninja bot controller
		//for _, s := range n.strategy.Strategies {
		//	n.dataFeed.Subscribe(option.Pair, s.Timeframe(), n.onCandle, false)
		//}
	}
	n.strategyService.Start()
}

// Run will initialize the strategy controller, order controller, preload data and start the bot
func (n *Bot) Run(ctx context.Context) {
	n.orderFeed.Start()
	n.orderService.Start()
	defer n.orderService.Stop()

	utils.Log.Infof("Loaded with %d Strategy.", len(n.strategy.Strategies))

	n.SettingPairs(ctx)

	// start data feed and receives new candles
	n.dataFeed.Start()

	// Start notifies
	if n.telegram != nil {
		n.telegram.Start()
	}

	for _, option := range n.settings.PairOptions {
		timeframaMap := n.strategy.TimeWarmupMap()
		for timeframe := range timeframaMap {
			go n.processCandles(option.Pair, timeframe)
		}
	}

	select {}
}
