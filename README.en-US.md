

# Yugong Quantitative Trading - Floolishman  

**Floolishman** is specifically designed to overcome human weaknesses in trading, eliminating emotional factors such as fear of losses and fear of profit drawdowns through algorithms. The system provides a **compound strategy engine, dynamic risk management**, and **AI-assisted decision-making** to facilitate robust quantitative trading.

---

## 📋 Feature Overview

### Core Trading Features
- ✅ **Multi-Coin Monitoring**: Supports single-coin/all-coin modes, flexibly switching market coverage
- ✅ **Intelligent Stop-Loss Mechanism**:
  - ✈️ Trailing Take-Profit: Volatility-driven take-profit line automatically rises
  - 🛑 Distance-based Stop-Loss: Triggered immediately when price deviates beyond threshold
  - ⏳ Time-based Stop-Loss: Positions automatically closed upon timeout
- ✅ **Position Modes**: Fixed position / Dynamic position (automatically adjusts based on market volatility)
- ✅ **Entry Decision Engine**: Automatically calculates optimal entry points by integrating technical indicators (RSI, EMA, etc.)

### Advanced Features
- 📈 **Backtesting**: Supports historical candlestick backtesting, calculating win rate/profit ratio/max drawdown
- 🤖 **AI Prediction Service**: TensorFlow deep learning model for price trend prediction (requires contact for access)
- 📊 **Scoring Mechanism**: Weighted optimization based on strategy scores across multiple timeframes

---

## ⚡ Quick Deployment

### 1. Exchange Configuration
- Apply for Binance API: Ensure **Futures Trading permissions** are enabled  
  _Recommended encryption: ED25519 (higher security)_
- [API Application Guide](https://www.binance.com/zh-CN/support/faq/360002502072)

### 2. Configuration File (config/bot.yaml)
```yaml
api:
  encrypt: ED25519
  key: ""
  secret: ""
  pem: ""
```

### 3. Run the Program
```bash
# Windows
floolishman.exe

# Mac/Linux
chmod +x floolishman
./floolishman
```

---

## 📊 Backtesting System

##### Data Fetching
```bash

go run cmd/tools/cmd.go download --pair ETHUSDT --timeframe 1m --futures --output ./testdata/eth-1m.csv --days 1
```

```bash
# 测试移动止损策略在BTCUSDT的表现
go run cmd/backtesting/main.go 
```

**Output Metrics List**:
- Strategy Win Rate (Win Rate)
- Profit/Loss Ratio (Profit Factor) 
- Max Drawdown (Max Drawdown)
- Sharpe Ratio (Sharpe Ratio)
- Annualized Return (Annual Return)   

---

## 🤖 TensorFlow Prediction Service
**Key Features**:
- 🧠 Trend prediction model based on LSTM neural networks
- 📉 Input Features: Price data + technical indicators such as RSI/MACD/Bollinger Bands
- 🔮 Output Results: Probability predictions for price up/down over the next 5 time units
_(Full functionality requires email application for activation)_

---

## ⚠️ Risk Disclaimer
- This system is a **quantitative learning project** and is strictly prohibited for use in live trading.
- Financial markets carry extremely high risks; users must assume all consequences independently.
- The author does not guarantee the effectiveness of the strategies and assumes no responsibility for any trading losses.

---

### Latest Updates
🆕 Integrated strategy backtesting module  
🆕 Added support for TensorFlow model invocation  
🆕 Optimized configuration wizard documentation  
🆕 Added dynamic position control logic

---

## 📬 Technical Support

**⚠️ Project Status Announcement**  
Due to other commitments, this project has ceased updates and maintenance. For support or historical versions, please contact:  
📧 Email: alex.qiubo@qq.com  
📱 Telegram: @golantingquer
