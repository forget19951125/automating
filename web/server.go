package web

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"binance_bot/config"
	"binance_bot/exchange"
	"binance_bot/riskmanager"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// StrategyController 策略控制接口
type StrategyController interface {
	Start()
	Stop()
	IsRunning() bool
	ForceCloseAll()
	GetStates() interface{}
	AddSymbol(symbol string) error
	RemoveSymbol(symbol string) error
	GetSymbols() []string
}

// Server Web 服务器
type Server struct {
	cfg     *config.Config
	client  *exchange.Client
	riskMgr *riskmanager.RiskManager
	engine  StrategyController
	router  *gin.Engine
}

// NewServer 创建 Web 服务器
func NewServer(cfg *config.Config, client *exchange.Client, riskMgr *riskmanager.RiskManager, engine StrategyController) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:    []string{"Origin", "Content-Type"},
		MaxAge:          12 * time.Hour,
	}))

	s := &Server{cfg: cfg, client: client, riskMgr: riskMgr, engine: engine, router: r}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	r := s.router
	r.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, indexHTML)
	})
	api := r.Group("/api")
	{
		api.GET("/account", s.handleGetAccount)
		api.GET("/status", s.handleGetStatus)
		api.GET("/positions", s.handleGetPositions)
		api.GET("/logs", s.handleGetLogs)
		api.GET("/config", s.handleGetConfig)
		// 币种管理
		api.GET("/symbols", s.handleGetSymbols)
		api.POST("/symbols", s.handleAddSymbol)
		api.DELETE("/symbols/:symbol", s.handleRemoveSymbol)
		// 动态仓位预览（核心新功能）
		api.GET("/position-preview", s.handlePositionPreview)
		api.POST("/start", s.handleStart)
		api.POST("/stop", s.handleStop)
		api.POST("/close-all", s.handleCloseAll)
		api.GET("/backtest", s.handleBacktest)
	}
}

func (s *Server) Run() error {
	addr := fmt.Sprintf(":%s", s.cfg.WebPort)
	Info("SYSTEM", fmt.Sprintf("Web 服务器启动于 http://0.0.0.0%s", addr))
	return s.router.Run(addr)
}

func (s *Server) handleGetAccount(c *gin.Context) {
	account, err := s.client.GetAccountInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	currEq := account.TotalBalance
	drawdown := s.riskMgr.Drawdown(currEq)
	highEq := s.riskMgr.HighEquity()
	canTrade := s.riskMgr.CanTrade(currEq)

	c.JSON(http.StatusOK, gin.H{
		"total_balance":      account.TotalBalance,
		"available_balance":  account.AvailableBalance,
		"unrealized_pnl":     account.UnrealizedPnL,
		"total_equity":       account.TotalEquity,
		"curr_eq":            currEq,
		"nominal_limit":      currEq * s.cfg.NominalMultiplier,
		"drawdown_pct":       drawdown,
		"high_equity":        highEq,
		"can_trade":          canTrade,
	})
}

func (s *Server) handleGetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"running":   s.engine.IsRunning(),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"states":    s.engine.GetStates(),
	})
}

func (s *Server) handleGetPositions(c *gin.Context) {
	positions := make([]interface{}, 0)
	for _, sym := range s.cfg.Symbols {
		pos, err := s.client.GetPosition(sym)
		if err != nil {
			continue
		}
		positions = append(positions, gin.H{
			"symbol":         pos.Symbol,
			"side":           pos.Side,
			"qty":            pos.Qty,
			"entry_price":    pos.EntryPrice,
			"unrealized_pnl": pos.UnrealizedPnL,
			"leverage":       pos.Leverage,
			"nominal_value":  pos.Qty * pos.EntryPrice,
		})
	}
	c.JSON(http.StatusOK, gin.H{"positions": positions})
}

func (s *Server) handleGetLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"logs": GlobalLogger.GetLast(100)})
}

func (s *Server) handleGetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"symbols":            s.cfg.Symbols,
		"leverage":           s.cfg.Leverage,
		"nominal_multiplier": s.cfg.NominalMultiplier,
		"base_risk":          s.cfg.BaseRisk,
		"max_hold_bars":      s.cfg.MaxHoldBars,
		"max_drawdown":       s.cfg.MaxDrawdown,
		"init_capital":       s.cfg.InitCapital,
		"use_testnet":        s.cfg.UseTestnet,
	})
}

