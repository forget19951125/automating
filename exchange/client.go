package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"binance_bot/config"

	binance "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
)

// Client 封装币安合约客户端
type Client struct {
	fc  *futures.Client
	cfg *config.Config
}

// Kline K线数据
type Kline struct {
	OpenTime  time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

// Position 持仓信息
type Position struct {
	Symbol        string
	Side          string  // "LONG" | "SHORT" | "NONE"
	Qty           float64
	EntryPrice    float64
	UnrealizedPnL float64
	Leverage      int
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalBalance    float64
	AvailableBalance float64
	UnrealizedPnL   float64
	TotalEquity     float64
}

// ExchangeInfo 交易对精度信息
type ExchangeInfo struct {
	StepSize  float64 // 数量精度
	TickSize  float64 // 价格精度
	MinQty    float64
	MinNotional float64
}

// NewClient 创建新的交易所客户端
func NewClient(cfg *config.Config) *Client {
	if cfg.UseTestnet {
		binance.UseTestnet = true
		futures.UseTestnet = true
	}
	fc := binance.NewFuturesClient(cfg.APIKey, cfg.SecretKey)
	return &Client{fc: fc, cfg: cfg}
}

// GetKlines 获取K线数据
func (c *Client) GetKlines(symbol, interval string, limit int) ([]Kline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	klines, err := c.fc.NewKlinesService().
		Symbol(symbol).
		Interval(interval).
		Limit(limit).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取K线失败 %s: %w", symbol, err)
	}

	result := make([]Kline, 0, len(klines))
	for _, k := range klines {
		openTime := time.UnixMilli(k.OpenTime)
		open, _ := strconv.ParseFloat(k.Open, 64)
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)
		closeP, _ := strconv.ParseFloat(k.Close, 64)
		vol, _ := strconv.ParseFloat(k.Volume, 64)
		result = append(result, Kline{
			OpenTime: openTime,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    closeP,
			Volume:   vol,
		})
	}
	return result, nil
}

// GetPosition 获取指定交易对持仓（兼容单向/双向持仓模式）
// 双向持仓模式下，返回 LONG 或 SHORT 中有仓位的那一方；若两方都有则返回 LONG
func (c *Client) GetPosition(symbol string) (*Position, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	positions, err := c.fc.NewGetPositionRiskService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败 %s: %w", symbol, err)
	}

	pos := &Position{Symbol: symbol, Side: "NONE"}
	for _, p := range positions {
		if p.Symbol != symbol {
			continue
		}
		qty, _ := strconv.ParseFloat(p.PositionAmt, 64)
		entry, _ := strconv.ParseFloat(p.EntryPrice, 64)
		pnl, _ := strconv.ParseFloat(p.UnRealizedProfit, 64)
		lev, _ := strconv.Atoi(p.Leverage)

		absQty := math.Abs(qty)
		if absQty == 0 {
			continue
		}

		// 双向持仓模式：PositionSide 字段为 "LONG" 或 "SHORT"
		// 单向持仓模式：PositionSide 字段为 "BOTH"，qty>0 表示多，qty<0 表示空
		side := "NONE"
		if string(p.PositionSide) == "LONG" {
			side = "LONG"
		} else if string(p.PositionSide) == "SHORT" {
			side = "SHORT"
		} else {
			// 单向持仓模式（BOTH）
			if qty > 0 {
				side = "LONG"
			} else if qty < 0 {
				side = "SHORT"
			}
		}

		if side != "NONE" {
			pos.Side = side
			pos.Qty = absQty
			pos.EntryPrice = entry
			pos.UnrealizedPnL = pnl
			pos.Leverage = lev
			// 如果找到 LONG 仓位优先返回（避免两方都有时混乱）
			if side == "LONG" {
				break
			}
		}
	}
	return pos, nil
}

// GetMarkPrice 获取实时标记价格
func (c *Client) GetMarkPrice(symbol string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	prices, err := c.fc.NewPremiumIndexService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取标记价格失败 %s: %w", symbol, err)
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("标记价格返回空 %s", symbol)
	}
	price, err := strconv.ParseFloat(prices[0].MarkPrice, 64)
	if err != nil {
		return 0, fmt.Errorf("标记价格解析失败: %w", err)
	}
	return price, nil
}

