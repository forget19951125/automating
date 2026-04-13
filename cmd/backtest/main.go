// cmd/backtest/main.go
// V37 策略回测 — v21 每根K线重新计算追踪止损

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var debugMode bool
var debugTradeNos map[int]bool

func emaCalc(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	if len(closes) < period {
		for i := range result {
			result[i] = math.NaN()
		}
		return result
	}
	k := 2.0 / float64(period+1)
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
		result[i] = math.NaN()
	}
	result[period-1] = sum / float64(period)
	for i := period; i < len(closes); i++ {
		result[i] = closes[i]*k + result[i-1]*(1-k)
	}
	return result
}

func rsiCalc(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	for i := range result {
		result[i] = math.NaN()
	}
	if len(closes) < period+1 {
		return result
	}
	gains := make([]float64, len(closes))
	losses := make([]float64, len(closes))
	for i := 1; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gains[i] = diff
		} else {
			losses[i] = -diff
		}
	}
	avgGain := 0.0
	avgLoss := 0.0
	for i := 1; i <= period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)
	if avgLoss == 0 {
		result[period] = 100
	} else {
		rs := avgGain / avgLoss
		result[period] = 100 - (100 / (1 + rs))
	}
	for i := period + 1; i < len(closes); i++ {
		avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)
		if avgLoss == 0 {
			result[i] = 100
		} else {
			rs := avgGain / avgLoss
			result[i] = 100 - (100 / (1 + rs))
		}
	}
	return result
}

func atrCalc(highs, lows, closes []float64, period int) []float64 {
	n := len(closes)
	result := make([]float64, n)
	for i := range result {
		result[i] = math.NaN()
	}
	if n < period+1 {
		return result
	}
	tr := make([]float64, n)
	tr[0] = highs[0] - lows[0]
	for i := 1; i < n; i++ {
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += tr[i]
	}
	result[period-1] = sum / float64(period)
	for i := period; i < n; i++ {
		result[i] = (result[i-1]*float64(period-1) + tr[i]) / float64(period)
	}
	return result
}

func crossoverLevel(series []float64, level float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return series[idx] > level && series[idx-1] <= level
}

func crossunderLevel(series []float64, level float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return series[idx] < level && series[idx-1] >= level
}

type Trade struct {
	No                 int     `json:"no"`
	Side               string  `json:"side"`
	EntryTimeUTC8      string  `json:"entry_time_utc8"`
	TVEntryTimeUTC8    string  `json:"tv_entry_time_utc8"`
	EntryPrice         float64 `json:"entry_price"`
	EntryATR           float64 `json:"entry_atr"`
	InitStopLoss       float64 `json:"init_stop_loss"`
	InitTakeProfit     float64 `json:"init_take_profit"`
	ExitTimeUTC8       string  `json:"exit_time_utc8"`
	TVExitTimeUTC8     string  `json:"tv_exit_time_utc8"`
	ExitPrice          float64 `json:"exit_price"`
	ExitReason         string  `json:"exit_reason"`
	BarsHeld           int     `json:"bars_held"`
	Qty                float64 `json:"qty"`
	PnL                float64 `json:"pnl_usdt"`
	BreakevenActivated bool    `json:"breakeven_activated"`
}

type btState struct {
	InPosition         bool
	Side               string
	EntryPrice         float64
	EntryATR           float64
	EntryBarIdx        int
	Qty                float64
	BreakevenActivated bool
	BESLPrice          float64
	EntryBarProcessed  bool
}

type Kline struct {
	OpenTime time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
}