// handlePositionPreview 动态仓位预览：实时计算当前如果触发信号会开多少仓
func (s *Server) handlePositionPreview(c *gin.Context) {
	// 支持 symbol 查询参数，默认 ETHUSDT
	symbol := c.DefaultQuery("symbol", "ETHUSDT")

	account, err := s.client.GetAccountInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	currEq := account.TotalBalance
	if currEq <= 0 {
		currEq = s.cfg.InitCapital
	}

	// 获取指定币种最新 K 线计算 ATR
	klines, err := s.client.GetKlines(symbol, "1h", 50)
	if err != nil || len(klines) < 20 {
		c.JSON(http.StatusOK, gin.H{"error": "K线数据不足"})
		return
	}

	n := len(klines)
	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	for i, k := range klines {
		closes[i] = k.Close
		highs[i] = k.High
		lows[i] = k.Low
	}

	atr14 := exchange.CalcATR(highs, lows, closes, 14)
	idx := n - 2
	currentATR := atr14[idx]
	currentPrice := closes[idx]

	if math.IsNaN(currentATR) || currentATR == 0 || currentPrice == 0 {
		c.JSON(http.StatusOK, gin.H{"error": "ATR 计算失败"})
		return
	}

	stopDist := currentATR * 2.0
	riskAmount := currEq * (s.cfg.BaseRisk / 100)
	tradeQty := riskAmount / stopDist

	maxNominal := currEq * s.cfg.NominalMultiplier
	maxQty := maxNominal / currentPrice
	capped := false
	if tradeQty > maxQty {
		tradeQty = maxQty
		capped = true
	}

	nominalValue := tradeQty * currentPrice
	marginRequired := nominalValue / float64(s.cfg.Leverage)
	stopLossLong := currentPrice - stopDist
	stopLossShort := currentPrice + stopDist
	tpLong := currentPrice + stopDist*3.5
	tpShort := currentPrice - stopDist*3.5

	c.JSON(http.StatusOK, gin.H{
		"symbol":           symbol,
		"curr_eq":          currEq,
		"current_price":    currentPrice,
		"current_atr":      currentATR,
		"stop_dist":        stopDist,
		"risk_amount":      riskAmount,
		"trade_qty":        tradeQty,
		"nominal_value":    nominalValue,
		"margin_required":  marginRequired,
		"max_nominal":      maxNominal,
		"capped_by_limit":  capped,
		"stop_loss_long":   stopLossLong,
		"stop_loss_short":  stopLossShort,
		"tp_long":          tpLong,
		"tp_short":         tpShort,
		"leverage":         s.cfg.Leverage,
		"base_risk_pct":    s.cfg.BaseRisk,
		"nominal_mult":     s.cfg.NominalMultiplier,
	})
}

func (s *Server) handleGetSymbols(c *gin.Context) {
	syms := s.engine.GetSymbols()
	c.JSON(http.StatusOK, gin.H{"symbols": syms})
}

func (s *Server) handleAddSymbol(c *gin.Context) {
	var req struct {
		Symbol string `json:"symbol"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 symbol 字段"})
		return
	}
	if err := s.engine.AddSymbol(req.Symbol); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": req.Symbol + " 已添加", "symbols": s.engine.GetSymbols()})
}

func (s *Server) handleRemoveSymbol(c *gin.Context) {
	symbol := c.Param("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 symbol"})
		return
	}
	if err := s.engine.RemoveSymbol(symbol); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": symbol + " 已删除", "symbols": s.engine.GetSymbols()})
}

func (s *Server) handleStart(c *gin.Context) {
	if s.engine.IsRunning() {
		c.JSON(http.StatusOK, gin.H{"message": "策略已在运行中"})
		return
	}
	s.engine.Start()
	c.JSON(http.StatusOK, gin.H{"message": "策略已启动"})
}

func (s *Server) handleStop(c *gin.Context) {
	s.engine.Stop()
	c.JSON(http.StatusOK, gin.H{"message": "策略已停止"})
}

func (s *Server) handleCloseAll(c *gin.Context) {
	s.engine.ForceCloseAll()
	c.JSON(http.StatusOK, gin.H{"message": "已发送强制平仓指令"})
}

// handleBacktest 运行回测并返回 JSON 结果
// 支持 symbol 查询参数：ETHUSDT / BTCUSDT / SOLUSDT（默认 ETHUSDT）
func (s *Server) handleBacktest(c *gin.Context) {
	symbol := c.DefaultQuery("symbol", "ETHUSDT")
	// 基本格式验证：转大写，必须以 USDT 结尾
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" || !strings.HasSuffix(symbol, "USDT") {
		symbol = "ETHUSDT"
	}
	cmd := exec.Command("/home/ubuntu/binance_bot_new/backtest_bin")
	cmd.Env = append(os.Environ(), "BACKTEST_JSON=1", "BACKTEST_SYMBOL="+symbol)
	out, err := cmd.Output()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("回测执行失败: %v", err)})
		return
	}
	// 从输出中提取 JSON 部分（跳过前面的文本输出）
	outStr := string(out)
	jsonStart := -1
	for i, ch := range outStr {
		if ch == '{' {
			jsonStart = i
			break
		}
	}
	if jsonStart < 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "回测输出中未找到 JSON 数据"})
		return
	}
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, outStr[jsonStart:])
}
