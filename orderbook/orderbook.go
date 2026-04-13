package orderbook

import (
	"sync"
	"time"
)

// TradeState 单个交易对的状态
type TradeState struct {
	mu sync.RWMutex

	Symbol      string
	Side        string    // "LONG" | "SHORT" | "NONE"
	EntryPrice  float64
	EntryTime   time.Time
	EntryBarIdx int       // 开仓时的 bar 索引（用于超时计算）
	Qty         float64

	// 止盈止损单 ID
	StopOrderID  int64
	TPOrderID    int64
	TrailOrderID int64

	// 保本标志和保本止损价
	BreakevenActivated bool
	BESLPrice          float64 // 保本止损价（avgP ± ATR×0.2）

	// 当前 ATR（开仓时记录）
	EntryATR float64

	// 当前 bar 索引（每次 tick 更新）
	CurrentBarIdx int
}

// BarsHeld 计算持仓 bar 数量
func (ts *TradeState) BarsHeld() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if ts.Side == "NONE" {
		return 0
	}
	return ts.CurrentBarIdx - ts.EntryBarIdx
}

// IsOpen 是否有持仓
func (ts *TradeState) IsOpen() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.Side != "NONE" && ts.Qty > 0
}

// Open 记录开仓
func (ts *TradeState) Open(side string, price, qty, atr float64, barIdx int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.Side = side
	ts.EntryPrice = price
	ts.EntryTime = time.Now()
	ts.EntryBarIdx = barIdx
	ts.CurrentBarIdx = barIdx
	ts.Qty = qty
	ts.EntryATR = atr
	ts.BreakevenActivated = false
	ts.BESLPrice = 0
	ts.StopOrderID = 0
	ts.TPOrderID = 0
	ts.TrailOrderID = 0
}

// Close 记录平仓
func (ts *TradeState) Close() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.Side = "NONE"
	ts.EntryPrice = 0
	ts.Qty = 0
	ts.StopOrderID = 0
	ts.TPOrderID = 0
	ts.TrailOrderID = 0
	ts.BreakevenActivated = false
	ts.BESLPrice = 0
}

// SetOrders 记录止盈止损单 ID
func (ts *TradeState) SetOrders(stopID, tpID, trailID int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.StopOrderID = stopID
	ts.TPOrderID = tpID
	ts.TrailOrderID = trailID
}

// SetStopOrder 单独更新止损单 ID
func (ts *TradeState) SetStopOrder(stopID int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.StopOrderID = stopID
}

// SetTPOrder 单独更新止盈单 ID
func (ts *TradeState) SetTPOrder(tpID int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.TPOrderID = tpID
}

// SetTrailOrder 单独更新追踪止损单 ID
func (ts *TradeState) SetTrailOrder(trailID int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.TrailOrderID = trailID
}

// GetTrailOrderID 获取追踪止损单 ID
func (ts *TradeState) GetTrailOrderID() int64 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.TrailOrderID
}

// SetBreakeven 标记保本已激活
func (ts *TradeState) SetBreakeven() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.BreakevenActivated = true
}

// SetBESLPrice 设置保本止损价
func (ts *TradeState) SetBESLPrice(price float64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.BESLPrice = price
}

// UpdateBarIdx 更新当前 bar 索引
func (ts *TradeState) UpdateBarIdx(idx int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.CurrentBarIdx = idx
}

// Snapshot 返回状态快照（用于 Web 展示）
type Snapshot struct {
	Symbol             string    `json:"symbol"`
	Side               string    `json:"side"`
	EntryPrice         float64   `json:"entry_price"`
	EntryTime          time.Time `json:"entry_time"`
	Qty                float64   `json:"qty"`
	BarsHeld           int       `json:"bars_held"`
	EntryATR           float64   `json:"entry_atr"`
	BreakevenActivated bool      `json:"breakeven_activated"`
	BESLPrice          float64   `json:"be_sl_price"`
	StopOrderID        int64     `json:"stop_order_id"`
	TPOrderID          int64     `json:"tp_order_id"`
	TrailOrderID       int64     `json:"trail_order_id"`
}

// GetSnapshot 获取状态快照
func (ts *TradeState) GetSnapshot() Snapshot {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	barsHeld := 0
	if ts.Side != "NONE" {
		barsHeld = ts.CurrentBarIdx - ts.EntryBarIdx
	}
	return Snapshot{
		Symbol:             ts.Symbol,
		Side:               ts.Side,
		EntryPrice:         ts.EntryPrice,
		EntryTime:          ts.EntryTime,
		Qty:                ts.Qty,
		BarsHeld:           barsHeld,
		EntryATR:           ts.EntryATR,
		BreakevenActivated: ts.BreakevenActivated,
		BESLPrice:          ts.BESLPrice,
		StopOrderID:        ts.StopOrderID,
		TPOrderID:          ts.TPOrderID,
		TrailOrderID:       ts.TrailOrderID,
	}
}

// NewTradeState 创建新的交易状态
func NewTradeState(symbol string) *TradeState {
	return &TradeState{
		Symbol: symbol,
		Side:   "NONE",
	}
}
