package notification

import (
	"errors"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/service"
	"floolishman/utils"
	"fmt"
	"regexp"
	"strings"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"
)

var (
	buyRegexp  = regexp.MustCompile(`/buy\s+(?P<pair>\w+)\s+(?P<amount>\d+(?:\.\d+)?)(?P<percent>%)?`)
	sellRegexp = regexp.MustCompile(`/sell\s+(?P<pair>\w+)\s+(?P<amount>\d+(?:\.\d+)?)(?P<percent>%)?`)
)

type telegram struct {
	settings     model.Settings
	orderService *service.ServiceOrder
	defaultMenu  *tb.ReplyMarkup
	client       *tb.Bot
}

type Option func(telegram *telegram)

func NewTelegram(orderService *service.ServiceOrder, settings model.Settings, options ...Option) (reference.Telegram, error) {
	menu := &tb.ReplyMarkup{ResizeReplyKeyboard: true}
	poller := &tb.LongPoller{Timeout: 10 * time.Second}

	userMiddleware := tb.NewMiddlewarePoller(poller, func(u *tb.Update) bool {
		if u.Message == nil || u.Message.Sender == nil {
			utils.Log.Error("no message, ", u)
			return false
		}

		for _, user := range settings.Telegram.Users {
			if int(u.Message.Sender.ID) == user {
				return true
			}
		}

		utils.Log.Error("invalid user, ", u.Message)
		return false
	})

	client, err := tb.NewBot(tb.Settings{
		ParseMode: tb.ModeMarkdown,
		Token:     settings.Telegram.Token,
		Poller:    userMiddleware,
	})
	if err != nil {
		return nil, err
	}

	var (
		statusBtn  = menu.Text("/status")
		profitBtn  = menu.Text("/profit")
		balanceBtn = menu.Text("/balance")
		startBtn   = menu.Text("/start")
		stopBtn    = menu.Text("/stop")
	)

	err = client.SetCommands([]tb.Command{
		{Text: "/help", Description: "Display help instructions"},
		{Text: "/stop", Description: "Stop buy and sell coins"},
		{Text: "/start", Description: "Start buy and sell coins"},
		{Text: "/status", Description: "Check bot status"},
		{Text: "/balance", Description: "Wallet balance"},
		{Text: "/profit", Description: "Summary of last trade results"},
	})
	if err != nil {
		return nil, err
	}

	menu.Reply(
		menu.Row(statusBtn, balanceBtn, profitBtn),
		menu.Row(startBtn, stopBtn),
	)

	bot := &telegram{
		orderService: orderService,
		client:       client,
		settings:     settings,
		defaultMenu:  menu,
	}

	for _, option := range options {
		option(bot)
	}

	client.Handle("/help", bot.HelpHandle)
	client.Handle("/start", bot.StartHandle)
	client.Handle("/stop", bot.StopHandle)
	client.Handle("/status", bot.StatusHandle)
	client.Handle("/balance", bot.BalanceHandle)
	client.Handle("/profit", bot.ProfitHandle)

	return bot, nil
}

func (t telegram) Start() {
	go t.client.Start()
	for _, id := range t.settings.Telegram.Users {
		_, err := t.client.Send(&tb.User{ID: int64(id)}, "Bot initialized.", t.defaultMenu)
		if err != nil {
			utils.Log.Error(err)
		}
	}
}

func (t telegram) Notify(text string) {
	for _, user := range t.settings.Telegram.Users {
		_, err := t.client.Send(&tb.User{ID: int64(user)}, text)
		if err != nil {
			utils.Log.Error(err)
		}
	}
}

