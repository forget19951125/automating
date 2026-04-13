package indicator

import "math"

// EMA 计算指数移动平均线
// 返回与输入等长的切片，前 period-1 个值为 NaN
func EMA(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	if len(closes) < period {
		for i := range result {
			result[i] = math.NaN()
		}
		return result
	}

	k := 2.0 / float64(period+1)
	// 初始 SMA
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

// RSI 计算相对强弱指数（Wilder's RMA，与 Pine Script ta.rsi() 完全一致）
// 初始种子：用索引 0..period-1 的 gain/loss 平均（与 Pine Script 一致）
func RSI(closes []float64, period int) []float64 {
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

	// 初始平均：用索引 1..period 的 gain/loss（对应 Pine Script 的前 period 个 diff）
	// Pine Script: ta.rsi() 的 RMA 种子 = SMA of first period values
	// 第一个 diff 在索引 1（closes[1]-closes[0]），所以种子用 gains[1..period]
	avgGain := 0.0
	avgLoss := 0.0
	for i := 1; i <= period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// 第一个有效 RSI 在索引 period（对应 period 根 K 线后）
	if avgLoss == 0 {
		result[period] = 100
	} else {
		rs := avgGain / avgLoss
		result[period] = 100 - (100 / (1 + rs))
	}

	// Wilder 平滑（RMA）：alpha = 1/period
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

// ATR 计算真实波动幅度（Wilder 平滑）
func ATR(highs, lows, closes []float64, period int) []float64 {
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

	// 初始 ATR = SMA of first period TRs
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

// Crossover 检测 a 上穿 b（当前 a>b，前一根 a<=b）
func Crossover(a, b []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return a[idx] > b[idx] && a[idx-1] <= b[idx-1]
}

// Crossunder 检测 a 下穿 b（当前 a<b，前一根 a>=b）
func Crossunder(a, b []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return a[idx] < b[idx] && a[idx-1] >= b[idx-1]
}

// CrossoverLevel 检测 series 上穿某个固定值
func CrossoverLevel(series []float64, level float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return series[idx] > level && series[idx-1] <= level
}

// CrossunderLevel 检测 series 下穿某个固定值
func CrossunderLevel(series []float64, level float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return series[idx] < level && series[idx-1] >= level
}

// Highest 计算最近 n 个值中的最大值
func Highest(series []float64, n, idx int) float64 {
	start := idx - n + 1
	if start < 0 {
		start = 0
	}
	max := series[start]
	for i := start + 1; i <= idx; i++ {
		if series[i] > max {
			max = series[i]
		}
	}
	return max
}
