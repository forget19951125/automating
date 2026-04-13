package strategy

import "binance_bot/orderbook"

// GetStates 返回 interface{} 以满足 web.StrategyController 接口
func (e *Engine) GetStates() interface{} {
	result := make(map[string]orderbook.Snapshot)
	e.mu.RLock()
	defer e.mu.RUnlock()
	for sym := range e.symbols {
		if state, ok := e.states[sym]; ok {
			result[sym] = state.GetSnapshot()
		}
	}
	return result
}
