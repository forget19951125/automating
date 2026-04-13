package riskmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"binance_bot/config"
	"binance_bot/indicator"
)

const highEqFile = "/home/ubuntu/binance_bot_new/data/high_equity.json"

type highEqData struct {
	HighEq float64 `json:"high_eq"`
}

func loadHighEq() float64 {
	data, err := os.ReadFile(highEqFile)
	if err != nil {
		return 0
	}
	var d highEqData
	if err := json.Unmarshal(data, &d); err != nil {
		return 0
	}
	fmt.Printf("[RiskMgr] 从文件恢复历史最高权益: %.2f USDT\n", d.HighEq)
	return d.HighEq
}

func saveHighEq(v float64) {
	_ = os.MkdirAll(filepath.Dir(highEqFile), 0755)
	data, _ := json.Marshal(highEqData{HighEq: v})
	_ = os.WriteFile(highEqFile, data, 0644)
}

// RiskManager 管理风控逻辑，对应 Pine Script 中的动态风控锁
type RiskManager struct {
	mu            sync.RWMutex
	cfg           *config.Config
	equityHistory []float64
	highEq        float64
	backtestMode  bool // 回测模式：不读写文件
	initialized   bool // 是否已用实际余额校验过 highEq
}

// New 创建 RiskManager，highEq 初始化为 0，首次 UpdateEquity 调用时从实际账户余额开始
func New(cfg *config.Config) *RiskManager {
	return &RiskManager{
		cfg:    cfg,
		highEq: loadHighEq(), // 从文件恢复历史最高权益
	}
}

// UpdateEquity 更新权益历史，对应 Pine Script 的 ta.highest(currEq, 1000)
//
// 首次调用时会校验 highEq 的合理性：
//   - 如果文件中的 highEq 为 0，初始化为当前余额
//   - 如果文件中的 highEq 远大于当前余额（回撤超过 50%），说明是脏数据，重置为当前余额
//     这防止了历史遗留的 highEq 永久锁死系统
func (rm *RiskManager) UpdateEquity(equity float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// 首次调用：校验 highEq 合理性
	if !rm.initialized {
		rm.initialized = true

		if rm.highEq == 0 {
			// 从未记录过，初始化为当前余额
			rm.highEq = equity
			if !rm.backtestMode {
				saveHighEq(rm.highEq)
			}
			fmt.Printf("[RiskMgr] 首次初始化 highEq = %.2f USDT\n", rm.highEq)
		} else if equity > 0 && rm.highEq > equity {
			// 文件中的 highEq 大于当前余额，检查回撤是否合理
			drawdown := (rm.highEq - equity) / rm.highEq * 100
			if drawdown > 50 {
				// 回撤超过 50%，说明 highEq 是脏数据（如之前不同账户或测试数据）
				// 重置为当前余额，避免永久锁死
				fmt.Printf("[RiskMgr] ⚠️ 检测到 highEq(%.2f) 远大于当前余额(%.2f)，回撤 %.1f%% > 50%%，疑似脏数据，重置为当前余额\n",
					rm.highEq, equity, drawdown)
				rm.highEq = equity
				if !rm.backtestMode {
					saveHighEq(rm.highEq)
				}
			}
		}
	}

	rm.equityHistory = append(rm.equityHistory, equity)
	if len(rm.equityHistory) > 1000 {
		rm.equityHistory = rm.equityHistory[len(rm.equityHistory)-1000:]
	}

	idx := len(rm.equityHistory) - 1
	newHigh := indicator.Highest(rm.equityHistory, 1000, idx)
	if newHigh > rm.highEq {
		rm.highEq = newHigh
		if !rm.backtestMode {
			saveHighEq(rm.highEq)
		}
	}
}

// CanTrade 检查是否允许开仓（对应 Pine Script: canTrade = drawdown < maxDrawdown）
func (rm *RiskManager) CanTrade(currentEquity float64) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.highEq <= 0 {
		return true
	}
	drawdown := (rm.highEq - currentEquity) / rm.highEq * 100
	return drawdown < rm.cfg.MaxDrawdown
}

// Drawdown 计算当前回撤百分比
func (rm *RiskManager) Drawdown(currentEquity float64) float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.highEq <= 0 {
		return 0
	}
	return (rm.highEq - currentEquity) / rm.highEq * 100
}

// HighEquity 获取历史最高权益
func (rm *RiskManager) HighEquity() float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.highEq
}

// NewRiskManager 是 New 的别名，保持向后兼容
func NewRiskManager(cfg *config.Config) *RiskManager {
	return New(cfg)
}

// ResetForBacktest 重置历史最高权益为指定值，并开启回测模式（不读写文件）
func (rm *RiskManager) ResetForBacktest(initCapital float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.highEq = initCapital
	rm.equityHistory = nil
	rm.backtestMode = true // 回测模式：不写入文件
	rm.initialized = true  // 回测模式视为已初始化
}
