package strategy

// 策略逻辑完全对应 Pine Script V37 - Reality Check
//
// Pine Script 原始逻辑：
//   currEq = strategy.equity
//   highEq = ta.highest(currEq, 1000)
//   drawdown = (highEq - currEq) / highEq * 100
//   canTrade = drawdown < 14.0
//
//   stopDist = atr * 2.0
//   tradeQty = canTrade ? (currEq * (baseRisk / 100)) / stopDist : 0
//
// 本系统对应：
//   currEq  = 账户保证金余额（实时从币安读取）
//   highEq  = 历史最高余额（内存中维护，最多1000个点）
//   tradeQty = (currEq * 3% / 100) / (ATR * 2.0)  // 不乘 NominalMultiplier
//
// 实盘与 Pine Script 的映射关系：
//   - 信号 K 线 = n-2（刚收盘的 K 线）
//   - 开仓时使用信号 K 线的 ATR 设置 SL/TP/Trail（对应 Pine 的 strategy.exit 在信号 K 线收盘时设置）
//   - 追踪止损/超时等平仓由 watchPositions 每 30 秒调用回测 API 驱动（不使用币安 TRAILING_STOP_MARKET）
//   - 回测 API 返回完整交易列表，匹配当前持仓的入场时间+方向，若回测显示已平仓则执行市价平仓
//   - 保本检查用信号 K 线的 close 和 ATR（对应 Pine 的 strategy.exit("BE") 在 K 线收盘时检查）

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"binance_bot/config"
	"binance_bot/exchange"
	"binance_bot/indicator"
	"binance_bot/orderbook"
	"binance_bot/riskmanager"
	"binance_bot/web"

	"github.com/adshao/go-binance/v2/futures"
)

const symbolsFile = "symbols.json"

// saveSymbols 将当前币种列表写入 symbols.json
func (e *Engine) saveSymbols() {
	syms := make([]string, 0, len(e.symbols))
	for sym := range e.symbols {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	data, err := json.MarshalIndent(syms, "", "  ")
	if err != nil {
		web.Warn("SYSTEM", fmt.Sprintf("币种列表序列化失败: %v", err))
		return
	}
	if err := os.WriteFile(symbolsFile, data, 0644); err != nil {
		web.Warn("SYSTEM", fmt.Sprintf("币种列表写入失败: %v", err))
		return
	}
	web.Info("SYSTEM", fmt.Sprintf("币种列表已保存: %v", syms))
}

// loadSymbols 从 symbols.json 读取币种列表，如果文件不存在则返回 nil
func loadSymbols() []string {
	data, err := os.ReadFile(symbolsFile)
	if err != nil {
		return nil // 文件不存在，正常
	}
	var syms []string
	if err := json.Unmarshal(data, &syms); err != nil {
		return nil
	}
	return syms
}

// Engine 策略引擎
type Engine struct {
	cfg          *config.Config
	client       *exchange.Client
	riskMgr      *riskmanager.RiskManager
	states       map[string]*orderbook.TradeState
	exchangeInfo map[string]*exchange.ExchangeInfo
	symbols      map[string]bool // 当前激活的交易对（支持运行时增删）
	mu           sync.RWMutex
	running      atomic.Bool
	barIdx       int
}

// NewEngine 创建策略引擎
func NewEngine(cfg *config.Config, client *exchange.Client, riskMgr *riskmanager.RiskManager) *Engine {
	states := make(map[string]*orderbook.TradeState)
	symbols := make(map[string]bool)

	// 优先从 symbols.json 读取币种列表，如果不存在则使用 cfg.Symbols
	initSyms := loadSymbols()
	if len(initSyms) == 0 {
		initSyms = cfg.Symbols
		web.Info("SYSTEM", fmt.Sprintf("从配置文件加载币种: %v", initSyms))
	} else {
		web.Info("SYSTEM", fmt.Sprintf("从 symbols.json 加载币种: %v", initSyms))
	}

	for _, sym := range initSyms {
		states[sym] = orderbook.NewTradeState(sym)
		symbols[sym] = true
	}
	return &Engine{
		cfg:          cfg,
		client:       client,
		riskMgr:      riskMgr,
		states:       states,
		exchangeInfo: make(map[string]*exchange.ExchangeInfo),
		symbols:      symbols,
	}
}

// AddSymbol 运行时添加交易对
func (e *Engine) AddSymbol(symbol string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.symbols[symbol] {
		return fmt.Errorf("交易对 %s 已存在", symbol)
	}

	// 设置杠杆和保证金模式
	if err := e.client.SetMarginType(symbol); err != nil {
		web.Warn(symbol, fmt.Sprintf("设置保证金模式: %v", err))
	}
	if err := e.client.SetLeverage(symbol, e.cfg.Leverage); err != nil {
		return fmt.Errorf("设置杠杆失败: %w", err)
	}

	// 获取精度信息
	info, err := e.client.GetExchangeInfo(symbol)
	if err != nil {
		return fmt.Errorf("获取精度信息失败: %w", err)
	}

	e.states[symbol] = orderbook.NewTradeState(symbol)
	e.exchangeInfo[symbol] = info
	e.symbols[symbol] = true
	web.Info(symbol, fmt.Sprintf("✅ 已添加交易对 %s（杠杆=%dx 精度stepSize=%.8f）", symbol, e.cfg.Leverage, info.StepSize))
	e.saveSymbols() // 持久化
	return nil
}

// RemoveSymbol 运行时删除交易对（有持仓时拒绝删除）
func (e *Engine) RemoveSymbol(symbol string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.symbols[symbol] {
		return fmt.Errorf("交易对 %s 不存在", symbol)
	}

	// 检查是否有持仓
	if state, ok := e.states[symbol]; ok && state.IsOpen() {
		return fmt.Errorf("交易对 %s 当前有持仓，请先平仓再删除", symbol)
	}

	delete(e.symbols, symbol)
	delete(e.states, symbol)
	delete(e.exchangeInfo, symbol)
	web.Info(symbol, fmt.Sprintf("✅ 已删除交易对 %s", symbol))
	e.saveSymbols() // 持久化
	return nil
}

// GetSymbols 获取当前激活的交易对列表
func (e *Engine) GetSymbols() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	syms := make([]string, 0, len(e.symbols))
	for sym := range e.symbols {
		syms = append(syms, sym)
	}
	return syms
}

// Start 启动策略引擎
func (e *Engine) Start() {
	if e.running.Swap(true) {
		web.Warn("SYSTEM", "策略引擎已在运行中")
		return
	}
	web.Info("SYSTEM", "策略引擎启动")
	go e.run()
	go e.watchPositions()
}

