package bot

import (
	"bytes"
	"context"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/notification"
	"floolishman/reference"
	"floolishman/service"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/metrics"
	"fmt"
	"github.com/aybabtme/uniplot/histogram"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type OrderSubscriber interface {
	OnOrder(model.Order)
}

type CandleSubscriber interface {
	OnCandle(string, model.Candle)
}

type Bot struct {
	backtest        bool
	storage         storage.Storage
	settings        model.Settings
	strategySetting service.StrategySetting
	exchange        reference.Exchange
	notifier        reference.Notifier
	telegram        reference.Telegram
	strategy        types.CompositesStrategy
	paperWallet     *exchange.PaperWallet

	serviceOrder         *service.ServiceOrder
	serviceStrategy      *service.ServiceStrategy
	priorityQueueCandles map[string]map[string]*model.PriorityQueue // [pair] [] queue
	orderFeed            *model.Feed
	dataFeed             *exchange.DataFeedSubscription
	mu                   sync.Mutex
}

type Option func(*Bot)

func NewBot(ctx context.Context, settings model.Settings, exch reference.Exchange, strategySetting service.StrategySetting, strategy types.CompositesStrategy,
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
		storagePath := viper.GetString("storage.path")
		dir := filepath.Dir(storagePath)
		// 判断文件目录是否存在
		_, err := os.Stat(dir)
		if err != nil {
			err = os.MkdirAll(dir, os.ModePerm)
			if err != nil {
				utils.Log.Panicf("mkdir error : %s", err.Error())
			}
		}
		bot.storage, err = storage.FromFile(storagePath)
		if err != nil {
			return nil, err
		}
	}
	// 加载订单服务
	bot.serviceOrder = service.NewServiceOrder(ctx, exch, bot.storage, bot.orderFeed)
	// 加载策略服务
	bot.serviceStrategy = service.NewServiceStrategy(ctx, strategySetting, strategy, bot.serviceOrder, bot.backtest)
	// 加载通知服务
	if settings.Telegram.Enabled {
		bot.telegram, err = notification.NewTelegram(bot.serviceOrder, settings)
		if err != nil {
			return nil, err
		}
		// register telegram as notifier
		WithNotifier(bot.telegram)(bot)
	}

	return bot, nil
}

// WithBacktest sets the bot to run in backtest mode, it is required for backtesting environments
// Backtest mode optimize the input read for CSV and deal with race conditions
func WithBacktest(wallet *exchange.PaperWallet) Option {
	return func(bot *Bot) {
		bot.backtest = true
		opt := WithPaperWallet(wallet)
		opt(bot)
	}
}

