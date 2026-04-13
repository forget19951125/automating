# 币安自动化交易系统设计文档

## 1. 策略逻辑拆解 (TradingView Pine Script)
策略名称：BTC/ETH Final V37 - Reality Check
周期：1H
标的：BTCUSDT, ETHUSDT

### 1.1 指标计算
*   **EMA200**: 200周期指数移动平均线。
*   **RSI**: 14周期相对强弱指数。
*   **ATR**: 14周期真实波动幅度。

### 1.2 风控逻辑 (Risk Management)
*   **基础资金**: 15000 USDT（用于计算回撤）。
*   **单笔风险**: 3.0%。
*   **最大持仓时间**: 100 根K线（100小时）。
*   **动态风控锁**:
    *   `currEq` = 当前账户权益（或设定为初始资金+累计盈亏）。
    *   `highEq` = 历史最高权益（过去1000个周期）。
    *   `drawdown` = (highEq - currEq) / highEq * 100。
    *   **交易条件**: `drawdown < 14.0%` 时才允许开仓。
*   **止损距离**: `stopDist = ATR * 2.0`。
*   **仓位计算**:
    *   按用户要求，固定使用 100x 杠杆，名义仓位 10,000 USDT（保证金 100 USDT）。
    *   PineScript 中原逻辑是基于风险计算仓位，这里我们将覆盖为固定名义仓位，但止损逻辑仍使用 ATR。

### 1.3 进场逻辑 (Entry)
*   **做多 (Long)**:
    *   条件：`Close > EMA200` 且 `RSI 上穿 45` 且 `当前无仓位` 且 `回撤 < 14%`。
*   **做空 (Short)**:
    *   条件：`Close < EMA200` 且 `RSI 下穿 55` 且 `当前无仓位` 且 `回撤 < 14%`。

### 1.4 离场逻辑 (Exit)
*   **止盈止损 (Take Profit / Stop Loss)**:
    *   做多止损：`AvgPrice - ATR * 2.0`
    *   做空止损：`AvgPrice + ATR * 2.0`
    *   做多止盈（Limit）：`AvgPrice + ATR * 3.5`
    *   做空止盈（Limit）：`AvgPrice - ATR * 3.5`
    *   追踪止损：`trail_points = ATR * 10`, `trail_offset = ATR * 3`。在币安合约中，这对应于激活价格和回调幅度。
*   **时间熔断 (Time Stop)**:
    *   持仓超过 100 小时，直接市价平仓。
*   **保本移仓 (Break-even)**:
    *   做多：如果 `Close > AvgPrice + ATR * 1.5`，将止损移动到 `AvgPrice + ATR * 0.2`。
    *   做空：如果 `Close < AvgPrice - ATR * 1.5`，将止损移动到 `AvgPrice - ATR * 0.2`。

## 2. 系统架构 (Golang)

### 2.1 模块划分
1.  **API 客户端 (`exchange`)**: 封装币安 U本位合约 API，使用 `github.com/adshao/go-binance/v2`。
2.  **数据引擎 (`data`)**: 定时获取 1H K线数据，计算 EMA, RSI, ATR。
3.  **策略引擎 (`strategy`)**: 执行上述进出场和风控逻辑。
4.  **订单管理 (`order`)**: 记录开仓时间、价格，管理止盈止损单和追踪止损单。
5.  **Web 界面 (`web`)**: 使用 Gin 框架提供简单的后台，展示当前状态、持仓和日志。

### 2.2 数据流
1. 每小时初（或每分钟检查一次，但基于 1H K线收盘），获取最新 K线。
2. 计算指标。
3. 检查当前持仓状态。
4. 检查离场条件（止盈止损、超时、保本）。
5. 检查进场条件。
6. 执行下单，并设置止盈止损单。

## 3. 注意事项
*   **100x 杠杆**：在币安设置杠杆时，需要先调用调整杠杆 API。
*   **仓位模式**：默认单向持仓模式。
*   **精度问题**：下单数量和价格必须符合币安的 `stepSize` 和 `tickSize`。