// watchPositions 秒级实时监控持仓（每 5 秒检查一次）
// 对应 TradingView 的 K 线内实时价格检查：
//  1. 检测持仓是否已被平仓（止损/止盈单触发），立即取消残留挂单
//  2. 检测保本条件（实时价格 > avgP + ATR×1.5），立即移动止损
//
// 保本触发时：只取消旧止损单，挂新的保本止损单
// 保留止盈单和追踪止损单（对应 Pine Script: strategy.exit("BE") 只覆盖 stop）
func (e *Engine) watchPositions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for e.running.Load() {
		<-ticker.C
		if !e.running.Load() {
			break
		}

		e.mu.RLock()
		syms := make([]string, 0, len(e.symbols))
		for sym := range e.symbols {
			syms = append(syms, sym)
		}
		e.mu.RUnlock()

		for _, sym := range syms {
			state := e.states[sym]
			if state == nil || !state.IsOpen() {
				continue
			}

			// 获取实时标记价格
			price, err := e.client.GetMarkPrice(sym)
			if err != nil {
				continue
			}

			// 1. 检测持仓是否已被平仓
			pos, err := e.client.GetPosition(sym)
			if err != nil {
				continue
			}
			if pos.Side == "NONE" && state.IsOpen() {
				web.Trade(sym, "实时监控：检测到持仓已平仓，取消残留挂单")
				state.Close()
				_ = e.client.CancelAllOrders(sym)
				_ = e.client.CancelAllAlgoOrders(sym)
				continue
			}

			snap := state.GetSnapshot()
			atr := snap.EntryATR
			if atr == 0 {
				continue
			}
			avgP := snap.EntryPrice

			// 2. 检测保本条件（实时价格 K 线内触发）
			if !snap.BreakevenActivated {
				var beTriggered bool
				var newStopPrice float64
				if snap.Side == "LONG" && price > avgP+atr*1.5 {
					newStopPrice = avgP + atr*0.2
					beTriggered = true
				} else if snap.Side == "SHORT" && price < avgP-atr*1.5 {
					newStopPrice = avgP - atr*0.2
					beTriggered = true
				}

				if beTriggered {
					web.Trade(sym, fmt.Sprintf(
						"🔒 实时保本触发！实时价=%.4f | 条件: avgP±ATR×1.5=%.4f | 新止损=%.4f",
						price, avgP+atr*1.5, newStopPrice,
					))

					e.mu.RLock()
					info := e.exchangeInfo[sym]
					e.mu.RUnlock()
					if info != nil {
						newStopPrice = exchange.RoundToTick(newStopPrice, info.TickSize)
					}

					oldStopID := snap.StopOrderID
					if oldStopID != 0 {
						_ = e.client.CancelAlgoOrder(oldStopID)
					}

					var stopSide futures.SideType
					if snap.Side == "LONG" {
						stopSide = futures.SideTypeSell
					} else {
						stopSide = futures.SideTypeBuy
					}

					stopID, err := e.client.PlaceStopOrder(sym, stopSide, pos.Qty, newStopPrice)
					if err != nil {
						web.Error(sym, fmt.Sprintf("实时保本止损单失败: %v", err))
					} else {
						state.SetStopOrder(stopID)
						state.SetBreakeven()
						state.SetBESLPrice(newStopPrice)
						web.Trade(sym, fmt.Sprintf("✅ 实时保本止损单 ID=%d 价格=%.4f", stopID, newStopPrice))
					}
					continue // 保本触发后跳过追踪止损检查
				}
			}

			// 3. 回测驱动平仓：调用回测 API，匹配当前持仓的交易，
			//    如果回测显示该交易已平仓，则按回测的平仓原因执行市价平仓
			if exitReason := e.checkBacktestExit(sym, snap); exitReason != "" {
				web.Trade(sym, fmt.Sprintf("📊 回测驱动平仓：%s", exitReason))
				_ = e.client.CancelAllOrders(sym)
				_ = e.client.CancelAllAlgoOrders(sym)
				if err := e.client.ClosePosition(sym, pos); err != nil {
					web.Error(sym, fmt.Sprintf("回测驱动平仓失败: %v", err))
				} else {
					state.Close()
				}
			}
		}
	}
}

// Stop 停止策略引擎
func (e *Engine) Stop() {
	e.running.Store(false)
	web.Info("SYSTEM", "策略引擎已停止")
}

// IsRunning 是否运行中
func (e *Engine) IsRunning() bool {
	return e.running.Load()
}

// run 主循环：等待每小时整点后 1 分钟执行（确保1H K线已收盘）
func (e *Engine) run() {
	e.initialize()
	e.tick()

	for e.running.Load() {
		now := time.Now()
		nextHour := now.Truncate(time.Hour).Add(time.Hour).Add(time.Minute)
		sleepDur := time.Until(nextHour)
		if sleepDur < 0 {
			sleepDur = time.Minute
		}
		web.Info("SYSTEM", fmt.Sprintf("下次执行时间: %s（%.1f 分钟后）",
			nextHour.Format("15:04:05"), sleepDur.Minutes()))

		timer := time.NewTimer(sleepDur)
		<-timer.C
		timer.Stop()
		if e.running.Load() {
			e.tick()
		}
	}
}

// initialize 初始化：设置杠杆和保证金模式，并同步已有持仓和挂单
func (e *Engine) initialize() {
	e.mu.RLock()
	syms := make([]string, 0, len(e.symbols))
	for sym := range e.symbols {
		syms = append(syms, sym)
	}
	e.mu.RUnlock()
	for _, sym := range syms {
		if err := e.client.SetMarginType(sym); err != nil {
			web.Warn(sym, fmt.Sprintf("设置保证金模式: %v（可能已是全仓）", err))
		}
		if err := e.client.SetLeverage(sym, e.cfg.Leverage); err != nil {
			web.Error(sym, fmt.Sprintf("设置杠杆失败: %v", err))
		} else {
			web.Info(sym, fmt.Sprintf("杠杆已设置为 %dx", e.cfg.Leverage))
		}
		info, err := e.client.GetExchangeInfo(sym)
		if err != nil {
			web.Error(sym, fmt.Sprintf("获取精度信息失败: %v", err))
		} else {
			e.mu.Lock()
			e.exchangeInfo[sym] = info
			e.mu.Unlock()
			web.Info(sym, fmt.Sprintf("精度: stepSize=%.8f tickSize=%.8f", info.StepSize, info.TickSize))
		}
		// 启动时同步已有持仓和挂单
		e.syncOnStartup(sym)
	}
}

// syncOnStartup 启动时同步持仓和挂单状态，防止重启后丢失仓位管理
func (e *Engine) syncOnStartup(symbol string) {
	state := e.states[symbol]
	if state == nil {
		return
	}

	// 1. 从币安读取当前持仓
	pos, err := e.client.GetPosition(symbol)
	if err != nil {
		web.Error(symbol, fmt.Sprintf("启动同步：获取持仓失败: %v", err))
		return
	}

	if pos.Side == "NONE" || pos.Qty == 0 {
		web.Info(symbol, "启动同步：无持仓")
		return
	}

	// 2. 恢复持仓状态（ATR=0，后续 tick 会用实时 ATR 覆盖）
	state.Open(pos.Side, pos.EntryPrice, pos.Qty, 0, e.barIdx-1)
	web.Warn(symbol, fmt.Sprintf(
		"🔄 启动同步：恢复持仓 %s %.4f @ %.4f",
		pos.Side, pos.Qty, pos.EntryPrice,
	))

	// 3. 从币安读取当前挂单，恢复止损/止盈/追踪止损单 ID
	orders, err := e.client.GetOpenOrders(symbol)
	if err != nil {
		web.Warn(symbol, fmt.Sprintf("启动同步：获取挂单失败（不影响持仓恢复）: %v", err))
		return
	}

	var stopID, tpID, trailID int64
	for _, o := range orders {
		switch string(o.Type) {
		case "STOP_MARKET":
			stopID = o.OrderID
			web.Info(symbol, fmt.Sprintf("启动同步：发现止损单 ID=%d 触发价=%.4f", o.OrderID, mustParseFloat(o.StopPrice)))
		case "TAKE_PROFIT_MARKET":
			tpID = o.OrderID
			web.Info(symbol, fmt.Sprintf("启动同步：发现止盈单 ID=%d 触发价=%.4f", o.OrderID, mustParseFloat(o.StopPrice)))
		case "TRAILING_STOP_MARKET":
			trailID = o.OrderID
			web.Info(symbol, fmt.Sprintf("启动同步：发现追踪止损单 ID=%d", o.OrderID))
		}
	}

	state.SetOrders(stopID, tpID, trailID)
	web.Info(symbol, fmt.Sprintf(
		"启动同步完成：止损ID=%d 止盈ID=%d 追踪止损ID=%d",
		stopID, tpID, trailID,
	))

	// 4. 如果有持仓但没有挂单，自动补挂止损/止盈/追踪止损单
	if stopID == 0 && tpID == 0 && trailID == 0 {
		web.Warn(symbol, "检测到裸仓（有持仓但无挂单），自动补挂止损止盈单...")
		e.repairExitOrders(symbol, state, pos)
	}
}