// WithPaperWallet sets the paper wallet for the bot (used for backtesting and live simulation)
func WithPaperWallet(wallet *exchange.PaperWallet) Option {
	return func(bot *Bot) {
		bot.paperWallet = wallet
	}
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
		bot.serviceOrder.SetNotifier(notifier)
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

func (n *Bot) OrderService() *service.ServiceOrder {
	return n.serviceOrder
}

func (n *Bot) Summary() {
	var (
		total  float64
		wins   int
		loses  int
		volume float64
		sqn    float64
	)

	buffer := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buffer)
	table.SetHeader([]string{"Pair", "Trades", "Win", "Loss", "% Win", "Payoff", "Pr Fact.", "SQN", "Profit", "Volume"})
	table.SetFooterAlignment(tablewriter.ALIGN_RIGHT)
	avgPayoff := 0.0
	avgProfitFactor := 0.0

	returns := make([]float64, 0)
	for _, summary := range n.serviceOrder.Results {
		avgPayoff += summary.Payoff() * float64(len(summary.Win())+len(summary.Lose()))
		avgProfitFactor += summary.ProfitFactor() * float64(len(summary.Win())+len(summary.Lose()))
		table.Append([]string{
			summary.Pair,
			strconv.Itoa(len(summary.Win()) + len(summary.Lose())),
			strconv.Itoa(len(summary.Win())),
			strconv.Itoa(len(summary.Lose())),
			fmt.Sprintf("%.1f %%", float64(len(summary.Win()))/float64(len(summary.Win())+len(summary.Lose()))*100),
			fmt.Sprintf("%.3f", summary.Payoff()),
			fmt.Sprintf("%.3f", summary.ProfitFactor()),
			fmt.Sprintf("%.1f", summary.SQN()),
			fmt.Sprintf("%.2f", summary.Profit()),
			fmt.Sprintf("%.2f", summary.Volume),
		})
		total += summary.Profit()
		sqn += summary.SQN()
		wins += len(summary.Win())
		loses += len(summary.Lose())
		volume += summary.Volume

		returns = append(returns, summary.WinPercent()...)
		returns = append(returns, summary.LosePercent()...)

		fmt.Println("------ CALLED STRATEGY -------")
		fmt.Printf("[ WinLong ] Pair: %s, %+v\n", summary.Pair, summary.WinLongStrateis)
		fmt.Printf("[ WinShort ] Pair: %s %+v\n", summary.Pair, summary.WinShortStrateis)
		fmt.Printf("[ LoseLong ] Pair: %s %+v\n", summary.Pair, summary.LoseLongStrateis)
		fmt.Printf("[ LoseShort ] Pair: %s %+v\n", summary.Pair, summary.LoseShortStrateis)
	}
	fmt.Println()

	table.SetFooter([]string{
		"TOTAL",
		strconv.Itoa(wins + loses),
		strconv.Itoa(wins),
		strconv.Itoa(loses),
		fmt.Sprintf("%.1f %%", float64(wins)/float64(wins+loses)*100),
		fmt.Sprintf("%.3f", avgPayoff/float64(wins+loses)),
		fmt.Sprintf("%.3f", avgProfitFactor/float64(wins+loses)),
		fmt.Sprintf("%.1f", sqn/float64(len(n.serviceOrder.Results))),
		fmt.Sprintf("%.2f", total),
		fmt.Sprintf("%.2f", volume),
	})
	table.Render()

	fmt.Println(buffer.String())
	fmt.Println("------ RETURN -------")
	totalReturn := 0.0
	returnsPercent := make([]float64, len(returns))
	for i, p := range returns {
		returnsPercent[i] = p * 100
		totalReturn += p
	}
	hist := histogram.Hist(15, returnsPercent)
	histogram.Fprint(os.Stdout, hist, histogram.Linear(10))
	fmt.Println()

	fmt.Println("------ CONFIDENCE INTERVAL (95%) -------")
	for pair, summary := range n.serviceOrder.Results {
		fmt.Printf("| %s |\n", pair)
		returns := append(summary.WinPercent(), summary.LosePercent()...)
		returnsInterval := metrics.Bootstrap(returns, metrics.Mean, 10000, 0.95)
		payoffInterval := metrics.Bootstrap(returns, metrics.Payoff, 10000, 0.95)
		profitFactorInterval := metrics.Bootstrap(returns, metrics.ProfitFactor, 10000, 0.95)

		fmt.Printf("RETURN:      %.2f%% (%.2f%% ~ %.2f%%)\n",
			returnsInterval.Mean*100, returnsInterval.Lower*100, returnsInterval.Upper*100)
		fmt.Printf("PAYOFF:      %.2f (%.2f ~ %.2f)\n",
			payoffInterval.Mean, payoffInterval.Lower, payoffInterval.Upper)
		fmt.Printf("PROF.FACTOR: %.2f (%.2f ~ %.2f)\n",
			profitFactorInterval.Mean, profitFactorInterval.Lower, profitFactorInterval.Upper)
	}

	fmt.Println()

	if n.paperWallet != nil {
		n.paperWallet.Summary()
	}

}

func (n *Bot) SaveReturns(outputDir string) error {
	for _, summary := range n.serviceOrder.Results {
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
		n.serviceStrategy.OnCandle(timeframe, candle)
		n.serviceOrder.OnCandle(candle)
	} else {
		n.serviceStrategy.OnRealCandle(timeframe, candle)
	}
}

// Process pending candles in buffer
func (n *Bot) processCandles(pair string, timeframe string) {
	for item := range n.priorityQueueCandles[pair][timeframe].PopLock() {
		n.processCandle(timeframe, item.(model.Candle))
	}
}

func (n *Bot) backtestCandles(pair string, timeframe string) {
	utils.Log.Info("[SETUP] Starting backtesting")

	for n.priorityQueueCandles[pair][timeframe].Len() > 0 {
		item := n.priorityQueueCandles[pair][timeframe].Pop()

		candle := item.(model.Candle)
		// 监听蜡烛数据，更新exchange order
		if n.paperWallet != nil {
			n.paperWallet.OnCandle(candle)
		}
		// 监听exchange订单，更新订单控制器
		n.serviceOrder.ListenUpdateOrders()
		// 更新订单最新价格
		n.serviceOrder.OnCandle(candle)
		// 处理开仓策略相关
		n.serviceStrategy.OnCandle(timeframe, candle)
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
		utils.Log.Info(option.String())
		err := n.exchange.SetPairOption(ctx, option)
		if err != nil {
			utils.Log.Error(err)
			return
		}
		n.serviceStrategy.SetPairDataframe(option)

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
	}
}

// Run will initialize the strategy controller, order controller, preload data and start the bot
func (n *Bot) Run(ctx context.Context) {
	// 输出策略详情
	n.strategy.Stdout()
	n.orderFeed.Start()
	n.serviceOrder.Start()
	defer n.serviceOrder.Stop()

	n.SettingPairs(ctx)

	n.serviceStrategy.Start()

	// start data feed and receives new candles
	n.dataFeed.Start()

	// Start notifies
	if n.telegram != nil {
		n.telegram.Start()
	}

	for _, option := range n.settings.PairOptions {
		timeframaMap := n.strategy.TimeWarmupMap()
		for timeframe := range timeframaMap {
			if n.backtest {
				n.backtestCandles(option.Pair, timeframe)
			} else {
				go n.processCandles(option.Pair, timeframe)
			}
		}
	}
	if n.backtest {
		n.Summary()
	}
	select {}
}
