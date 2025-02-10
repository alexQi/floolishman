
# 愚公量化交易 - Floolishman  

**Floolishman** 专为克服交易中的人性弱点设计，通过算法消除惧怕亏损、害怕盈利回撤等情绪因素。系统提供 **复合策略引擎、动态风险管控** 和 **AI辅助决策** ，助力稳健量化交易。

---

## 📋 功能概览

### 核心交易功能
- ✅ **多币种监控**：支持单币种/全币种模式，灵活切换市场覆盖
- ✅ **智能止损机制**：
  - ✈️ 移动止盈：波动率驱动止盈线自动上浮
  - 🛑 空间止损：价格偏离阈值立即触发
  - ⏳ 时间止损：持仓超时自动平仓
- ✅ **仓位模式**：固定仓位 / 动态仓位（根据市场波动自动调整）
- ✅ **开仓决策引擎**：结合技术指标（RSI, EMA等）自动计算最佳点位

### 高级功能
- 📈 **回溯测试**：支持历史 K 线回测，计算胜率/盈亏比/最大回撤
- 🤖 **AI 预测服务**：TensorFlow 深度学习模型预测价格趋势（需联系获取）
- 📊 **评分机制**：多时间周期策略评分加权择优

---

## ⚡ 快速部署

### 1. 交易所配置
- 申请币安 API：确保勾选 **合约交易权限**  
  _推荐加密方式：ED25519（更高安全性）_
- [API申请指南](https://www.binance.com/zh-CN/support/faq/360002502072)

### 2. 配置文件（config/bot.yaml）
```yaml
api:
  encrypt: ED25519
  key: ""
  secret: ""
  pem: ""
```

### 3. 启动程序
```bash
# Windows
floolishman.exe

# Mac/Linux
chmod +x floolishman
./floolishman
```

---

## 📊 回溯测试系统

##### 拉取数据
```bash

go run cmd/tools/cmd.go download --pair ETHUSDT --timeframe 1m --futures --output ./testdata/eth-1m.csv --days 1
```

```bash
# 测试移动止损策略在BTCUSDT的表现
go run cmd/backtesting/main.go 
```

**输出指标清单**：
- 策略胜率（Win Rate）
- 盈亏比（Profit Factor） 
- 最大回撤（Max Drawdown）
- 夏普比率（Sharpe Ratio）
- 年化收益率（Annual Return）   

---

## 🤖 TensorFlow 预测服务
**功能特色**：
- 🧠 基于 LSTM 神经网络的趋势预测模型
- 📉 输入特征：价格数据 + RSI/MACD/布林带等技术指标
- 🔮 输出结果：未来5个时间单位的涨跌概率预测
_（完整功能需邮件申请激活）_

---

## ⚠️ 风险声明
- 本系统为**量化学习项目**，严禁用于真实交易
- 金融市场存在极高风险，使用者需自行承担所有后果
- 作者不保证策略有效性，不承担任何交易亏损责任

---

### 最新更新
🆕 集成策略回测模块  
🆕 支持 TensorFlow 模型调用  
🆕 优化配置向导文档  
🆕 新增动态仓位控制逻辑

---

## 📬 技术支持

**⚠️ 项目状态公告**  
由于其他原因，本项目已停止更新维护。如需获取支持或历史版本，请联系：  
📧 邮箱：alex.qiubo@qq.com  
📱 Telegram：@golantingquer