// mustParseFloat 安全解析浮点数（用于日志）
func mustParseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// repairExitOrders 裂仓修复：为已有持仓补挂止损/止盈/追踪止损单
// 使用当前实时 K 线的 ATR 计算止损/止盈价格
func (e *Engine) repairExitOrders(symbol string, state *orderbook.TradeState, pos *exchange.Position) {
	// 获取最新 K 线计算 ATR
	klines, err := e.client.GetKlines(symbol, "1h", 20)
	if err != nil || len(klines) < 16 {
		web.Error(symbol, fmt.Sprintf("补挂失败：获取K线失败: %v", err))
		return
	}

	n := len(klines)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i, k := range klines {
		highs[i] = k.High
		lows[i] = k.Low
		closes[i] = k.Close
	}
	atr14 := indicator.ATR(highs, lows, closes, 14)
	atr := atr14[n-2] // 使用信号K线的 ATR
	if math.IsNaN(atr) || atr == 0 {
		web.Error(symbol, "补挂失败：ATR 为 NaN 或 0")
		return
	}

	// 更新状态中的 ATR（之前同步时 ATR=0）
	snap := state.GetSnapshot()
	state.Open(snap.Side, snap.EntryPrice, snap.Qty, atr, state.BarsHeld())

	e.mu.RLock()
	info := e.exchangeInfo[symbol]
	e.mu.RUnlock()

	web.Warn(symbol, fmt.Sprintf(
		"补挂单：开仓价=%.4f 数量=%.4f ATR=%.4f",
		pos.EntryPrice, pos.Qty, atr,
	))

	e.placeExitOrders(symbol, state, pos.Side, pos.EntryPrice, pos.Qty, atr, info)
}

// tick 每根1H K线收盘后执行一次
func (e *Engine) tick() {
	e.barIdx++
	web.Info("SYSTEM", fmt.Sprintf("=== Tick #%d @ %s ===", e.barIdx, time.Now().Format("2006-01-02 15:04:05")))

	// 读取账户余额（对应 Pine Script: currEq = strategy.equity）
	account, err := e.client.GetAccountInfo()
	if err != nil {
		web.Error("SYSTEM", fmt.Sprintf("获取账户信息失败: %v", err))
		e.riskMgr.UpdateEquity(e.cfg.InitCapital)
	} else {
		currEq := account.TotalBalance
		if currEq <= 0 {
			currEq = e.cfg.InitCapital
		}
		e.riskMgr.UpdateEquity(currEq)
		web.Info("SYSTEM", fmt.Sprintf(
			"账户余额(currEq)=%.2f USDT | 可用=%.2f USDT | 未实现盈亏=%.2f USDT | 名义仓位上限=%.2f USDT",
			currEq, account.AvailableBalance, account.UnrealizedPnL,
			currEq*e.cfg.NominalMultiplier,
		))
	}

	e.mu.RLock()
	syms := make([]string, 0, len(e.symbols))
	for sym := range e.symbols {
		syms = append(syms, sym)
	}
	e.mu.RUnlock()
	for _, sym := range syms {
		e.processSymbol(sym)
	}
}

// processSymbol 处理单个交易对的完整策略逻辑
func (e *Engine) processSymbol(symbol string) {
	web.Info(symbol, fmt.Sprintf("--- 处理 %s ---", symbol))

	// 获取300根1H K线（保证 EMA200 有效需要至少 210 根）
	klines, err := e.client.GetKlines(symbol, "1h", 300)
	if err != nil {
		web.Error(symbol, fmt.Sprintf("获取K线失败: %v", err))
		return
	}
	if len(klines) < 210 {
		web.Warn(symbol, fmt.Sprintf("K线数量不足: %d", len(klines)))
		return
	}

	// 提取 OHLC 数组
	n := len(klines)
	opens := make([]float64, n)
	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	for i, k := range klines {
		opens[i] = k.Open
		closes[i] = k.Close
		highs[i] = k.High
		lows[i] = k.Low
	}

	// K线索引说明：
	//   n-1 = 当前未收盘K线（不参与计算）
	//   n-2 = 信号K线（已收盘）：用于计算指标、检测信号
	//
	// Pine Script 执行模型：
	//   K 线 i 收盘时执行脚本 → strategy.exit 用 K 线 i 的 ATR 设置参数
	//   K 线 i+1 内 broker emulator 执行这些参数
	//
	// 实盘映射：
	//   信号 K 线 = n-2（刚收盘）
	//   currentATR = atr14[n-2]（信号 K 线的 ATR）
	//   → 对应 Pine Script 中 K 线 i 收盘时的 ATR
	//   → 用于 strategy.exit 参数设置（SL/TP/Trail/BE）
	signalIdx := n - 2
	idx := signalIdx

	ema200 := indicator.EMA(closes, 200)
	rsi14 := indicator.RSI(closes, 14)
	atr14 := indicator.ATR(highs, lows, closes, 14)

	currentClose := closes[signalIdx]
	currentEMA200 := ema200[signalIdx]
	currentRSI := rsi14[signalIdx]
	currentATR := atr14[signalIdx]

	if math.IsNaN(currentEMA200) || math.IsNaN(currentRSI) || math.IsNaN(currentATR) || currentATR == 0 {
		web.Warn(symbol, "指标含 NaN 或 ATR=0，跳过")
		return
	}

	web.Info(symbol, fmt.Sprintf(
		"Close=%.4f | EMA200=%.4f | RSI=%.2f | ATR(信号)=%.4f | stopDist(ATR×2)=%.4f",
		currentClose, currentEMA200, currentRSI, currentATR, currentATR*2.0,
	))

	// 更新 bar 索引
	state := e.states[symbol]
	state.UpdateBarIdx(e.barIdx)

	// 同步实际持仓状态
	pos, err := e.client.GetPosition(symbol)
	if err != nil {
		web.Error(symbol, fmt.Sprintf("获取持仓失败: %v", err))
		return
	}
	e.syncPosition(symbol, pos, state)

	// ===== 离场逻辑（优先于进场）=====
	if state.IsOpen() {
		// 传入 currentATR（信号 K 线的 ATR）和当前 K 线（n-1）的 OHLC
		// 对应 Pine Script: strategy.exit 在信号 K 线收盘时用该 K 线的 ATR 设置参数
		// 当前 K 线 = n-1（刚收盘的 K 线），用于追踪止损的 OHLC 检查
		// 注意：实盘在整点+1分执行，n-1 已收盘，等价于回测中的 bar i
		execBarIdx := n - 1
		barOHLC := [4]float64{opens[execBarIdx], highs[execBarIdx], lows[execBarIdx], closes[execBarIdx]}
		e.handleExit(symbol, state, pos, currentClose, currentATR, barOHLC)
		return
	}

	// ===== 进场逻辑 =====
	e.handleEntry(symbol, state, currentClose, currentEMA200, currentRSI, currentATR, rsi14, idx)
}