// GetAccountInfo 获取账户信息
func (c *Client) GetAccountInfo() (*AccountInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	account, err := c.fc.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取账户信息失败: %w", err)
	}

	totalBalance, _ := strconv.ParseFloat(account.TotalWalletBalance, 64)
	available, _ := strconv.ParseFloat(account.AvailableBalance, 64)
	pnl, _ := strconv.ParseFloat(account.TotalUnrealizedProfit, 64)
	equity, _ := strconv.ParseFloat(account.TotalMarginBalance, 64)

	return &AccountInfo{
		TotalBalance:     totalBalance,
		AvailableBalance: available,
		UnrealizedPnL:    pnl,
		TotalEquity:      equity,
	}, nil
}

// SetLeverage 设置杠杆
func (c *Client) SetLeverage(symbol string, leverage int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := c.fc.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("设置杠杆失败 %s %dx: %w", symbol, leverage, err)
	}
	return nil
}

// SetMarginType 设置保证金模式为全仓（CROSSED）
func (c *Client) SetMarginType(symbol string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := c.fc.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(futures.MarginTypeCrossed).
		Do(ctx)
	if err != nil {
		// 已经是全仓模式时会报错，忽略
		return nil
	}
	return nil
}

// GetExchangeInfo 获取交易对精度信息
func (c *Client) GetExchangeInfo(symbol string) (*ExchangeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := c.fc.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取交易规则失败: %w", err)
	}

	for _, s := range info.Symbols {
		if s.Symbol != symbol {
			continue
		}
		ei := &ExchangeInfo{}
		for _, f := range s.Filters {
			switch f["filterType"] {
			case "LOT_SIZE":
				ei.StepSize, _ = strconv.ParseFloat(f["stepSize"].(string), 64)
				ei.MinQty, _ = strconv.ParseFloat(f["minQty"].(string), 64)
			case "PRICE_FILTER":
				ei.TickSize, _ = strconv.ParseFloat(f["tickSize"].(string), 64)
			case "MIN_NOTIONAL":
				ei.MinNotional, _ = strconv.ParseFloat(f["notional"].(string), 64)
			}
		}
		return ei, nil
	}
	return nil, fmt.Errorf("未找到交易对 %s", symbol)
}

// RoundToStep 将数量按精度取整
func RoundToStep(qty, step float64) float64 {
	if step == 0 {
		return qty
	}
	precision := math.Round(-math.Log10(step))
	factor := math.Pow(10, precision)
	return math.Floor(qty*factor) / factor
}

// RoundToTick 将价格按精度取整
func RoundToTick(price, tick float64) float64 {
	if tick == 0 {
		return price
	}
	precision := math.Round(-math.Log10(tick))
	factor := math.Pow(10, precision)
	return math.Round(price*factor) / factor
}

// positionSideFromSide 根据开仓方向推断 positionSide
// 开多（BUY）→ LONG；开空（SELL）→ SHORT
func positionSideFromOpenSide(side futures.SideType) futures.PositionSideType {
	if side == futures.SideTypeBuy {
		return futures.PositionSideTypeLong
	}
	return futures.PositionSideTypeShort
}

// positionSideFromCloseSide 根据平仓方向推断 positionSide
// 平多（SELL）→ LONG；平空（BUY）→ SHORT
func positionSideFromCloseSide(side futures.SideType) futures.PositionSideType {
	if side == futures.SideTypeSell {
		return futures.PositionSideTypeLong
	}
	return futures.PositionSideTypeShort
}

// PlaceMarketOrder 市价开仓（兼容双向持仓模式）
// side: BUY=做多开仓, SELL=做空开仓
func (c *Client) PlaceMarketOrder(symbol string, side futures.SideType, qty float64) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	qtyStr := strconv.FormatFloat(qty, 'f', -1, 64)
	order, err := c.fc.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(positionSideFromOpenSide(side)).
		Type(futures.OrderTypeMarket).
		Quantity(qtyStr).
		Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("市价下单失败 %s %s %.4f: %w", symbol, side, qty, err)
	}
	return order.OrderID, nil
}

