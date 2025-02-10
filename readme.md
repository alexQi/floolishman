## 愚公量化交易。 (Floolishman)

为了克服人性弱点，惧怕亏损，害怕盈利回撤，Floolishman 诞生了。提供以下能力

- 复合策略
- 评分机制
- 开仓决策算饭
- 空间止损
- 时间止损
- 移动止盈

| 免责声明                                                           |
|----------------------------------------------------------------|
| 本软件仅供学习用途。不要拿你害怕失去的钱去冒险。使用本软件的风险由您自己承担。作者和所有附属机构不对您的交易结果承担任何责任 |

## 安装运行

#### API申请

币安网页版申请api，需要打开允许合约交易，加密方式选择ED25519；具体申请方式请点击[币安文档中心](https://www.binance.com/zh-CN/support/faq/%E5%A6%82%E4%BD%95%E5%9C%A8%E5%B8%81%E5%AE%89%E5%88%9B%E5%BB%BAapi%E5%AF%86%E9%92%A5-360002502072)

#### 修改配置文件

```
{
    "trading": {
        // 开仓大小 0.1表示10%的仓位
        "fullSpaceRatio": 0.1,
        // 初始止损亏损比例
        "initLossRatio": 0.5,5
        // 移动止损利润网格比例 10%
        "profitableScale": 0.1,
        // 触发移动止损利润比
        "initProfitRatioLimit": 0.25
    },
    "proxy": {
        "status": true,
        "url": "http://127.0.0.1:7890"
    },
    "storage": {
        "driver": "sqlite",
        "path": "runtime/data/floolishman.db"
    },
    "log": {
        "level": "info",
        "flag": "floolishman",
        "path": "runtime/logs",
        "suffix": "log",
        "stdout": true
    },
    "api": {
        "encrypt": "ED25519", //加密方式 ED25519 MMAC 推荐ED25519
        "key": "", // api key
        "secret": "", // api secret  HMAC方式时需要
        "pem": ""   // 证书路径  ED25519时需要
    },
    "telegram": {
        "token": "",
        "user": ""
    }
}
```

#### 程序运行

- windows用户，修改完配置文件，将证书放入certs/文件夹下，启动floolishman.exe
- macos,linux用户，修改完配置文件，将证书放入certs/文件夹下，启动floolishman
- 接入tensorflow 通过数据预测。

``
``

由于其他原因，本项目暂停开发，如有需求，可联系邮箱alex.qiubo@qq.com