// syncPosition 同步本地状态与交易所实际持仓
func (e *Engine) syncPosition(symbol string, pos *exchange.Position, state *orderbook.TradeState) {
	if pos.Side == "NONE" && state.IsOpen() {
		web.Trade(symbol, "检测到仓位已被止盈/止损平仓，重置本地状态")
		state.Close()
		// 自动取消所有残留挂单（普通单 + Algo 单）
		if err := e.client.CancelAllOrders(symbol); err != nil {
			web.Warn(symbol, fmt.Sprintf("取消残留挂单失败（可能已无挂单）: %v", err))
		} else {
			web.Info(symbol, "已自动取消所有残留挂单")
		}
		_ = e.client.CancelAllAlgoOrders(symbol)
	} else if pos.Side != "NONE" && !state.IsOpen() {
		web.Warn(symbol, fmt.Sprintf("发现未记录持仓: %s %.4f @ %.4f，同步状态",
			pos.Side, pos.Qty, pos.EntryPrice))
		state.Open(pos.Side, pos.EntryPrice, pos.Qty, 0, e.barIdx-1)
	}
}

// handleExit 处理离场逻辑（完全对应 Pine Script 离场部分，对齐回测 v21）
//
// currentATR: 信号 K 线的 ATR（= 回测的 prevATR）
// barOHLC: 当前执行 K 线的 [Open, High, Low, Close]（= 回测中 bar i 的 OHLC）
//
// 实盘离场策略（对齐回测 backtest_v21.go）：
//   - 每根 K 线用 currentATR 重新计算 SL/TP/Trail 参数
//   - 追踪止损每根 K 线重新计算（fresh trail，对应 Pine Script strategy.exit 每根 K 线替换订单）
//   - 用 OHLC 4 点模型检查追踪止损激活和触发
//   - 如果追踪止损触发，主动市价平仓
//   - 追踪止损由 watchPositions 每 30 秒调用 checkTrailOnBar 驱动（不使用币安 TRAILING_STOP_MARKET）
func (e *Engine) handleExit(symbol string, state *orderbook.TradeState, pos *exchange.Position, currentClose, currentATR float64, barOHLC [4]float64) {
	snap := state.GetSnapshot()
	avgP := snap.EntryPrice
	atr := currentATR // 保本逻辑使用信号 K 线的 ATR
	if atr == 0 {
		atr = snap.EntryATR
	}

	e.mu.RLock()
	info := e.exchangeInfo[symbol]
	e.mu.RUnlock()

	barsHeld := state.BarsHeld()
	web.Info(symbol, fmt.Sprintf(
		"持仓中: %s %.4f @ %.4f | 持仓 %d/%d 根K线 | 保本: %v | ATR(信号)=%.4f | ATR(开仓)=%.4f",
		snap.Side, snap.Qty, avgP, barsHeld, e.cfg.MaxHoldBars, snap.BreakevenActivated,
		currentATR, snap.EntryATR,
	))

	// 1. 超时强平（对应回测逻辑: barsHeld >= maxHoldBars → 超时强平）
	if barsHeld >= e.cfg.MaxHoldBars {
		web.Trade(symbol, fmt.Sprintf("⏰ 超时强制平仓！持仓 %d 根 >= %d 根", barsHeld, e.cfg.MaxHoldBars))
		_ = e.client.CancelAllOrders(symbol)
		_ = e.client.CancelAllAlgoOrders(symbol)
		if err := e.client.ClosePosition(symbol, pos); err != nil {
			web.Error(symbol, fmt.Sprintf("平仓失败: %v", err))
		} else {
			state.Close()
		}
		return
	}

	// 2. 保本移仓（对应 Pine Script 保本逻辑）
	//
	// Pine Script:
	//   if (position_size > 0 and close > avgP + atr * 1.5)
	//       strategy.exit("BE", "L", stop=avgP + atr * 0.2)
	//
	// 实盘映射：
	//   currentClose = 信号 K 线收盘价 = Pine Script 中当前 K 线的 close
	//   atr = 信号 K 线的 ATR = Pine Script 中当前 K 线的 atr
	//
	// strategy.exit("BE") 只覆盖 stop，不影响 limit 和 trail
	// 所以保本触发后：只取消旧止损单，挂新的保本止损单，保留止盈单和追踪止损单
	if !snap.BreakevenActivated {
		var beTriggered bool
		var newStopPrice float64

		if snap.Side == "LONG" && currentClose > avgP+atr*1.5 {
			newStopPrice = avgP + atr*0.2
			beTriggered = true
		} else if snap.Side == "SHORT" && currentClose < avgP-atr*1.5 {
			newStopPrice = avgP - atr*0.2
			beTriggered = true
		}

			if beTriggered {
				web.Trade(symbol, fmt.Sprintf(
					"🔒 保本触发！close=%.4f | 条件: avgP±ATR×1.5=%.4f | 新止损=%.4f（avgP±ATR×0.2）",
					currentClose, avgP+atr*1.5, newStopPrice,
				))

				if info != nil {
					newStopPrice = exchange.RoundToTick(newStopPrice, info.TickSize)
				}

				// 只取消旧止损单（保留止盈单和追踪止损单）
				// 对应 Pine Script: strategy.exit("BE") 只覆盖 stop
				oldStopID := snap.StopOrderID
				if oldStopID != 0 {
					if err := e.client.CancelAlgoOrder(oldStopID); err != nil {
						web.Warn(symbol, fmt.Sprintf("取消旧止损单失败: %v", err))
					}
				}

				var stopSide futures.SideType
				if snap.Side == "LONG" {
					stopSide = futures.SideTypeSell
				} else {
					stopSide = futures.SideTypeBuy
				}

				stopID, err := e.client.PlaceStopOrder(symbol, stopSide, pos.Qty, newStopPrice)
				if err != nil {
					web.Error(symbol, fmt.Sprintf("保本止损单失败: %v", err))
				} else {
					state.SetStopOrder(stopID)
					state.SetBreakeven()
					state.SetBESLPrice(newStopPrice)
					web.Trade(symbol, fmt.Sprintf("✅ 保本止损单 ID=%d 止损价=%.4f", stopID, newStopPrice))
				}
				return
			}
	}

	// 3. 追踪止损 K 线级检查（对齐回测 v21: fresh trail per bar）
	//
	// 回测 v21 中追踪止损每根 K 线重新计算（strategy.exit 每根 K 线替换订单）
	// 用 OHLC 4 点模型检查激活和触发
	if trailExit := e.checkTrailOnBar(symbol, snap, avgP, atr, barOHLC, info); trailExit {
		web.Trade(symbol, "📉 追踪止损触发（K线级检查），主动市价平仓")
		_ = e.client.CancelAllOrders(symbol)
		_ = e.client.CancelAllAlgoOrders(symbol)
		if err := e.client.ClosePosition(symbol, pos); err != nil {
			web.Error(symbol, fmt.Sprintf("追踪止损平仓失败: %v", err))
		} else {
			state.Close()
		}
		return
	}

	// 4. 每根 K 线用最新 ATR 更新 SL/TP/Trail 单（对齐回测: prevATR 每根 K 线重算）
	e.updateExitOrders(symbol, state, pos, atr)
}