type BacktestStats struct {
	TotalTrades  int     `json:"total_trades"`
	ClosedTrades int     `json:"closed_trades"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	WinRate      float64 `json:"win_rate"`
	TotalPnL     float64 `json:"total_pnl"`
	InitCapital  float64 `json:"init_capital"`
	FinalEquity  float64 `json:"final_equity"`
	ReturnPct    float64 `json:"return_pct"`
	MaxWin       float64 `json:"max_win"`
	MaxLoss      float64 `json:"max_loss"`
	AvgBars      float64 `json:"avg_bars"`
	MaxDrawdown  float64 `json:"max_drawdown"`
}

type BacktestResult struct {
	Symbol  string        `json:"symbol"`
	Version string        `json:"version"`
	Stats   BacktestStats `json:"stats"`
	Trades  []Trade       `json:"trades"`
}

func main() {
	debugMode = os.Getenv("DEBUG_TRADES") != ""
	debugTradeNos = make(map[int]bool)
	if dt := os.Getenv("DEBUG_TRADES"); dt != "" {
		for _, s := range strings.Split(dt, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				debugTradeNos[n] = true
			}
		}
	}

	symbol := os.Getenv("BACKTEST_SYMBOL")
	if symbol == "" {
		symbol = "ETHUSDT"
	}
	for _, arg := range os.Args[1:] {
		if len(arg) > 8 && arg[:8] == "-symbol=" {
			symbol = arg[8:]
		}
	}
	jsonMode := os.Getenv("BACKTEST_JSON") == "1"

	var mintick, qtyStep, minQty float64
	switch symbol {
	case "BTCUSDT":
		mintick = 0.1
		qtyStep = 0.001
		minQty = 0.001
	case "1000SHIBUSDT":
		mintick = 0.000001
		qtyStep = 1.0
		minQty = 1.0
	case "TAOUSDT":
		mintick = 0.01
		qtyStep = 0.001
		minQty = 0.001
	case "SOLUSDT":
		mintick = 0.01
		qtyStep = 0.01
		minQty = 0.01
	default:
		mintick = 0.01
		qtyStep = 0.001
		minQty = 0.001
	}

	initCapital := 15000.0
	baseRisk := 3.0
	maxHoldBars := 100
	maxDrawdown := 14.0
	nominalMultiplier := 4.0

	if !jsonMode {
		fmt.Printf("%s 1H V37 回测 (v21)\n", symbol)
		fmt.Printf("mintick=%v nomMul=%.1f\n", mintick, nominalMultiplier)
	}

	klines, err := fetchAllKlines(symbol, "1h")
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取K线失败: %v\n", err)
		os.Exit(1)
	}
	n := len(klines)
	cst := time.FixedZone("CST", 8*3600)
	if !jsonMode {
		fmt.Printf("K线: %d 根, %s ~ %s\n",
			n,
			klines[0].OpenTime.In(cst).Format("2006-01-02 15:04"),
			klines[n-1].OpenTime.In(cst).Format("2006-01-02 15:04"))
	}

	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	opens := make([]float64, n)
	for i, k := range klines {
		opens[i] = k.Open
		closes[i] = k.Close
		highs[i] = k.High
		lows[i] = k.Low
	}

	ema200 := emaCalc(closes, 200)
	rsi14 := rsiCalc(closes, 14)
	atr14 := atrCalc(highs, lows, closes, 14)

	equity := initCapital
	equityHistory := make([]float64, 0, n)
	peakEquity := initCapital
	maxDDPct := 0.0
	var state btState
	var allTrades []Trade

	startIdx := 200
	if startIdx >= n-1 {
		startIdx = n - 2
	}

	for i := startIdx; i < n-1; i++ {
		curClose := closes[i]
		curHigh := highs[i]
		curLow := lows[i]
		curOpen := opens[i]
		curEMA := ema200[i]
		curRSI := rsi14[i]
		curATR := atr14[i]

		if math.IsNaN(curEMA) || math.IsNaN(curRSI) || math.IsNaN(curATR) || curATR == 0 {
			equityHistory = append(equityHistory, equity)
			continue
		}
		equityHistory = append(equityHistory, equity)

		highEq := equity
		windowStart := len(equityHistory) - 1000
		if windowStart < 0 {
			windowStart = 0
		}
		for j := windowStart; j < len(equityHistory); j++ {
			if equityHistory[j] > highEq {
				highEq = equityHistory[j]
			}
		}

		if state.InPosition {
			curTradeNo := len(allTrades) + 1
			barTime := klines[i].OpenTime.In(cst).Format("2006-01-02 15:04")
			isDbg := debugMode && debugTradeNos[curTradeNo]

			// Entry bar (ebi+1): only initialize breakeven check, skip exit
			if i == state.EntryBarIdx+1 && !state.EntryBarProcessed {
				state.EntryBarProcessed = true
				if isDbg {
					fmt.Fprintf(os.Stderr, "[T#%d] ENTRY_BAR bar=%s C=%.2f\n",
						curTradeNo, barTime, curClose)
				}
				continue
			}

			if i < state.EntryBarIdx+2 {
				continue
			}

			// Use previous bar's ATR for all calculations (matches Pine's execution model)
			prevATR := atr14[i-1]
			if math.IsNaN(prevATR) || prevATR == 0 {
				prevATR = state.EntryATR
			}
			trailOff := prevATR * 3.0 * mintick
			trailPts := prevATR * 10.0 * mintick
			stopDist := prevATR * 2.0
			tpDist := prevATR * 2.0 * 3.5

			// Calculate SL/TP
			var stopLoss, takeProfit float64
			if state.BreakevenActivated {
				stopLoss = state.BESLPrice
			} else {
				if state.Side == "LONG" {
					stopLoss = state.EntryPrice - stopDist
				} else {
					stopLoss = state.EntryPrice + stopDist
				}
			}
			if state.Side == "LONG" {
				takeProfit = state.EntryPrice + tpDist
			} else {
				takeProfit = state.EntryPrice - tpDist
			}

			// Trail activation level (recalculated each bar)
			var activationLevel float64
			if state.Side == "LONG" {
				activationLevel = state.EntryPrice + trailPts
			} else {
				activationLevel = state.EntryPrice - trailPts
			}

			// Breakeven check using previous bar's close and ATR
			prevClose := closes[i-1]
			if !state.BreakevenActivated {
				if state.Side == "LONG" && prevClose > state.EntryPrice+prevATR*1.5 {
					state.BreakevenActivated = true
					state.BESLPrice = state.EntryPrice + prevATR*0.2
					stopLoss = state.BESLPrice
				} else if state.Side == "SHORT" && prevClose < state.EntryPrice-prevATR*1.5 {
					state.BreakevenActivated = true
					state.BESLPrice = state.EntryPrice - prevATR*0.2
					stopLoss = state.BESLPrice
				}
			}

			exitReason := ""
			exitPrice := 0.0
			h := curHigh
			l := curLow
			o := curOpen
			c := curClose

			// OHLC order: |O-H| < |O-L| → highFirst (O→H→L→C)
			highFirst := math.Abs(o-h) < math.Abs(o-l)

			// Trail stop is recalculated FRESH each bar (strategy.exit replaces order each bar)
			localTrailActive := false
			localTrailStop := 0.0

			if isDbg {
				fmt.Fprintf(os.Stderr, "[T#%d] bar=%s O=%.2f H=%.2f L=%.2f C=%.2f hf=%v actLvl=%.2f trOff=%.4f SL=%.2f TP=%.2f BE=%v\n",
					curTradeNo, barTime, o, h, l, c, highFirst,
					activationLevel, trailOff, stopLoss, takeProfit, state.BreakevenActivated)
			}

			if state.Side == "LONG" {
				if highFirst {
					// O → H → L → C
					// Phase O: gap checks + trail activation
					if o <= stopLoss {
						exitReason = "止损(SL-Gap)"
						if state.BreakevenActivated { exitReason = "保本止损(BE-SL-Gap)" }
						exitPrice = o
					} else if o >= takeProfit {
						exitReason = "止盈(TP-Gap)"
						exitPrice = o
					}
					if exitReason == "" && o >= activationLevel {
						localTrailActive = true
						localTrailStop = o - trailOff
						if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@O(hf) o=%.2f ts=%.2f\n", o, localTrailStop) }
					}
					// Phase H: check TP, then activate/update trail
					if exitReason == "" && h >= takeProfit {
						exitReason = "止盈(TP)"
						exitPrice = takeProfit
					}
					if exitReason == "" {
						if h >= activationLevel {
							localTrailActive = true
							localTrailStop = h - trailOff
							if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@H h=%.2f ts=%.2f\n", h, localTrailStop) }
						}
					}
					// Phase L: check trail exit, check SL
					if exitReason == "" {
						if localTrailActive && l <= localTrailStop {
							exitReason = "追踪止损(Trail)"
							exitPrice = localTrailStop
							if isDbg { fmt.Fprintf(os.Stderr, "  EXIT: trail@L l=%.2f<=ts=%.2f\n", l, localTrailStop) }
						} else if l <= stopLoss {
							exitReason = "止损(SL)"
							if state.BreakevenActivated { exitReason = "保本止损(BE-SL)" }
							exitPrice = stopLoss
						}
					}
					// Phase C: check trail exit
					if exitReason == "" && localTrailActive && c <= localTrailStop {
						exitReason = "追踪止损(Trail)"
						exitPrice = localTrailStop
					}
				} else {
					// O → L → H → C
					// Phase O: gap checks + trail activation
					if o <= stopLoss {
						exitReason = "止损(SL-Gap)"
						if state.BreakevenActivated { exitReason = "保本止损(BE-SL-Gap)" }
						exitPrice = o
					} else if o >= takeProfit {
						exitReason = "止盈(TP-Gap)"
						exitPrice = o
					}
					if exitReason == "" && o >= activationLevel {
						localTrailActive = true
						localTrailStop = o - trailOff
						if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@O(lf) o=%.2f ts=%.2f\n", o, localTrailStop) }
					}
					// Phase L: check trail exit (if O activated), check SL
					if exitReason == "" {
						if localTrailActive && l <= localTrailStop {
							exitReason = "追踪止损(Trail)"
							exitPrice = localTrailStop
							if isDbg { fmt.Fprintf(os.Stderr, "  EXIT: trail@L(lf) l=%.2f<=ts=%.2f\n", l, localTrailStop) }
						} else if l <= stopLoss {
							exitReason = "止损(SL)"
							if state.BreakevenActivated { exitReason = "保本止损(BE-SL)" }
							exitPrice = stopLoss
						}
					}
					// Phase H: check TP, activate trail
					if exitReason == "" && h >= takeProfit {
						exitReason = "止盈(TP)"
						exitPrice = takeProfit
					}
					if exitReason == "" {
						if h >= activationLevel {
							localTrailActive = true
							localTrailStop = h - trailOff
							if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@H(lf) h=%.2f ts=%.2f\n", h, localTrailStop) }
						}
					}
					// Phase C: check trail exit
					if exitReason == "" && localTrailActive && c <= localTrailStop {
						exitReason = "追踪止损(Trail)"
						exitPrice = localTrailStop
					}
				}
			} else {
				// SHORT
				if highFirst {
					// O → H → L → C
					// Phase O: gap checks + trail activation
					if o >= stopLoss {
						exitReason = "止损(SL-Gap)"
						if state.BreakevenActivated { exitReason = "保本止损(BE-SL-Gap)" }
						exitPrice = o
					} else if o <= takeProfit {
						exitReason = "止盈(TP-Gap)"
						exitPrice = o
					}
					if exitReason == "" && o <= activationLevel {
						localTrailActive = true
						localTrailStop = o + trailOff
						if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@O(hf) o=%.2f ts=%.2f\n", o, localTrailStop) }
					}
					// Phase H: check trail exit (if O activated), check SL
					if exitReason == "" {
						if localTrailActive && h >= localTrailStop {
							exitReason = "追踪止损(Trail)"
							exitPrice = localTrailStop
							if isDbg { fmt.Fprintf(os.Stderr, "  EXIT: trail@H(hf) h=%.2f>=ts=%.2f\n", h, localTrailStop) }
						} else if h >= stopLoss {
							exitReason = "止损(SL)"
							if state.BreakevenActivated { exitReason = "保本止损(BE-SL)" }
							exitPrice = stopLoss
						}
					}
					// Phase L: check TP, activate/update trail
					if exitReason == "" && l <= takeProfit {
						exitReason = "止盈(TP)"
						exitPrice = takeProfit
					}
					if exitReason == "" {
						if l <= activationLevel {
							localTrailActive = true
							localTrailStop = l + trailOff
							if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@L l=%.2f ts=%.2f\n", l, localTrailStop) }
						}
					}
					// Phase C: check trail exit
					if exitReason == "" && localTrailActive && c >= localTrailStop {
						exitReason = "追踪止损(Trail)"
						exitPrice = localTrailStop
						if isDbg { fmt.Fprintf(os.Stderr, "  EXIT: trail@C c=%.2f>=ts=%.2f\n", c, localTrailStop) }
					}
				} else {
					// O → L → H → C
					// Phase O: gap checks + trail activation
					if o >= stopLoss {
						exitReason = "止损(SL-Gap)"
						if state.BreakevenActivated { exitReason = "保本止损(BE-SL-Gap)" }
						exitPrice = o
					} else if o <= takeProfit {
						exitReason = "止盈(TP-Gap)"
						exitPrice = o
					}
					if exitReason == "" && o <= activationLevel {
						localTrailActive = true
						localTrailStop = o + trailOff
						if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@O(lf) o=%.2f ts=%.2f\n", o, localTrailStop) }
					}
					// Phase L: check TP, activate/update trail
					if exitReason == "" && l <= takeProfit {
						exitReason = "止盈(TP)"
						exitPrice = takeProfit
					}
					if exitReason == "" {
						if l <= activationLevel {
							localTrailActive = true
							localTrailStop = l + trailOff
							if isDbg { fmt.Fprintf(os.Stderr, "  TRAIL_ACT@L(lf) l=%.2f ts=%.2f\n", l, localTrailStop) }
						}
					}
					// Phase H: check trail exit, check SL
					if exitReason == "" {
						if localTrailActive && h >= localTrailStop {
							exitReason = "追踪止损(Trail)"
							exitPrice = localTrailStop
							if isDbg { fmt.Fprintf(os.Stderr, "  EXIT: trail@H(lf) h=%.2f>=ts=%.2f\n", h, localTrailStop) }
						} else if h >= stopLoss {
							exitReason = "止损(SL)"
							if state.BreakevenActivated { exitReason = "保本止损(BE-SL)" }
							exitPrice = stopLoss
						}
					}
					// Phase C: check trail exit
					if exitReason == "" && localTrailActive && c >= localTrailStop {
						exitReason = "追踪止损(Trail)"
						exitPrice = localTrailStop
					}
				}
			}

			if exitReason == "" && (i-state.EntryBarIdx) >= maxHoldBars {
				exitReason = "超时强平(Timeout)"
				exitPrice = curClose
			}

			if exitReason != "" {
				var pnl float64
				if state.Side == "LONG" {
					pnl = (exitPrice - state.EntryPrice) * state.Qty
				} else {
					pnl = (state.EntryPrice - exitPrice) * state.Qty
				}
				initSL := state.EntryPrice - state.EntryATR*2.0
				if state.Side == "SHORT" { initSL = state.EntryPrice + state.EntryATR*2.0 }
				initTP := state.EntryPrice + state.EntryATR*2.0*3.5
				if state.Side == "SHORT" { initTP = state.EntryPrice - state.EntryATR*2.0*3.5 }

				signalBarTime := klines[state.EntryBarIdx].OpenTime.In(cst)
				tvEntryTime := klines[state.EntryBarIdx+1].OpenTime.In(cst)
				exitBarTime := klines[i].OpenTime.In(cst)

				t := Trade{
					No: len(allTrades) + 1, Side: state.Side,
					EntryTimeUTC8: signalBarTime.Format("2006-01-02 15:04"),
					TVEntryTimeUTC8: tvEntryTime.Format("2006-01-02 15:04"),
					EntryPrice: state.EntryPrice, EntryATR: state.EntryATR,
					InitStopLoss: initSL, InitTakeProfit: initTP,
					ExitTimeUTC8: exitBarTime.Format("2006-01-02 15:04"),
					TVExitTimeUTC8: exitBarTime.Format("2006-01-02 15:04"),
					ExitPrice: exitPrice, ExitReason: exitReason,
					BarsHeld: i - state.EntryBarIdx, Qty: state.Qty,
					PnL: pnl, BreakevenActivated: state.BreakevenActivated,
				}
				allTrades = append(allTrades, t)
				if isDbg {
					fmt.Fprintf(os.Stderr, "[T#%d] === EXIT === %s @ %.2f pnl=%.2f\n\n", curTradeNo, exitReason, exitPrice, pnl)
				}
				equity += pnl
				if equity > peakEquity { peakEquity = equity }
				if peakEquity > 0 {
					dd := (peakEquity - equity) / peakEquity * 100
					if dd > maxDDPct { maxDDPct = dd }
				}
				state = btState{}
			}
		}

		if !state.InPosition {
			drawdown := 0.0
			if highEq > 0 { drawdown = (highEq - equity) / highEq * 100 }
			if drawdown >= maxDrawdown { continue }

			rsiCrossover45 := crossoverLevel(rsi14, 45, i)
			rsiCrossunder55 := crossunderLevel(rsi14, 55, i)

			if curClose > curEMA && rsiCrossover45 {
				stopDist := curATR * 2.0
				riskAmt := equity * (baseRisk / 100)
				qty := riskAmt / stopDist
				maxQty := (equity * nominalMultiplier) / curClose
				if qty > maxQty { qty = maxQty }
				if qtyStep >= 1.0 { qty = math.Floor(qty) } else { qty = math.Floor(qty/qtyStep) * qtyStep }
				if qty >= minQty {
					state = btState{InPosition: true, Side: "LONG", EntryPrice: curClose, EntryATR: curATR, EntryBarIdx: i, Qty: qty}
					if debugMode && debugTradeNos[len(allTrades)+1] {
						fmt.Fprintf(os.Stderr, "\n[T#%d] ENTRY LONG @ %.2f qty=%.6f ATR=%.8f bar=%s\n",
							len(allTrades)+1, curClose, qty, curATR, klines[i].OpenTime.In(cst).Format("2006-01-02 15:04"))
					}
				}
			}
			if !state.InPosition && curClose < curEMA && rsiCrossunder55 {
				stopDist := curATR * 2.0
				riskAmt := equity * (baseRisk / 100)
				qty := riskAmt / stopDist
				maxQty := (equity * nominalMultiplier) / curClose
				if qty > maxQty { qty = maxQty }
				if qtyStep >= 1.0 { qty = math.Floor(qty) } else { qty = math.Floor(qty/qtyStep) * qtyStep }
				if qty >= minQty {
					state = btState{InPosition: true, Side: "SHORT", EntryPrice: curClose, EntryATR: curATR, EntryBarIdx: i, Qty: qty}
					if debugMode && debugTradeNos[len(allTrades)+1] {
						fmt.Fprintf(os.Stderr, "\n[T#%d] ENTRY SHORT @ %.2f qty=%.6f ATR=%.8f bar=%s\n",
							len(allTrades)+1, curClose, qty, curATR, klines[i].OpenTime.In(cst).Format("2006-01-02 15:04"))
					}
				}
			}
		}
	}

	if state.InPosition {
		lastIdx := n - 2
		var pnl float64
		if state.Side == "LONG" { pnl = (closes[lastIdx] - state.EntryPrice) * state.Qty } else { pnl = (state.EntryPrice - closes[lastIdx]) * state.Qty }
		initSL := state.EntryPrice - state.EntryATR*2.0
		if state.Side == "SHORT" { initSL = state.EntryPrice + state.EntryATR*2.0 }
		initTP := state.EntryPrice + state.EntryATR*2.0*3.5
		if state.Side == "SHORT" { initTP = state.EntryPrice - state.EntryATR*2.0*3.5 }
		allTrades = append(allTrades, Trade{
			No: len(allTrades) + 1, Side: state.Side,
			EntryTimeUTC8: klines[state.EntryBarIdx].OpenTime.In(cst).Format("2006-01-02 15:04"),
			TVEntryTimeUTC8: klines[state.EntryBarIdx+1].OpenTime.In(cst).Format("2006-01-02 15:04"),
			EntryPrice: state.EntryPrice, EntryATR: state.EntryATR,
			InitStopLoss: initSL, InitTakeProfit: initTP,
			ExitTimeUTC8: klines[lastIdx].OpenTime.In(cst).Format("2006-01-02 15:04"),
			TVExitTimeUTC8: klines[lastIdx].OpenTime.In(cst).Format("2006-01-02 15:04"),
			ExitPrice: closes[lastIdx], ExitReason: "持仓中(Open)",
			BarsHeld: lastIdx - state.EntryBarIdx, Qty: state.Qty, PnL: pnl,
			BreakevenActivated: state.BreakevenActivated,
		})
	}

	total := len(allTrades)
	wins, losses, openPos := 0, 0, 0
	totalPnL := 0.0
	for _, t := range allTrades {
		totalPnL += t.PnL
		if t.ExitReason == "持仓中(Open)" { openPos++ } else if t.PnL > 0 { wins++ } else { losses++ }
	}
	closed := total - openPos

	if jsonMode {
		winRate := 0.0
		if closed > 0 { winRate = float64(wins) / float64(closed) * 100 }
		maxWin, maxLoss := 0.0, 0.0
		totalBars := 0
		for _, t := range allTrades {
			if t.ExitReason != "持仓中(Open)" {
				if t.PnL > maxWin { maxWin = t.PnL }
				if t.PnL < maxLoss { maxLoss = t.PnL }
				totalBars += t.BarsHeld
			}
		}
		avgBars := 0.0
		if closed > 0 { avgBars = float64(totalBars) / float64(closed) }
		result := BacktestResult{Symbol: symbol, Version: "v21",
			Stats: BacktestStats{TotalTrades: total, ClosedTrades: closed, Wins: wins, Losses: losses,
				WinRate: winRate, TotalPnL: totalPnL, InitCapital: initCapital, FinalEquity: equity,
				ReturnPct: (equity - initCapital) / initCapital * 100, MaxWin: maxWin, MaxLoss: maxLoss,
				AvgBars: avgBars, MaxDrawdown: maxDDPct},
			Trades: allTrades}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	fmt.Printf("\n全部 %d 笔交易:\n", total)
	fmt.Printf("%-4s %-6s %-17s %-10s %-17s %-10s %-10s %-20s\n",
		"#", "方向", "TV开仓时间", "开仓价", "TV平仓时间", "平仓价", "盈亏", "原因")
	fmt.Println(strings.Repeat("-", 110))
	for _, t := range allTrades {
		fmt.Printf("%-4d %-6s %-17s %-10.2f %-17s %-10.2f %+-10.2f %-20s\n",
			t.No, t.Side, t.TVEntryTimeUTC8, t.EntryPrice,
			t.TVExitTimeUTC8, t.ExitPrice, t.PnL, t.ExitReason)
	}
	fmt.Printf("\n统计: %d笔 %d胜 %d负 总PnL=%+.2f 权益=%.2f 回撤=%.2f%%\n",
		closed, wins, losses, totalPnL, equity, maxDDPct)

	saveCSV(allTrades, fmt.Sprintf("/tmp/backtest_v21_%s.csv", symbol))
}

func fetchAllKlines(symbol, interval string) ([]Kline, error) {
	var allKlines []Kline
	startTime := int64(0)
	for {
		url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=1500", symbol, interval)
		if startTime > 0 { url += fmt.Sprintf("&startTime=%d", startTime) }
		resp, err := http.Get(url)
		if err != nil { return nil, err }
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil { return nil, err }
		var raw [][]interface{}
		if err := json.Unmarshal(body, &raw); err != nil { return nil, err }
		if len(raw) == 0 { break }
		for _, r := range raw {
			openTimeMs, _ := r[0].(float64)
			open, _ := strconv.ParseFloat(r[1].(string), 64)
			high, _ := strconv.ParseFloat(r[2].(string), 64)
			low, _ := strconv.ParseFloat(r[3].(string), 64)
			closeP, _ := strconv.ParseFloat(r[4].(string), 64)
			vol, _ := strconv.ParseFloat(r[5].(string), 64)
			allKlines = append(allKlines, Kline{OpenTime: time.UnixMilli(int64(openTimeMs)), Open: open, High: high, Low: low, Close: closeP, Volume: vol})
		}
		lastTime := int64(raw[len(raw)-1][0].(float64))
		startTime = lastTime + 3600000
		if len(raw) < 1500 { break }
		time.Sleep(200 * time.Millisecond)
	}
	return allKlines, nil
}

func saveCSV(trades []Trade, path string) {
	f, _ := os.Create(path)
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"#", "方向", "信号K线时间", "TV开仓时间", "开仓价", "初始止损", "初始止盈", "平仓K线时间", "TV平仓时间", "平仓价", "盈亏", "持仓K线", "原因", "保本", "数量", "ATR"})
	for _, t := range trades {
		be := "否"
		if t.BreakevenActivated { be = "是" }
		_ = w.Write([]string{strconv.Itoa(t.No), t.Side, t.EntryTimeUTC8, t.TVEntryTimeUTC8,
			fmt.Sprintf("%.6f", t.EntryPrice), fmt.Sprintf("%.6f", t.InitStopLoss), fmt.Sprintf("%.6f", t.InitTakeProfit),
			t.ExitTimeUTC8, t.TVExitTimeUTC8, fmt.Sprintf("%.6f", t.ExitPrice), fmt.Sprintf("%.4f", t.PnL),
			strconv.Itoa(t.BarsHeld), t.ExitReason, be, fmt.Sprintf("%.6f", t.Qty), fmt.Sprintf("%.8f", t.EntryATR)})
	}
}
