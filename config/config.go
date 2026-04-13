package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config 全局配置
type Config struct {
	APIKey    string
	SecretKey string
	UseTestnet bool

	// 策略参数（完全对应 Pine Script）
	// baseRisk = input.float(3.0, "单笔风险 (%)")
	BaseRisk    float64
	// maxHoldBars = input.int(100, "最大持仓 K 线数")
	MaxHoldBars int
	// drawdown < 14.0 时才允许开仓
	MaxDrawdown float64
	// 策略初始资金，对应 Pine Script 的 initial_capital
	// 本系统中：InitCapital = 账户保证金余额（实时读取）
	// 用户设定的基准值仅用于首次启动时的回撤历史初始化
	InitCapital float64

	// 杠杆设置
		// 用户要求：20倍全仓杠杆（账户余额 × 20 = 名义仓位）
		Leverage int

		// 名义仓位倍数（名义仓位 = 账户余额 × NominalMultiplier）
		// 对应用户需求：3500 余额 × 20 = 70000 USDT 名义仓位
		NominalMultiplier float64

	// Web 服务
	WebPort string

	// 交易对（只交易 ETH）
	Symbols []string
}

// Load 从环境变量加载配置
func Load() *Config {
	_ = godotenv.Load(".env")

	cfg := &Config{
		APIKey:            getEnv("BINANCE_API_KEY", ""),
		SecretKey:         getEnv("BINANCE_SECRET_KEY", ""),
		UseTestnet:        getEnvBool("USE_TESTNET", true),
		BaseRisk:          getEnvFloat("BASE_RISK", 3.0),
		MaxHoldBars:       getEnvInt("MAX_HOLD_BARS", 100),
		MaxDrawdown:       getEnvFloat("MAX_DRAWDOWN", 14.0),
		InitCapital:       getEnvFloat("INIT_CAPITAL", 15000.0),
		Leverage:          getEnvInt("LEVERAGE", 20),
		NominalMultiplier: getEnvFloat("NOMINAL_MULTIPLIER", 20.0),
		WebPort:           getEnv("WEB_PORT", "8080"),
		Symbols:           getEnvSymbols("SYMBOLS", []string{"ETHUSDT"}),
	}
	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getEnvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getEnvSymbols(key string, def []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return def
	}
	return result
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