// updateExitOrders 每根 K 线用最新 ATR 更新 SL/TP 单
//
// 对齐回测 backtest_v20.go 第 371-395 行：
//   prevATR := atr14[i-1]
//   stopDist := prevATR * 2.0
//   tpDist := prevATR * 2.0 * 3.5
//   stopLoss = entryPrice ± stopDist
//   takeProfit = entryPrice ± tpDist
//
// 回测中每根 K 线都用最新的 prevATR 重新计算 SL/TP，实盘中通过取消旧单重挂新单来模拟
func (e *Engine) updateExitOrders(symbol string, state *orderbook.TradeState, pos *exchange.Position, currentATR float64) {
	snap := state.GetSnapshot()
	if !state.IsOpen() {
		return
	}

	atr := currentATR
	if atr == 0 {
		atr = snap.EntryATR
	}
	if atr == 0 {
		return
	}

	e.mu.RLock()
	info := e.exchangeInfo[symbol]
	e.mu.RUnlock()

	avgP := snap.EntryPrice
	stopDist := atr * 2.0

	var exitSide futures.SideType
	if snap.Side == "LONG" {
		exitSide = futures.SideTypeSell
	} else {
		exitSide = futures.SideTypeBuy
	}

	// 计算新的 SL/TP 价格
	var newStopPrice, newTPPrice float64
	if snap.BreakevenActivated {
		// 保本已触发，止损用保本价（不更新），只更新止盈
		if snap.Side == "LONG" {
			newTPPrice = avgP + stopDist*3.5
		} else {
			newTPPrice = avgP - stopDist*3.5
		}
	} else {
		// 未触发保本，SL 和 TP 都更新
		if snap.Side == "LONG" {
			newStopPrice = avgP - stopDist
			newTPPrice = avgP + stopDist*3.5
		} else {
			newStopPrice = avgP + stopDist
			newTPPrice = avgP - stopDist*3.5
		}
	}

	if info != nil {
		if !snap.BreakevenActivated {
			newStopPrice = exchange.RoundToTick(newStopPrice, info.TickSize)
		}
		newTPPrice = exchange.RoundToTick(newTPPrice, info.TickSize)
	}

	// 获取当前挂单，检查是否需要更新
	orders, err := e.client.GetOpenOrders(symbol)
	if err != nil {
		web.Warn(symbol, fmt.Sprintf("获取挂单失败: %v", err))
		return
	}

	var currentStopID int64
	var currentTPID int64
	var currentStopPrice, currentTPPriceVal float64
	hasTrail := false
	for _, o := range orders {
		switch string(o.Type) {
		case "STOP_MARKET":
			currentStopID = o.OrderID
			currentStopPrice, _ = strconv.ParseFloat(o.StopPrice, 64)
		case "TAKE_PROFIT_MARKET":
			currentTPID = o.OrderID
			currentTPPriceVal, _ = strconv.ParseFloat(o.StopPrice, 64)
		case "TRAILING_STOP_MARKET":
			hasTrail = true
		}
	}

	// 更新止损单（仅在未触发保本时）
	if !snap.BreakevenActivated {
		if currentStopID != 0 && math.Abs(currentStopPrice-newStopPrice) > 0.0001 {
			// 价格变化，取消旧单重挂新单
			if err := e.client.CancelAlgoOrder(currentStopID); err != nil {
				web.Warn(symbol, fmt.Sprintf("取消旧止损单失败: %v", err))
			}
			stopID, err := e.client.PlaceStopOrder(symbol, exitSide, pos.Qty, newStopPrice)
			if err != nil {
				web.Error(symbol, fmt.Sprintf("更新止损单失败: %v", err))
			} else {
				state.SetStopOrder(stopID)
				web.Info(symbol, fmt.Sprintf("🔄 更新止损单 ID=%d 价格: %.4f → %.4f", stopID, currentStopPrice, newStopPrice))
			}
		} else if currentStopID == 0 {
			// 缺失，补挂
			stopID, err := e.client.PlaceStopOrder(symbol, exitSide, pos.Qty, newStopPrice)
			if err != nil {
				web.Error(symbol, fmt.Sprintf("补挂止损单失败: %v", err))
			} else {
				state.SetStopOrder(stopID)
				web.Warn(symbol, fmt.Sprintf("🔧 补挂止损单 ID=%d 价格=%.4f", stopID, newStopPrice))
			}
		}
	} else {
		// 保本已触发，检查止损单是否存在（不更新价格）
		if currentStopID == 0 {
			var beStopPrice float64
			if snap.Side == "LONG" {
				beStopPrice = avgP + atr*0.2
			} else {
				beStopPrice = avgP - atr*0.2
			}
			if info != nil {
				beStopPrice = exchange.RoundToTick(beStopPrice, info.TickSize)
			}
			stopID, err := e.client.PlaceStopOrder(symbol, exitSide, pos.Qty, beStopPrice)
			if err != nil {
				web.Error(symbol, fmt.Sprintf("补挂保本止损单失败: %v", err))
			} else {
				state.SetStopOrder(stopID)
				web.Warn(symbol, fmt.Sprintf("🔧 补挂保本止损单 ID=%d 价格=%.4f", stopID, beStopPrice))
			}
		}
	}

	// 更新止盈单
	if currentTPID != 0 && math.Abs(currentTPPriceVal-newTPPrice) > 0.0001 {
		// 价格变化，取消旧单重挂新单
		if err := e.client.CancelAlgoOrder(currentTPID); err != nil {
			web.Warn(symbol, fmt.Sprintf("取消旧止盈单失败: %v", err))
		}
		tpID, err := e.client.PlaceTakeProfitOrder(symbol, exitSide, pos.Qty, newTPPrice)
		if err != nil {
			web.Error(symbol, fmt.Sprintf("更新止盈单失败: %v", err))
		} else {
			state.SetTPOrder(tpID)
			web.Info(symbol, fmt.Sprintf("🔄 更新止盈单 ID=%d 价格: %.4f → %.4f", tpID, currentTPPriceVal, newTPPrice))
		}
	} else if currentTPID == 0 {
		// 缺失，补挂
		tpID, err := e.client.PlaceTakeProfitOrder(symbol, exitSide, pos.Qty, newTPPrice)
		if err != nil {
			web.Error(symbol, fmt.Sprintf("补挂止盈单失败: %v", err))
		} else {
			state.SetTPOrder(tpID)
			web.Warn(symbol, fmt.Sprintf("🔧 补挂止盈单 ID=%d 价格=%.4f", tpID, newTPPrice))
		}
	}

	// 追踪止损不再使用币安 TRAILING_STOP_MARKET
	// 由 watchPositions 每 30 秒调用 checkTrailOnBar（回测 v21 逻辑）驱动
	// 清理可能残留的追踪止损单
	if hasTrail {
		for _, o := range orders {
			if string(o.Type) == "TRAILING_STOP_MARKET" {
				if err := e.client.CancelAlgoOrder(o.OrderID); err != nil {
					web.Warn(symbol, fmt.Sprintf("取消残留追踪止损单失败: %v", err))
				} else {
					web.Info(symbol, "已取消残留的币安追踪止损单")
				}
			}
		}
	}
}