func (t telegram) BalanceHandle(m *tb.Message) {
	message := "*BALANCE*\n"
	quotesValue := make(map[string]float64)
	total := 0.0

	account, err := t.orderService.Account()
	if err != nil {
		utils.Log.Error(err)
		t.OnError(err)
		return
	}

	for _, option := range t.settings.PairOptions {
		assetPair, quotePair := exchange.SplitAssetQuote(option.Pair)
		assetBalance, quoteBalance := account.Balance(assetPair, quotePair)

		assetSize := assetBalance.Free + assetBalance.Lock
		quoteSize := quoteBalance.Free + quoteBalance.Lock

		quote, err := t.orderService.LastQuote(option.Pair)
		if err != nil {
			utils.Log.Error(err)
			t.OnError(err)
			return
		}

		assetValue := assetSize * quote
		quotesValue[quotePair] = quoteSize
		total += assetValue
		message += fmt.Sprintf("%s: `%.4f` ≅ `%.2f` %s \n", assetPair, assetSize, assetValue, quotePair)
	}

	for quote, value := range quotesValue {
		total += value
		message += fmt.Sprintf("%s: `%.4f`\n", quote, value)
	}

	message += fmt.Sprintf("-----\nTotal: `%.4f`\n", total)

	_, err = t.client.Send(m.Sender, message)
	if err != nil {
		utils.Log.Error(err)
	}
}

func (t telegram) HelpHandle(m *tb.Message) {
	commands, err := t.client.GetCommands()
	if err != nil {
		utils.Log.Error(err)
		t.OnError(err)
		return
	}

	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, fmt.Sprintf("/%s - %s", command.Text, command.Description))
	}

	_, err = t.client.Send(m.Sender, strings.Join(lines, "\n"))
	if err != nil {
		utils.Log.Error(err)
	}
}

func (t telegram) ProfitHandle(m *tb.Message) {
	if len(t.orderService.Results) == 0 {
		_, err := t.client.Send(m.Sender, "No trades registered.")
		if err != nil {
			utils.Log.Error(err)
		}
		return
	}

	for pair, summary := range t.orderService.Results {
		_, err := t.client.Send(m.Sender, fmt.Sprintf("*PAIR*: `%s`\n`%s`", pair, summary.String()))
		if err != nil {
			utils.Log.Error(err)
		}
	}
}

func (t telegram) StatusHandle(m *tb.Message) {
	status := t.orderService.Status()
	_, err := t.client.Send(m.Sender, fmt.Sprintf("Status: `%s`", status))
	if err != nil {
		utils.Log.Error(err)
	}
}

func (t telegram) StartHandle(m *tb.Message) {
	if t.orderService.Status() == service.StatusRunning {
		_, err := t.client.Send(m.Sender, "Bot is already running.", t.defaultMenu)
		if err != nil {
			utils.Log.Error(err)
		}
		return
	}

	t.orderService.Start()
	_, err := t.client.Send(m.Sender, "Bot started.", t.defaultMenu)
	if err != nil {
		utils.Log.Error(err)
	}
}

func (t telegram) StopHandle(m *tb.Message) {
	if t.orderService.Status() == service.StatusStopped {
		_, err := t.client.Send(m.Sender, "Bot is already stopped.", t.defaultMenu)
		if err != nil {
			utils.Log.Error(err)
		}
		return
	}

	t.orderService.Stop()
	_, err := t.client.Send(m.Sender, "Bot stopped.", t.defaultMenu)
	if err != nil {
		utils.Log.Error(err)
	}
}

func (t telegram) OnOrder(order model.Order) {
	title := ""
	switch order.Status {
	case model.OrderStatusTypeFilled:
		title = fmt.Sprintf("✅ ORDER FILLED - %s", order.Pair)
	case model.OrderStatusTypeNew:
		title = fmt.Sprintf("🆕 NEW ORDER - %s", order.Pair)
	case model.OrderStatusTypeCanceled, model.OrderStatusTypeRejected:
		title = fmt.Sprintf("❌ ORDER CANCELED / REJECTED - %s", order.Pair)
	}
	message := fmt.Sprintf("%s\n-----\n%s", title, order)
	t.Notify(message)
}

func (t telegram) OnError(err error) {
	title := "🛑 ERROR"

	var orderError *exchange.OrderError
	if errors.As(err, &orderError) {
		message := fmt.Sprintf(`%s
		-----
		Pair: %s
		Quantity: %.4f
		-----
		%s`, title, orderError.Pair, orderError.Quantity, orderError.Err)
		t.Notify(message)
		return
	}

	t.Notify(fmt.Sprintf("%s\n-----\n%s", title, err))
}