// algoOrderBaseURL 根据是否测试网返回 Algo Order API 基址
func (c *Client) algoOrderBaseURL() string {
	if c.cfg.UseTestnet {
		return "https://testnet.binancefuture.com"
	}
	return "https://fapi.binance.com"
}

// signParams 对参数字符串进行 HMAC-SHA256 签名
func (c *Client) signParams(params string) string {
	mac := hmac.New(sha256.New, []byte(c.cfg.SecretKey))
	mac.Write([]byte(params))
	return hex.EncodeToString(mac.Sum(nil))
}

// algoOrderResponse Algo Order API 返回结构
type algoOrderResponse struct {
	AlgoID      int64  `json:"algoId"`
	ClientAlgoID string `json:"clientAlgoId"`
	Code        int    `json:"code"`
	Msg         string `json:"msg"`
}

// placeAlgoOrder 通过 Algo Order API 下单（/fapi/v1/algoOrder）
func (c *Client) placeAlgoOrder(params url.Values) (int64, error) {
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("algoType", "CONDITIONAL")
	rawQuery := params.Encode()
	sig := c.signParams(rawQuery)
	rawQuery += "&signature=" + sig

	baseURL := c.algoOrderBaseURL()
	reqURL := baseURL + "/fapi/v1/algoOrder"

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(rawQuery))
	if err != nil {
		return 0, fmt.Errorf("algo order 请求创建失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-MBX-APIKEY", c.cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("algo order 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result algoOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("algo order 解析失败: %s", string(body))
	}
	if result.Code != 0 {
		return 0, fmt.Errorf("algo order 错误 code=%d msg=%s", result.Code, result.Msg)
	}
	return result.AlgoID, nil
}

// PlaceStopOrder 止损单（使用 Algo Order API，兼容双向持仓模式）
// side: 平多用 SELL（positionSide=LONG），平空用 BUY（positionSide=SHORT）
func (c *Client) PlaceStopOrder(symbol string, side futures.SideType, qty, stopPrice float64) (int64, error) {
	posSide := string(positionSideFromCloseSide(side))
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", string(side))
	params.Set("positionSide", posSide)
	params.Set("type", "STOP_MARKET")
	params.Set("quantity", strconv.FormatFloat(qty, 'f', -1, 64))
	params.Set("triggerPrice", strconv.FormatFloat(stopPrice, 'f', -1, 64))

	id, err := c.placeAlgoOrder(params)
	if err != nil {
		return 0, fmt.Errorf("止损单失败 %s: %w", symbol, err)
	}
	return id, nil
}

// PlaceTakeProfitOrder 止盈单（使用 Algo Order API，兼容双向持仓模式）
func (c *Client) PlaceTakeProfitOrder(symbol string, side futures.SideType, qty, tpPrice float64) (int64, error) {
	posSide := string(positionSideFromCloseSide(side))
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", string(side))
	params.Set("positionSide", posSide)
	params.Set("type", "TAKE_PROFIT_MARKET")
	params.Set("quantity", strconv.FormatFloat(qty, 'f', -1, 64))
	params.Set("triggerPrice", strconv.FormatFloat(tpPrice, 'f', -1, 64))

	id, err := c.placeAlgoOrder(params)
	if err != nil {
		return 0, fmt.Errorf("止盈单失败 %s: %w", symbol, err)
	}
	return id, nil
}

// PlaceTrailingStopOrder 追踪止损单（使用 Algo Order API，兼容双向持仓模式）
func (c *Client) PlaceTrailingStopOrder(symbol string, side futures.SideType, qty, callbackRate float64) (int64, error) {
	posSide := string(positionSideFromCloseSide(side))
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", string(side))
	params.Set("positionSide", posSide)
	params.Set("type", "TRAILING_STOP_MARKET")
	params.Set("quantity", strconv.FormatFloat(qty, 'f', -1, 64))
	params.Set("callbackRate", strconv.FormatFloat(callbackRate, 'f', 2, 64))

	id, err := c.placeAlgoOrder(params)
	if err != nil {
		return 0, fmt.Errorf("追踪止损单失败 %s: %w", symbol, err)
	}
	return id, nil
}

// PlaceTrailingStopOrderWithActivation 带激活价格的追踪止损单（使用 Algo Order API，兼容双向持仓模式）
func (c *Client) PlaceTrailingStopOrderWithActivation(symbol string, side futures.SideType, qty, activationPrice, callbackRate float64) (int64, error) {
	posSide := string(positionSideFromCloseSide(side))
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", string(side))
	params.Set("positionSide", posSide)
	params.Set("type", "TRAILING_STOP_MARKET")
	params.Set("quantity", strconv.FormatFloat(qty, 'f', -1, 64))
	params.Set("callbackRate", strconv.FormatFloat(callbackRate, 'f', 2, 64))
	params.Set("activatePrice", strconv.FormatFloat(activationPrice, 'f', -1, 64))

	id, err := c.placeAlgoOrder(params)
	if err != nil {
		return 0, fmt.Errorf("带激活价追踪止损单失败 %s: %w", symbol, err)
	}
	return id, nil
}

// CancelAllOrders 取消所有挂单
func (c *Client) CancelAllOrders(symbol string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := c.fc.NewCancelAllOpenOrdersService().Symbol(symbol).Do(ctx)
	if err != nil {
		return fmt.Errorf("取消订单失败 %s: %w", symbol, err)
	}
	return nil
}

// ClosePosition 市价平仓（兼容双向持仓模式）
func (c *Client) ClosePosition(symbol string, pos *Position) error {
	if pos.Side == "NONE" || pos.Qty == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 平多：SELL + positionSide=LONG
	// 平空：BUY  + positionSide=SHORT
	var side futures.SideType
	var posSide futures.PositionSideType
	if pos.Side == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	qtyStr := strconv.FormatFloat(pos.Qty, 'f', -1, 64)
	_, err := c.fc.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeMarket).
		Quantity(qtyStr).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("市价平仓失败 %s %s: %w", symbol, pos.Side, err)
	}
	return nil
}