// checkTrailOnBar 追踪止损 K 线级检查（完全对齐回测 v21 的 OHLC 4 点模型）
//
// 回测 v21 中追踪止损每根 K 线重新计算（fresh trail）：
//   - activationLevel = avgP ± ATR * 10 * mintick
//   - trailOff = ATR * 3 * mintick
//   - 用 OHLC 4 点模型检查激活和触发
//   - highFirst = |O-H| < |O-L|
//
// 返回 true 表示追踪止损触发，需要平仓
// backtestTrade 回测交易结果结构
type backtestTrade struct {
	No              int     `json:"no"`
	Side            string  `json:"side"`
	EntryTimeUTC8   string  `json:"entry_time_utc8"`
	TVEntryTimeUTC8 string  `json:"tv_entry_time_utc8"`
	EntryPrice      float64 `json:"entry_price"`
	ExitTimeUTC8    string  `json:"exit_time_utc8"`
	TVExitTimeUTC8  string  `json:"tv_exit_time_utc8"`
	ExitPrice       float64 `json:"exit_price"`
	ExitReason      string  `json:"exit_reason"`
	BarsHeld        int     `json:"bars_held"`
	PnlUSDT         float64 `json:"pnl_usdt"`
}

type backtestResult struct {
	Symbol string           `json:"symbol"`
	Trades []backtestTrade  `json:"trades"`
}

// checkBacktestExit 调用回测 API，匹配当前持仓的交易，
// 如果回测显示该交易已平仓，返回平仓原因；否则返回空字符串
func (e *Engine) checkBacktestExit(symbol string, snap orderbook.Snapshot) string {
	// 调用本地回测 API
	url := fmt.Sprintf("http://127.0.0.1:%s/api/backtest?symbol=%s", e.cfg.WebPort, symbol)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		web.Warn(symbol, fmt.Sprintf("回测 API 调用失败: %v", err))
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		web.Warn(symbol, fmt.Sprintf("回测 API 读取失败: %v", err))
		return ""
	}

	var result backtestResult
	if err := json.Unmarshal(body, &result); err != nil {
		web.Warn(symbol, fmt.Sprintf("回测 API 解析失败: %v", err))
		return ""
	}

	// 将实盘开仓时间转换为 UTC+8 格式（与回测 entry_time_utc8 匹配）
	// EntryBarTime 是 UTC 整点，转 UTC+8 需要 +8h
	entryBarUTC8 := snap.EntryBarTime.Add(8 * time.Hour).Format("2006-01-02 15:04")

	// 在回测结果中查找匹配的交易
	for _, trade := range result.Trades {
		// 匹配条件：入场时间一致 且 方向一致
		if trade.EntryTimeUTC8 == entryBarUTC8 && strings.EqualFold(trade.Side, snap.Side) {
			// 找到匹配的交易
			if trade.ExitTimeUTC8 != "" {
				// 回测显示该交易已平仓
				web.Info(symbol, fmt.Sprintf(
					"回测匹配 #%d: %s 入场=%s 出场=%s 价格=%.4f 原因=%s PnL=%.2f",
					trade.No, trade.Side, trade.EntryTimeUTC8, trade.ExitTimeUTC8,
					trade.ExitPrice, trade.ExitReason, trade.PnlUSDT,
				))
				return fmt.Sprintf("%s | 回测#%d 出场价=%.4f PnL=%.2f",
					trade.ExitReason, trade.No, trade.ExitPrice, trade.PnlUSDT)
			}
			// 匹配到但还未平仓，正常
			web.Info(symbol, fmt.Sprintf(
				"回测匹配 #%d: %s 入场=%s 尚未平仓（持仓 %d 根K线）",
				trade.No, trade.Side, trade.EntryTimeUTC8, trade.BarsHeld,
			))
			return ""
		}
	}

	// 未找到匹配的交易（可能开仓时间不在回测范围内）
	web.Warn(symbol, fmt.Sprintf(
		"回测未匹配: %s 入场时间=%s 方向=%s（共 %d 笔交易）",
		symbol, entryBarUTC8, snap.Side, len(result.Trades),
	))
	return ""
}

func (e *Engine) checkTrailOnBar(symbol string, snap orderbook.Snapshot, avgP, atr float64, barOHLC [4]float64, info *exchange.ExchangeInfo) bool {
	o, h, l, c := barOHLC[0], barOHLC[1], barOHLC[2], barOHLC[3]

	mintick := 0.01
	if info != nil && info.TickSize > 0 {
		mintick = info.TickSize
	}

	activateDist := atr * 10.0 * mintick
	trailOff := atr * 3.0 * mintick

	var activationLevel float64
	if snap.Side == "LONG" {
		activationLevel = avgP + activateDist
	} else {
		activationLevel = avgP - activateDist
	}

	highFirst := math.Abs(o-h) < math.Abs(o-l)

	localTrailActive := false
	localTrailStop := 0.0

	if snap.Side == "LONG" {
		if highFirst {
			// O → H → L → C
			// Phase O: trail activation
			if o >= activationLevel {
				localTrailActive = true
				localTrailStop = o - trailOff
			}
			// Phase H: activate/update trail
			if h >= activationLevel {
				localTrailActive = true
				localTrailStop = h - trailOff
			}
			// Phase L: check trail exit
			if localTrailActive && l <= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@L(hf): l=%.4f <= ts=%.4f | actLvl=%.4f trailOff=%.4f",
					l, localTrailStop, activationLevel, trailOff))
				return true
			}
			// Phase C: check trail exit
			if localTrailActive && c <= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@C(hf): c=%.4f <= ts=%.4f", c, localTrailStop))
				return true
			}
		} else {
			// O → L → H → C
			// Phase O: trail activation
			if o >= activationLevel {
				localTrailActive = true
				localTrailStop = o - trailOff
			}
			// Phase L: check trail exit (if O activated)
			if localTrailActive && l <= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@L(lf): l=%.4f <= ts=%.4f", l, localTrailStop))
				return true
			}
			// Phase H: activate trail
			if h >= activationLevel {
				localTrailActive = true
				localTrailStop = h - trailOff
			}
			// Phase C: check trail exit
			if localTrailActive && c <= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@C(lf): c=%.4f <= ts=%.4f", c, localTrailStop))
				return true
			}
		}
	} else {
		// SHORT
		if highFirst {
			// O → H → L → C
			// Phase O: trail activation
			if o <= activationLevel {
				localTrailActive = true
				localTrailStop = o + trailOff
			}
			// Phase H: check trail exit (if O activated)
			if localTrailActive && h >= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@H(hf): h=%.4f >= ts=%.4f", h, localTrailStop))
				return true
			}
			// Phase L: activate/update trail
			if l <= activationLevel {
				localTrailActive = true
				localTrailStop = l + trailOff
			}
			// Phase C: check trail exit
			if localTrailActive && c >= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@C(hf): c=%.4f >= ts=%.4f", c, localTrailStop))
				return true
			}
		} else {
			// O → L → H → C
			// Phase O: trail activation
			if o <= activationLevel {
				localTrailActive = true
				localTrailStop = o + trailOff
			}
			// Phase L: activate/update trail
			if l <= activationLevel {
				localTrailActive = true
				localTrailStop = l + trailOff
			}
			// Phase H: check trail exit
			if localTrailActive && h >= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@H(lf): h=%.4f >= ts=%.4f", h, localTrailStop))
				return true
			}
			// Phase C: check trail exit
			if localTrailActive && c >= localTrailStop {
				web.Trade(symbol, fmt.Sprintf("📉 Trail@C(lf): c=%.4f >= ts=%.4f", c, localTrailStop))
				return true
			}
		}
	}

	if localTrailActive {
		web.Info(symbol, fmt.Sprintf("🔍 Trail激活未触发: ts=%.4f actLvl=%.4f trailOff=%.4f hf=%v",
			localTrailStop, activationLevel, trailOff, highFirst))
	}

	return false
}

