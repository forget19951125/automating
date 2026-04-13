package exchange

import "math"

// CalcATR 计算 ATR（供外部包调用）
func CalcATR(highs, lows, closes []float64, period int) []float64 {
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