// cancelAlgoOrderRaw 取消单个 Algo Order
func (c *Client) cancelAlgoOrderRaw(algoID int64) error {
	params := url.Values{}
	params.Set("algoId", strconv.FormatInt(algoID, 10))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	rawQuery := params.Encode()
	sig := c.signParams(rawQuery)
	rawQuery += "&signature=" + sig

	baseURL := c.algoOrderBaseURL()
	reqURL := baseURL + "/fapi/v1/algoOrder?" + rawQuery

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("cancel algo order 请求创建失败: %w", err)
	}
	req.Header.Set("X-MBX-APIKEY", c.cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cancel algo order 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("cancel algo order 解析失败: %s", string(body))
	}
	if result.Code != 0 {
		return fmt.Errorf("cancel algo order 错误 code=%d msg=%s", result.Code, result.Msg)
	}
	return nil
}

// CancelAlgoOrder 取消单个追踪止损 Algo 单
func (c *Client) CancelAlgoOrder(algoID int64) error {
	if algoID == 0 {
		return nil
	}
	return c.cancelAlgoOrderRaw(algoID)
}

// CancelAllAlgoOrders 取消某币种所有 Algo 单
func (c *Client) CancelAllAlgoOrders(symbol string) error {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	rawQuery := params.Encode()
	sig := c.signParams(rawQuery)
	rawQuery += "&signature=" + sig

	baseURL := c.algoOrderBaseURL()
	reqURL := baseURL + "/fapi/v1/allAlgoOrders?" + rawQuery

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("cancel all algo orders 请求创建失败: %w", err)
	}
	req.Header.Set("X-MBX-APIKEY", c.cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cancel all algo orders 请求失败: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// GetOpenOrders 获取当前挂单
func (c *Client) GetOpenOrders(symbol string) ([]*futures.Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orders, err := c.fc.NewListOpenOrdersService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取挂单失败 %s: %w", symbol, err)
	}
	return orders, nil
}