// checkAndRepairOrders 检查并修复缺失的挂单（已被 updateExitOrders 替代，保留作为备用）
func (e *Engine) checkAndRepairOrders(symbol string, state *orderbook.TradeState, pos *exchange.Position) {
	snap := state.GetSnapshot()
	if !state.IsOpen() {
		return
	}

	// 获取当前挂单
	orders, err := e.client.GetOpenOrders(symbol)
	if err != nil {
		web.Warn(symbol, fmt.Sprintf("检查挂单失败: %v", err))
		return
	}

	hasStop := false
	hasTP := false
	hasTrail := false
	for _, o := range orders {
		switch string(o.Type) {
		case "STOP_MARKET":
			hasStop = true
		case "TAKE_PROFIT_MARKET":
			hasTP = true
		case "TRAILING_STOP_MARKET":
			hasTrail = true
		}
	}

	atr := snap.EntryATR
	if atr == 0 {
		return // 无法计算参数
	}

	e.mu.RLock()
	info := e.exchangeInfo[symbol]
	e.mu.RUnlock()

	avgP := snap.EntryPrice
	stopDist := atr * 2.0

	var exitSide futures.SideType
	if snap.Side == "LONG" {
		exitSide = futures.SideTypeSell
	} else {
		exitSide = futures.SideTypeBuy
	}

	// 补挂止损单
	if !hasStop {
		var stopPrice float64
		if snap.BreakevenActivated {
			// 保本止损价
			if snap.Side == "LONG" {
				stopPrice = avgP + atr*0.2
			} else {
				stopPrice = avgP - atr*0.2
			}
		} else {
			if snap.Side == "LONG" {
				stopPrice = avgP - stopDist
			} else {
				stopPrice = avgP + stopDist
			}
		}
		if info != nil {
			stopPrice = exchange.RoundToTick(stopPrice, info.TickSize)
		}
		stopID, err := e.client.PlaceStopOrder(symbol, exitSide, pos.Qty, stopPrice)
		if err != nil {
			web.Error(symbol, fmt.Sprintf("补挂止损单失败: %v", err))
		} else {
			state.SetStopOrder(stopID)
			web.Warn(symbol, fmt.Sprintf("🔧 补挂止损单 ID=%d 价格=%.4f", stopID, stopPrice))
		}
	}

	// 补挂止盈单
	if !hasTP {
		var tpPrice float64
		if snap.Side == "LONG" {
			tpPrice = avgP + stopDist*3.5
		} else {
			tpPrice = avgP - stopDist*3.5
		}
		if info != nil {
			tpPrice = exchange.RoundToTick(tpPrice, info.TickSize)
		}
		tpID, err := e.client.PlaceTakeProfitOrder(symbol, exitSide, pos.Qty, tpPrice)
		if err != nil {
			web.Error(symbol, fmt.Sprintf("补挂止盈单失败: %v", err))
		} else {
			state.SetTPOrder(tpID)
			web.Warn(symbol, fmt.Sprintf("🔧 补挂止盈单 ID=%d 价格=%.4f", tpID, tpPrice))
		}
	}

	// 补挂追踪止损单（只在非保本状态下）
	if !hasTrail && !snap.BreakevenActivated {
		mintick := 0.01 // 默认
		if info != nil && info.TickSize > 0 {
			mintick = info.TickSize
		}
		activateDist := atr * 10.0 * mintick
		trailDist := atr * 3.0 * mintick

		var activationPrice float64
		if snap.Side == "LONG" {
			activationPrice = avgP + activateDist
		} else {
			activationPrice = avgP - activateDist
		}
		if info != nil {
			activationPrice = exchange.RoundToTick(activationPrice, info.TickSize)
		}
		callbackRate := trailDist / avgP * 100
		callbackRate = math.Max(0.1, math.Min(5.0, callbackRate))

		trailID, err := e.client.PlaceTrailingStopOrderWithActivation(
			symbol, exitSide, pos.Qty, activationPrice, callbackRate,
		)
		if err != nil {
			web.Warn(symbol, fmt.Sprintf("补挂追踪止损单失败: %v", err))
		} else {
			state.SetTrailOrder(trailID)
			web.Warn(symbol, fmt.Sprintf("🔧 补挂追踪止损单 ID=%d 激活价=%.4f 回调率=%.4f%%",
				trailID, activationPrice, callbackRate))
		}
	}
}

// handleEntry 处理进场逻辑（完全对应 Pine Script 进场部分）
func (e *Engine) handleEntry(symbol string, state *orderbook.TradeState,
	currentClose, currentEMA200, currentRSI, currentATR float64,
	rsi []float64, idx int) {

	// 读取当前账户余额（对应 Pine Script: currEq = strategy.equity）
	account, err := e.client.GetAccountInfo()
	if err != nil {
		web.Error(symbol, fmt.Sprintf("获取账户信息失败，跳过进场: %v", err))
		return
	}

	currEq := account.TotalBalance
	if currEq <= 0 {
		web.Warn(symbol, "账户余额为0，跳过进场")
		return
	}

	// 风控检查（对应 Pine Script: canTrade = drawdown < 14.0）
	if !e.riskMgr.CanTrade(currEq) {
		dd := e.riskMgr.Drawdown(currEq)
		web.Warn(symbol, fmt.Sprintf(
			"🔒 风控锁定！回撤 %.2f%% >= %.2f%%，禁止开仓（highEq=%.2f currEq=%.2f）",
			dd, e.cfg.MaxDrawdown, e.riskMgr.HighEquity(), currEq,
		))
		return
	}

	// 进场信号检测（对应 Pine Script 进场条件）
	rsiCrossover45 := indicator.CrossoverLevel(rsi, 45, idx)
	rsiCrossunder55 := indicator.CrossunderLevel(rsi, 55, idx)

	// 做多：Close > EMA200 且 RSI 上穿 45
	if currentClose > currentEMA200 && rsiCrossover45 {
		web.Trade(symbol, fmt.Sprintf(
			"📈 做多信号！Close(%.4f) > EMA200(%.4f) 且 RSI上穿45(%.2f→%.2f)",
			currentClose, currentEMA200, rsi[idx-1], currentRSI,
		))
		e.openPosition(symbol, state, "LONG", currentClose, currentATR, currEq)
		return
	}

	// 做空：Close < EMA200 且 RSI 下穿 55
	if currentClose < currentEMA200 && rsiCrossunder55 {
		web.Trade(symbol, fmt.Sprintf(
			"📉 做空信号！Close(%.4f) < EMA200(%.4f) 且 RSI下穿55(%.2f→%.2f)",
			currentClose, currentEMA200, rsi[idx-1], currentRSI,
		))
		e.openPosition(symbol, state, "SHORT", currentClose, currentATR, currEq)
		return
	}

	web.Info(symbol, fmt.Sprintf(
		"无信号 | Close vs EMA200: %.4f vs %.4f | RSI: %.2f | 回撤: %.2f%%",
		currentClose, currentEMA200, currentRSI, e.riskMgr.Drawdown(currEq),
	))
}

