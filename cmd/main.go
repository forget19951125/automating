package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"binance_bot/config"
	"binance_bot/exchange"
	"binance_bot/riskmanager"
	"binance_bot/strategy"
	"binance_bot/web"
)

func main() {
	fmt.Println("=== BTC/ETH 自动化交易系统 V37 ===")
	fmt.Println("正在初始化...")

	// 加载配置
	cfg := config.Load()

	if cfg.APIKey == "" || cfg.SecretKey == "" {
		fmt.Println("⚠️  警告: 未配置 API Key，将以演示模式运行（无法实际交易）")
		fmt.Println("   请在 .env 文件中配置 BINANCE_API_KEY 和 BINANCE_SECRET_KEY")
	}

	if cfg.UseTestnet {
		fmt.Println("🔵 运行模式: 测试网 (Testnet)")
	} else {
		fmt.Println("🔴 运行模式: 正式网 (Mainnet) - 请谨慎操作！")
	}

	// 创建交易所客户端
	client := exchange.NewClient(cfg)

	// 创建风控管理器
	riskMgr := riskmanager.NewRiskManager(cfg)

	// 创建策略引擎
	engine := strategy.NewEngine(cfg, client, riskMgr)

	// 创建 Web 服务器
	server := web.NewServer(cfg, client, riskMgr, engine)

	// 记录启动日志
	web.Info("SYSTEM", fmt.Sprintf("系统启动 | 交易对: %v | 杠杆: %dx | 名义倍数: %.0fx | 测试网: %v",
		cfg.Symbols, cfg.Leverage, cfg.NominalMultiplier, cfg.UseTestnet))

	// 优雅退出处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("\n正在优雅退出...")
		engine.Stop()
		web.Info("SYSTEM", "系统正在关闭...")
		os.Exit(0)
	}()

	// 自动启动策略引擎（无需手动点击界面按鈕）
	engine.Start()
	web.Info("SYSTEM", "策略引擎已自动启动")

	// 启动 Web 服务器（阻塞）
	fmt.Printf("🌐 Web 管理界面: http://localhost:%s\n", cfg.WebPort)
	fmt.Println("   策略引擎已自动启动")
	fmt.Println("   按 Ctrl+C 退出")
	if err := server.Run(); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
		os.Exit(1)
	}
}