// openPosition 执行开仓，仓位计算完全对应 Pine Script 原始逻辑
func (e *Engine) openPosition(symbol string, state *orderbook.TradeState,
	side string, price, atr, currEq float64) {

	e.mu.RLock()
	info := e.exchangeInfo[symbol]
	e.mu.RUnlock()

	// ============================================================
	// 仓位计算（严格对齐回测 backtest_v20.go 第 714-717 行）
	//
	// 回测逻辑（已与 Pine Script 对齐）：
	//   stopDist = curATR * 2.0
	//   riskAmt  = equity * (baseRisk / 100)   // 不乘 NominalMultiplier
	//   qty      = riskAmt / stopDist
	//   maxQty   = (equity * nominalMultiplier) / curClose
	//
	// 实盘映射：
	//   currEq = 账户保证金余额（对应回测的 equity）
	//   riskAmount = currEq * baseRisk%（不乘 NominalMultiplier）
	//   maxQty = currEq * NominalMultiplier / price（名义仓位上限）
	// ============================================================

	stopDist := atr * 2.0
	riskAmount := currEq * (e.cfg.BaseRisk / 100)
	tradeQty := riskAmount / stopDist

	// 名义仓位上限
	maxQty := (currEq * e.cfg.NominalMultiplier) / price
	if tradeQty > maxQty {
		web.Info(symbol, fmt.Sprintf(
			"仓位截断: ATR计算量=%.4f > 名义上限量=%.4f，取上限",
			tradeQty, maxQty,
		))
		tradeQty = maxQty
	}

	// 按精度取整
	if info != nil {
		tradeQty = exchange.RoundToStep(tradeQty, info.StepSize)
		if tradeQty < info.MinQty {
			web.Warn(symbol, fmt.Sprintf(
				"计算数量 %.6f < 最小数量 %.6f，跳过开仓",
				tradeQty, info.MinQty,
			))
			return
		}
	}

	nominalValue := tradeQty * price
	marginRequired := nominalValue / float64(e.cfg.Leverage)

	web.Trade(symbol, fmt.Sprintf(
		"📊 仓位计算 | currEq=%.2f USDT | ATR=%.4f | stopDist=%.4f | 风险金额=%.2f USDT | "+
			"计算手数=%.4f | 名义价值=%.2f USDT | 所需保证金=%.2f USDT（%dx杠杆）",
		currEq, atr, stopDist, riskAmount, tradeQty, nominalValue, marginRequired, e.cfg.Leverage,
	))

	// 下市价单
	var entrySide futures.SideType
	if side == "LONG" {
		entrySide = futures.SideTypeBuy
	} else {
		entrySide = futures.SideTypeSell
	}

	orderID, err := e.client.PlaceMarketOrder(symbol, entrySide, tradeQty)
	if err != nil {
		web.Error(symbol, fmt.Sprintf("开仓失败: %v", err))
		return
	}
	web.Trade(symbol, fmt.Sprintf("✅ 开仓成功！订单ID=%d", orderID))

	// 记录开仓状态
	state.Open(side, price, tradeQty, atr, e.barIdx)

	// 等待成交，然后读取实际成交价格
	time.Sleep(2 * time.Second)
	if actualPos, err := e.client.GetPosition(symbol); err == nil && actualPos.EntryPrice > 0 {
		price = actualPos.EntryPrice
		tradeQty = actualPos.Qty
		state.Open(side, price, tradeQty, atr, e.barIdx)
		web.Info(symbol, fmt.Sprintf("实际成交: 价格=%.4f 数量=%.4f", price, tradeQty))
	}

	// 挂出止盈止损单（使用开仓时的 ATR，一次性设置）
	e.placeExitOrders(symbol, state, side, price, tradeQty, atr, info)
}

// placeExitOrders 设置止盈止损和追踪止损单（完全对应 Pine Script 离场参数）
//
// Pine Script:
//
//	strategy.exit("Ex",
//	  stop  = avgP ± stopDist        (ATR × 2.0)
//	  limit = avgP ± stopDist × 3.5  (ATR × 7.0)
//	  trail_points = atr * 10        (激活距离 = ATR×10×mintick)
//	  trail_offset = atr * 3         (回撤距离 = ATR×3×mintick)
//	)
//
// 追踪止损：
//
//	activationPrice = entryPrice ± ATR×10×mintick（基于开仓价！）
//	callbackRate = ATR×3×mintick / price × 100
//
// 只在开仓时调用一次，由交易所管理后续激活和追踪
func (e *Engine) placeExitOrders(symbol string, state *orderbook.TradeState,
	side string, price, qty, atr float64, info *exchange.ExchangeInfo) {

	stopDist := atr * 2.0

	var stopPrice, tpPrice float64
	var exitSide futures.SideType

	if side == "LONG" {
		stopPrice = price - stopDist
		tpPrice = price + stopDist*3.5
		exitSide = futures.SideTypeSell
	} else {
		stopPrice = price + stopDist
		tpPrice = price - stopDist*3.5
		exitSide = futures.SideTypeBuy
	}

	if info != nil {
		stopPrice = exchange.RoundToTick(stopPrice, info.TickSize)
		tpPrice = exchange.RoundToTick(tpPrice, info.TickSize)
	}

	web.Info(symbol, fmt.Sprintf(
		"设置离场单 | 止损=%.4f（ATR×2=%.4f）| 止盈=%.4f（ATR×7=%.4f）",
		stopPrice, atr*2.0, tpPrice, atr*7.0,
	))

	// 止损单（STOP_MARKET）
	stopID, err := e.client.PlaceStopOrder(symbol, exitSide, qty, stopPrice)
	if err != nil {
		web.Error(symbol, fmt.Sprintf("止损单失败: %v", err))
	} else {
		web.Trade(symbol, fmt.Sprintf("✅ 止损单 ID=%d 价格=%.4f", stopID, stopPrice))
	}

	// 止盈单（TAKE_PROFIT_MARKET）
	tpID, err := e.client.PlaceTakeProfitOrder(symbol, exitSide, qty, tpPrice)
	if err != nil {
		web.Error(symbol, fmt.Sprintf("止盈单失败: %v", err))
	} else {
		web.Trade(symbol, fmt.Sprintf("✅ 止盈单 ID=%d 价格=%.4f", tpID, tpPrice))
	}

	// 追踪止损不再使用币安 TRAILING_STOP_MARKET（callbackRate 最小 0.1% 导致过早触发）
	// 改为由 watchPositions 每 30 秒调用 checkTrailOnBar（回测 v21 逻辑）驱动
	var trailID int64 = 0
	web.Info(symbol, "追踪止损由 checkTrailOnBar（回测 v21 逻辑）驱动，不挂币安 TRAILING_STOP_MARKET")

	state.SetOrders(stopID, tpID, trailID)
}

// ForceCloseAll 强制平仓所有持仓（紧急用）
func (e *Engine) ForceCloseAll() {
	web.Warn("SYSTEM", "🚨 执行强制全部平仓！")
	e.mu.RLock()
	syms := make([]string, 0, len(e.symbols))
	for sym := range e.symbols {
		syms = append(syms, sym)
	}
	e.mu.RUnlock()
	for _, sym := range syms {
		pos, err := e.client.GetPosition(sym)
		if err != nil {
			web.Error(sym, fmt.Sprintf("获取持仓失败: %v", err))
			continue
		}
		if pos.Side == "NONE" {
			web.Info(sym, "无持仓，跳过")
			continue
		}
		_ = e.client.CancelAllOrders(sym)
		_ = e.client.CancelAllAlgoOrders(sym)
		if err := e.client.ClosePosition(sym, pos); err != nil {
			web.Error(sym, fmt.Sprintf("平仓失败: %v", err))
		} else {
			e.states[sym].Close()
			web.Trade(sym, "✅ 强制平仓成功")
		}
	}
}
