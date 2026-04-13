package web

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ETH 自动交易系统 V37</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: 'Segoe UI', sans-serif; background: #0d1117; color: #c9d1d9; min-height: 100vh; }
  .header { background: linear-gradient(135deg, #161b22, #21262d); padding: 14px 24px; border-bottom: 1px solid #30363d; display: flex; align-items: center; justify-content: space-between; }
  .header h1 { font-size: 18px; font-weight: 700; color: #f0f6fc; }
  .badge { font-size: 11px; padding: 3px 10px; border-radius: 20px; font-weight: 600; }
  .badge-running { background: #1a4731; color: #3fb950; border: 1px solid #238636; }
  .badge-stopped { background: #3d1a1a; color: #f85149; border: 1px solid #da3633; }
  .badge-testnet { background: #1c2a3a; color: #58a6ff; border: 1px solid #1f6feb; margin-left: 8px; }
  .container { max-width: 1400px; margin: 0 auto; padding: 16px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; margin-bottom: 16px; }
  .grid-5 { display: grid; grid-template-columns: repeat(5, 1fr); gap: 12px; margin-bottom: 16px; }
  .card { background: #161b22; border: 1px solid #30363d; border-radius: 10px; padding: 16px; }
  .card h3 { font-size: 11px; color: #8b949e; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 10px; }
  .metric { font-size: 24px; font-weight: 700; color: #f0f6fc; }
  .metric-sm { font-size: 18px; font-weight: 700; color: #f0f6fc; }
  .metric-sub { font-size: 12px; color: #8b949e; margin-top: 3px; }
  .metric.green, .metric-sm.green { color: #3fb950; }
  .metric.red, .metric-sm.red { color: #f85149; }
  .metric.yellow, .metric-sm.yellow { color: #d29922; }
  .metric.blue, .metric-sm.blue { color: #58a6ff; }
  .btn { padding: 8px 18px; border-radius: 6px; border: none; cursor: pointer; font-size: 13px; font-weight: 600; transition: all 0.2s; }
  .btn-green { background: #238636; color: #fff; }
  .btn-green:hover { background: #2ea043; }
  .btn-red { background: #da3633; color: #fff; }
  .btn-red:hover { background: #f85149; }
  .btn-orange { background: #9e6a03; color: #fff; }
  .btn-orange:hover { background: #d29922; }
  .btn-gray { background: #21262d; color: #c9d1d9; border: 1px solid #30363d; }
  .btn-gray:hover { background: #30363d; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .controls { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 16px; align-items: center; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th { background: #21262d; color: #8b949e; padding: 9px 12px; text-align: left; font-weight: 600; border-bottom: 1px solid #30363d; }
  td { padding: 9px 12px; border-bottom: 1px solid #21262d; }
  tr:hover td { background: #1c2128; }
  .side-long { color: #3fb950; font-weight: 700; }
  .side-short { color: #f85149; font-weight: 700; }
  .side-none { color: #8b949e; }
  .log-container { height: 360px; overflow-y: auto; background: #0d1117; border: 1px solid #30363d; border-radius: 6px; padding: 8px; font-family: 'Courier New', monospace; font-size: 12px; }
  .log-entry { padding: 2px 0; border-bottom: 1px solid #161b22; display: flex; gap: 8px; }
  .log-time { color: #8b949e; min-width: 75px; }
  .log-symbol { color: #58a6ff; min-width: 75px; }
  .log-msg { flex: 1; word-break: break-all; }
  .log-INFO .log-msg { color: #c9d1d9; }
  .log-WARN .log-msg { color: #d29922; }
  .log-ERROR .log-msg { color: #f85149; }
  .log-TRADE .log-msg { color: #3fb950; font-weight: 600; }
  .log-level { min-width: 48px; font-weight: 700; }
  .log-INFO .log-level { color: #58a6ff; }
  .log-WARN .log-level { color: #d29922; }
  .log-ERROR .log-level { color: #f85149; }
  .log-TRADE .log-level { color: #3fb950; }
  .tabs { display: flex; gap: 0; margin-bottom: 14px; border-bottom: 1px solid #30363d; }
  .tab { padding: 8px 16px; cursor: pointer; font-size: 13px; color: #8b949e; border-bottom: 2px solid transparent; transition: all 0.2s; }
  .tab.active { color: #f0f6fc; border-bottom-color: #58a6ff; }
  .tab-content { display: none; }
  .tab-content.active { display: block; }
  .alert { padding: 10px 14px; border-radius: 6px; margin-bottom: 14px; font-size: 13px; }
  .alert-warning { background: #2d2000; border: 1px solid #9e6a03; color: #d29922; }
  .risk-bar-bg { background: #21262d; border-radius: 4px; height: 7px; overflow: hidden; margin-top: 8px; }
  .risk-bar-fill { height: 100%; border-radius: 4px; transition: width 0.5s; }
  .risk-bar-label { font-size: 11px; color: #8b949e; margin-top: 4px; }
  .formula-box { background: #0d1117; border: 1px solid #30363d; border-radius: 6px; padding: 12px 16px; margin-bottom: 14px; font-family: 'Courier New', monospace; font-size: 12px; }
  .formula-box .title { font-size: 11px; color: #8b949e; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 8px; font-family: 'Segoe UI', sans-serif; }
  .formula-line { display: flex; justify-content: space-between; padding: 3px 0; border-bottom: 1px solid #161b22; }
  .formula-key { color: #8b949e; }
  .formula-val { color: #f0f6fc; font-weight: 600; }
  .formula-val.highlight { color: #3fb950; font-size: 14px; }
  .formula-val.warn { color: #d29922; }
  .section-title { font-size: 14px; font-weight: 700; color: #f0f6fc; margin-bottom: 12px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; margin-right: 6px; }
  .dot-green { background: #3fb950; animation: pulse 2s infinite; }
  .dot-red { background: #f85149; }
  @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
  .refresh-note { font-size: 11px; color: #8b949e; }
  .divider { border: none; border-top: 1px solid #30363d; margin: 14px 0; }
  .bt-win { color: #3fb950; font-weight: 700; }
  .bt-loss { color: #f85149; font-weight: 700; }
  .bt-trail { color: #58a6ff; }
  .bt-timeout { color: #d29922; }
  .bt-be { color: #3fb950; font-size: 11px; }
</style>
</head>
<body>

<div class="header">
  <div style="display:flex;align-items:center;gap:10px">
    <h1>⚡ ETH 自动交易系统 V37</h1>
    <span id="testnet-badge" class="badge badge-testnet">TESTNET</span>
    <span id="engine-badge" class="badge badge-stopped">已停止</span>
  </div>
  <div style="display:flex;align-items:center;gap:12px">
    <span class="refresh-note" id="last-update">--</span>
    <button class="btn btn-gray" style="font-size:12px;padding:5px 12px" onclick="refreshAll()">🔄 刷新</button>
  </div>
</div>

<div class="container">

  <div class="alert alert-warning">
    ⚠️ <strong>风险提示：</strong>杠杆交易存在爆仓风险。本系统使用 <strong>4倍杠杆</strong>，名义仓位 = 账户余额 × 4。正式使用前请先在测试网验证。
  </div>

  <!-- 控制按钮 -->
  <div class="controls">
    <button class="btn btn-green" id="btn-start" onclick="startEngine()">▶ 启动策略</button>
    <button class="btn btn-red" id="btn-stop" onclick="stopEngine()" disabled>⏹ 停止策略</button>
    <button class="btn btn-orange" onclick="confirmCloseAll()">🚨 强制平仓</button>
  </div>

  <!-- 账户概览 5格 -->
  <div class="grid-5">
    <div class="card">
      <h3>账户余额 (currEq)</h3>
      <div class="metric blue" id="curr-eq">--</div>
      <div class="metric-sub">USDT</div>
    </div>
    <div class="card">
      <h3>名义仓位上限 (×4)</h3>
      <div class="metric" id="nominal-limit">--</div>
      <div class="metric-sub">USDT = 余额 × 4</div>
    </div>
    <div class="card">
      <h3>可用余额</h3>
      <div class="metric" id="available">--</div>
      <div class="metric-sub">USDT</div>
    </div>
    <div class="card">
      <h3>未实现盈亏</h3>
      <div class="metric" id="upnl">--</div>
      <div class="metric-sub">USDT</div>
    </div>
    <div class="card">
      <h3>最大回撤</h3>
      <div class="metric" id="drawdown">--</div>
      <div class="metric-sub">历史最高: <span id="high-equity">--</span> USDT</div>
      <div class="risk-bar-bg"><div class="risk-bar-fill" id="drawdown-bar" style="width:0%;background:#3fb950"></div></div>
      <div class="risk-bar-label">阈值: 14% | <span id="can-trade-text">--</span></div>
    </div>
  </div>

  <!-- 动态仓位计算面板（核心展示区）-->
  <div class="card" style="margin-bottom:16px">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px;flex-wrap:wrap;gap:8px">
      <div class="section-title" style="margin-bottom:0">📐 当前仓位计算预览（Pine Script 原始公式）</div>
      <div id="preview-sym-tabs" style="display:flex;gap:6px;flex-wrap:wrap"></div>
    </div>
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:16px">
      <div>
        <div class="formula-box">
          <div class="title">Pine Script 公式对应</div>
          <div class="formula-line"><span class="formula-key">currEq（账户余额）</span><span class="formula-val" id="pv-curr-eq">--</span></div>
          <div class="formula-line"><span class="formula-key">ATR(14) 当前値</span><span class="formula-val" id="pv-atr">--</span></div>
          <div class="formula-line"><span class="formula-key">stopDist = ATR × 2</span><span class="formula-val" id="pv-stop-dist">--</span></div>
          <div class="formula-line"><span class="formula-key">风险金额 = currEq × 3%</span><span class="formula-val" id="pv-risk-amt">--</span></div>
          <div class="formula-line"><span class="formula-key">tradeQty = 风险金额 / stopDist</span><span class="formula-val highlight" id="pv-qty">--</span></div>
          <div class="formula-line"><span class="formula-key">名义价値 = qty × 价格</span><span class="formula-val" id="pv-nominal">--</span></div>
          <div class="formula-line"><span class="formula-key">所需保证金（÷4杠杆）</span><span class="formula-val" id="pv-margin">--</span></div>
          <div class="formula-line"><span class="formula-key">是否触及名义上限</span><span class="formula-val" id="pv-capped">--</span></div>
        </div>
      </div>
      <div>
        <div class="formula-box">
          <div class="title">当前价格下的止盈止损位</div>
          <div class="formula-line"><span class="formula-key" id="pv-sym-label">币种 当前价格</span><span class="formula-val" id="pv-price">--</span></div>
          <div class="formula-line"><span class="formula-key">做多止损（价格 - ATR×2）</span><span class="formula-val red" id="pv-sl-long">--</span></div>
          <div class="formula-line"><span class="formula-key">做多止盈（价格 + ATR×7）</span><span class="formula-val green" id="pv-tp-long">--</span></div>
          <div class="formula-line"><span class="formula-key">做空止损（价格 + ATR×2）</span><span class="formula-val red" id="pv-sl-short">--</span></div>
          <div class="formula-line"><span class="formula-key">做空止盈（价格 - ATR×7）</span><span class="formula-val green" id="pv-tp-short">--</span></div>
          <div class="formula-line"><span class="formula-key">追踪止损回调率（ATR×3/价格）</span><span class="formula-val" id="pv-trail">--</span></div>
          <div class="formula-line"><span class="formula-key">保本触发线（做多，+1.5×ATR）</span><span class="formula-val yellow" id="pv-be-long">--</span></div>
          <div class="formula-line"><span class="formula-key">保本触发线（做空，-1.5×ATR）</span><span class="formula-val yellow" id="pv-be-short">--</span></div>
        </div>
      </div>
    </div>
  </div>

  <!-- 标签页 -->
  <div class="tabs">
    <div class="tab active" onclick="switchTab('positions')">📊 持仓状态</div>
    <div class="tab" onclick="switchTab('strategy')">🎯 策略状态</div>
    <div class="tab" onclick="switchTab('symbols')">🪙 币种管理</div>
    <div class="tab" onclick="switchTab('config')">⚙️ 配置参数</div>
    <div class="tab" onclick="switchTab('logs')">📋 运行日志</div>
    <div class="tab" onclick="switchTab('backtest')">📈 回测结果</div>
  </div>

  <!-- 持仓状态 -->
  <div id="tab-positions" class="tab-content active">
    <div class="card">
      <div class="section-title"><span class="dot" id="pos-dot"></span>实时持仓（来自币安）</div>
      <div style="overflow-x:auto">
        <table>
          <thead><tr><th>交易对</th><th>方向</th><th>数量(ETH)</th><th>开仓价</th><th>名义价值</th><th>未实现盈亏</th><th>杠杆</th></tr></thead>
          <tbody id="positions-tbody"><tr><td colspan="7" style="text-align:center;color:#8b949e">加载中...</td></tr></tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- 策略状态 -->
  <div id="tab-strategy" class="tab-content">
    <div class="card">
      <div class="section-title">策略引擎内部状态</div>
      <div style="overflow-x:auto">
        <table>
          <thead><tr><th>交易对</th><th>方向</th><th>数量(ETH)</th><th>开仓价</th><th>开仓时间</th><th>持仓K线数</th><th>开仓ATR</th><th>保本状态</th></tr></thead>
          <tbody id="strategy-tbody"><tr><td colspan="8" style="text-align:center;color:#8b949e">加载中...</td></tr></tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- 币种管理 -->
  <div id="tab-symbols" class="tab-content">
    <div class="card">
      <div class="section-title">🪙 交易币种管理</div>
      <div style="margin-bottom:14px;font-size:13px;color:#8b949e">
        可动态增删交易币种，无需重启服务。添加时会自动设置杠杆和全仓模式。删除前请确保已平仓。
      </div>
      <!-- 添加币种 -->
      <div style="display:flex;gap:10px;align-items:center;margin-bottom:16px;flex-wrap:wrap">
        <input id="sym-input" type="text" placeholder="输入币种，如 BTCUSDT、SOLUSDT" style="background:#21262d;border:1px solid #30363d;border-radius:6px;padding:8px 12px;color:#f0f6fc;font-size:13px;width:220px;outline:none" />
        <button class="btn btn-green" onclick="addSymbol()">+ 添加币种</button>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          <button class="btn btn-gray" style="font-size:12px" onclick="quickAdd('BTCUSDT')">BTC</button>
          <button class="btn btn-gray" style="font-size:12px" onclick="quickAdd('ETHUSDT')">ETH</button>
          <button class="btn btn-gray" style="font-size:12px" onclick="quickAdd('SOLUSDT')">SOL</button>
          <button class="btn btn-gray" style="font-size:12px" onclick="quickAdd('BNBUSDT')">BNB</button>
          <button class="btn btn-gray" style="font-size:12px" onclick="quickAdd('DOGEUSDT')">DOGE</button>
          <button class="btn btn-gray" style="font-size:12px" onclick="quickAdd('XRPUSDT')">XRP</button>
        </div>
      </div>
      <!-- 当前币种列表 -->
      <div id="symbols-list"><div style="color:#8b949e">加载中...</div></div>
    </div>
  </div>

  <!-- 配置 -->
  <div id="tab-config" class="tab-content">
    <div class="card">
      <div class="section-title">当前运行配置</div>
      <div id="config-content" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:10px">加载中...</div>
    </div>
  </div>

  <!-- 日志 -->
  <div id="tab-logs" class="tab-content">
    <div class="card">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px">
        <div class="section-title" style="margin-bottom:0">运行日志（最近 100 条）</div>
        <button class="btn btn-gray" style="font-size:11px;padding:4px 10px" onclick="clearLogs()">清空显示</button>
      </div>
      <div class="log-container" id="log-container">加载中...</div>
    </div>
  </div>

  <!-- ===== 回测结果标签页 ===== -->
  <div id="tab-backtest" class="tab-content">
    <div class="card">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px;flex-wrap:wrap;gap:8px">
        <div class="section-title" style="margin-bottom:0">📈 <span id="bt-title">ETH/USDT</span> 1H 回测结果（V37 · v16 Broker Emulator 完全对齐）</div>
        <button class="btn btn-gray" style="font-size:12px;padding:5px 14px" onclick="runBacktest()" id="bt-run-btn">🔄 运行回测</button>
      </div>
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;flex-wrap:wrap">
        <span style="font-size:12px;color:#8b949e">交易对：</span>
        <input id="bt-sym-input" type="text" value="ETHUSDT" placeholder="如 ETHUSDT、BNBUSDT" style="background:#161b22;border:1px solid #30363d;border-radius:6px;color:#e0e0e0;font-size:13px;padding:4px 10px;width:140px;outline:none" oninput="onBtSymInput(this.value)" onkeydown="if(event.key==='Enter')runBacktest()" />
        <span style="font-size:11px;color:#555">快捷：</span>
        <button class="btn btn-gray" style="font-size:11px;padding:3px 10px" onclick="setBtSym('ETHUSDT')">ETH</button>
        <button class="btn btn-gray" style="font-size:11px;padding:3px 10px" onclick="setBtSym('BTCUSDT')">BTC</button>
        <button class="btn btn-gray" style="font-size:11px;padding:3px 10px" onclick="setBtSym('SOLUSDT')">SOL</button>
        <button class="btn btn-gray" style="font-size:11px;padding:3px 10px" onclick="setBtSym('BNBUSDT')">BNB</button>
        <button class="btn btn-gray" style="font-size:11px;padding:3px 10px" onclick="setBtSym('DOGEUSDT')">DOGE</button>
      </div>
      <div id="bt-status" style="color:#8b949e;font-size:13px;padding:6px 0">点击"运行回测"拉取最新 1500 根 K 线并计算...</div>
      <div id="bt-stats-grid" style="display:none;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:10px;margin-bottom:16px"></div>
      <div id="bt-table-wrap" style="display:none;overflow-x:auto">
        <table>
          <thead>
            <tr>
              <th>#</th><th>方向</th><th>开仓时间(UTC+8)</th><th>开仓价</th>
              <th>平仓时间(UTC+8)</th><th>平仓价</th>
              <th>盈亏USDT</th><th>平仓原因</th>
              <th>持仓K</th><th>保本</th>
            </tr>
          </thead>
          <tbody id="bt-tbody"></tbody>
        </table>
      </div>
    </div>
  </div>

</div>

<script>
async function api(path, method='GET') {
  const res = await fetch('/api' + path, { method });
  return res.json();
}

async function refreshAccount() {
  try {
    const d = await api('/account');
    const currEq = d.curr_eq || 0;
    const avail = d.available_balance || 0;
    const pnl = d.unrealized_pnl || 0;
    const dd = d.drawdown_pct || 0;
    const highEq = d.high_equity || 0;
    const nominal = d.nominal_limit || 0;

    document.getElementById('curr-eq').textContent = currEq.toFixed(2);
    document.getElementById('nominal-limit').textContent = nominal.toFixed(2);
    document.getElementById('available').textContent = avail.toFixed(2);
    document.getElementById('high-equity').textContent = highEq.toFixed(2);

    const pnlEl = document.getElementById('upnl');
    pnlEl.textContent = (pnl >= 0 ? '+' : '') + pnl.toFixed(4);
    pnlEl.className = 'metric ' + (pnl >= 0 ? 'green' : 'red');

    const ddEl = document.getElementById('drawdown');
    ddEl.textContent = dd.toFixed(2) + '%';
    ddEl.className = 'metric ' + (dd < 7 ? 'green' : dd < 12 ? 'yellow' : 'red');

    const bar = document.getElementById('drawdown-bar');
    bar.style.width = Math.min(dd / 14 * 100, 100) + '%';
    bar.style.background = dd < 7 ? '#3fb950' : dd < 12 ? '#d29922' : '#f85149';

    const ct = document.getElementById('can-trade-text');
    ct.textContent = d.can_trade ? '✅ 允许交易' : '🔒 风控锁定';
    ct.style.color = d.can_trade ? '#3fb950' : '#f85149';
  } catch(e) { console.error(e); }
}

let currentPreviewSymbol = 'ETHUSDT';

async function renderPreviewTabs() {
  try {
    const d = await fetch('/api/symbols').then(r => r.json());
    const syms = (d.symbols || []).sort();
    if (!syms.length) return;
    // 如果当前选中的币种不在列表中，切换到第一个
    if (!syms.includes(currentPreviewSymbol)) currentPreviewSymbol = syms[0];
    const container = document.getElementById('preview-sym-tabs');
    container.innerHTML = syms.map(s => {
      const active = s === currentPreviewSymbol;
      const color = s.startsWith('BTC') ? '#f7931a' : s.startsWith('ETH') ? '#627eea' : s.startsWith('SOL') ? '#9945ff' : '#58a6ff';
      return '<button onclick="switchPreviewSymbol(\'' + s + '\')" style="' +
        'background:' + (active ? color : '#21262d') + ';' +
        'color:' + (active ? '#fff' : color) + ';' +
        'border:1px solid ' + color + ';' +
        'border-radius:6px;padding:4px 12px;cursor:pointer;font-size:12px;font-weight:700">' +
        s.replace('USDT','') + '</button>';
    }).join('');
  } catch(e) { console.error(e); }
}

async function switchPreviewSymbol(sym) {
  currentPreviewSymbol = sym;
  await renderPreviewTabs();
  await refreshPreview();
}

async function refreshPreview() {
  try {
    const sym = currentPreviewSymbol || 'ETHUSDT';
    const d = await fetch('/api/position-preview?symbol=' + sym).then(r => r.json());
    if (d.error) return;

    const base = (d.symbol || sym).replace('USDT','');
    const fmt2 = v => v != null ? v.toFixed(2) : '--';
    const fmt4 = v => v != null ? v.toFixed(4) : '--';
    // 数量精度：BTC用6位，其世用4位
    const fmtQty = v => v != null ? (base === 'BTC' ? v.toFixed(6) : v.toFixed(4)) : '--';

    document.getElementById('pv-curr-eq').textContent = fmt2(d.curr_eq) + ' USDT';
    document.getElementById('pv-atr').textContent = fmt4(d.current_atr) + ' USDT';
    document.getElementById('pv-stop-dist').textContent = fmt4(d.stop_dist) + ' USDT';
    document.getElementById('pv-risk-amt').textContent = fmt2(d.risk_amount) + ' USDT';
    document.getElementById('pv-qty').textContent = fmtQty(d.trade_qty) + ' ' + base;
    document.getElementById('pv-nominal').textContent = fmt2(d.nominal_value) + ' USDT';
    document.getElementById('pv-margin').textContent = fmt2(d.margin_required) + ' USDT';

    const cappedEl = document.getElementById('pv-capped');
    cappedEl.textContent = d.capped_by_limit ? '是（已截断至名义上限）' : '否（ATR公式结果）';
    cappedEl.className = 'formula-val ' + (d.capped_by_limit ? 'warn' : 'green');

    document.getElementById('pv-sym-label').textContent = base + ' 当前价格';
    document.getElementById('pv-price').textContent = fmt2(d.current_price) + ' USDT';
    document.getElementById('pv-sl-long').textContent = fmt2(d.stop_loss_long) + ' USDT';
    document.getElementById('pv-tp-long').textContent = fmt2(d.tp_long) + ' USDT';
    document.getElementById('pv-sl-short').textContent = fmt2(d.stop_loss_short) + ' USDT';
    document.getElementById('pv-tp-short').textContent = fmt2(d.tp_short) + ' USDT';

    const trailPct = d.current_atr && d.current_price ? (d.current_atr * 3 / d.current_price * 100) : 0;
    document.getElementById('pv-trail').textContent = trailPct.toFixed(2) + '%';

    const beLong = d.current_price && d.current_atr ? (d.current_price + d.current_atr * 1.5) : 0;
    const beShort = d.current_price && d.current_atr ? (d.current_price - d.current_atr * 1.5) : 0;
    document.getElementById('pv-be-long').textContent = fmt2(beLong) + ' USDT';
    document.getElementById('pv-be-short').textContent = fmt2(beShort) + ' USDT';
  } catch(e) { console.error(e); }
}

async function refreshStatus() {
  try {
    const d = await api('/status');
    const running = d.running;
    const badge = document.getElementById('engine-badge');
    badge.textContent = running ? '运行中' : '已停止';
    badge.className = 'badge ' + (running ? 'badge-running' : 'badge-stopped');
    document.getElementById('btn-start').disabled = running;
    document.getElementById('btn-stop').disabled = !running;

    const tbody = document.getElementById('strategy-tbody');
    const states = d.states || {};
    const rows = Object.values(states);
    if (!rows.length) {
      tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:#8b949e">无数据</td></tr>';
    } else {
      tbody.innerHTML = rows.map(s => {
        const sc = s.side === 'LONG' ? 'side-long' : s.side === 'SHORT' ? 'side-short' : 'side-none';
        const et = s.side !== 'NONE' && s.entry_time ? new Date(s.entry_time).toLocaleString('zh-CN') : '--';
        const bh = s.side !== 'NONE' ? s.bars_held + ' / 100' : '--';
        return '<tr>' +
          '<td><strong>' + s.symbol + '</strong></td>' +
          '<td class="' + sc + '">' + s.side + '</td>' +
          '<td>' + (s.qty > 0 ? s.qty.toFixed(4) : '--') + '</td>' +
          '<td>' + (s.entry_price > 0 ? s.entry_price.toFixed(2) : '--') + '</td>' +
          '<td>' + et + '</td>' +
          '<td>' + bh + '</td>' +
          '<td>' + (s.entry_atr > 0 ? s.entry_atr.toFixed(4) : '--') + '</td>' +
          '<td>' + (s.breakeven_activated ? '<span style="color:#3fb950">✅ 已激活</span>' : '<span style="color:#8b949e">未激活</span>') + '</td>' +
          '</tr>';
      }).join('');
    }
    document.getElementById('last-update').textContent = '更新: ' + new Date().toLocaleTimeString('zh-CN');
  } catch(e) { console.error(e); }
}

async function refreshPositions() {
  try {
    const d = await api('/positions');
    const positions = d.positions || [];
    const tbody = document.getElementById('positions-tbody');
    const dot = document.getElementById('pos-dot');
    const hasPos = positions.some(p => p.side !== 'NONE' && p.qty > 0);
    dot.className = 'dot ' + (hasPos ? 'dot-green' : 'dot-red');

    if (!positions.length || !hasPos) {
      tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#8b949e">无持仓</td></tr>';
      return;
    }
    tbody.innerHTML = positions.filter(p => p.side !== 'NONE').map(p => {
      const sc = p.side === 'LONG' ? 'side-long' : 'side-short';
      const pc = p.unrealized_pnl >= 0 ? '#3fb950' : '#f85149';
      return '<tr>' +
        '<td><strong>' + p.symbol + '</strong></td>' +
        '<td class="' + sc + '">' + p.side + '</td>' +
        '<td>' + (p.qty > 0 ? p.qty.toFixed(4) : '--') + '</td>' +
        '<td>' + (p.entry_price > 0 ? p.entry_price.toFixed(2) : '--') + '</td>' +
        '<td>' + (p.nominal_value > 0 ? p.nominal_value.toFixed(2) + ' USDT' : '--') + '</td>' +
        '<td style="color:' + pc + '">' + (p.unrealized_pnl >= 0 ? '+' : '') + p.unrealized_pnl.toFixed(4) + '</td>' +
        '<td>' + p.leverage + 'x</td>' +
        '</tr>';
    }).join('');
  } catch(e) { console.error(e); }
}

async function refreshConfig() {
  try {
    const d = await api('/config');
    const items = [
      ['交易对', d.symbols ? d.symbols.join(', ') : '--'],
      ['杠杆倍数', d.leverage + 'x'],
      ['名义仓位倍数', d.nominal_multiplier + 'x（余额×' + d.nominal_multiplier + '）'],
      ['单笔风险', d.base_risk + '%（Pine Script baseRisk）'],
      ['最大持仓K线', d.max_hold_bars + ' 根（1H）'],
      ['最大回撤', d.max_drawdown + '%（Pine Script drawdown阈值）'],
      ['初始资金基准', d.init_capital + ' USDT'],
      ['网络模式', d.use_testnet ? '🔵 测试网' : '🔴 正式网'],
      ['K线周期', '1H'],
      ['止损距离', 'ATR × 2.0'],
      ['止盈距离', 'ATR × 7.0（stopDist×3.5）'],
      ['追踪止损', 'ATR × 3 / 价格（%）'],
    ];
    document.getElementById('config-content').innerHTML = items.map(([k, v]) =>
      '<div style="background:#21262d;border-radius:6px;padding:10px 14px">' +
      '<div style="font-size:11px;color:#8b949e;margin-bottom:4px">' + k + '</div>' +
      '<div style="font-size:13px;font-weight:700;color:#f0f6fc">' + v + '</div></div>'
    ).join('');
    const tb = document.getElementById('testnet-badge');
    if (!d.use_testnet) {
      tb.textContent = 'MAINNET';
      tb.style.cssText = 'background:#3d1a1a;color:#f85149;border-color:#da3633';
    }
  } catch(e) { console.error(e); }
}

async function refreshLogs() {
  try {
    const d = await api('/logs');
    const logs = d.logs || [];
    const c = document.getElementById('log-container');
    if (!logs.length) { c.innerHTML = '<div style="color:#8b949e;padding:8px">暂无日志</div>'; return; }
    c.innerHTML = logs.map(l => {
      const t = new Date(l.time).toLocaleTimeString('zh-CN');
      return '<div class="log-entry log-' + l.level + '">' +
        '<span class="log-time">' + t + '</span>' +
        '<span class="log-level">' + l.level + '</span>' +
        '<span class="log-symbol">' + l.symbol + '</span>' +
        '<span class="log-msg">' + l.message + '</span></div>';
    }).join('');
  } catch(e) { console.error(e); }
}

function clearLogs() {
  document.getElementById('log-container').innerHTML = '<div style="color:#8b949e;padding:8px">已清空显示</div>';
}

async function refreshAll() {
  await Promise.all([refreshAccount(), refreshStatus(), refreshPositions(), renderPreviewTabs().then(() => refreshPreview())]);
  const active = document.querySelector('.tab-content.active');
  if (active && active.id === 'tab-logs') refreshLogs();
  if (active && active.id === 'tab-config') refreshConfig();
}

async function startEngine() {
  const res = await api('/start', 'POST');
  alert(res.message);
  refreshAll();
}

async function stopEngine() {
  if (!confirm('确定要停止策略吗？（不会自动平仓）')) return;
  const res = await api('/stop', 'POST');
  alert(res.message);
  refreshAll();
}

async function refreshSymbols() {
  try {
    const d = await fetch('/api/symbols').then(r => r.json());
    const syms = d.symbols || [];
    const el = document.getElementById('symbols-list');
    if (!syms.length) {
      el.innerHTML = '<div style="color:#8b949e;padding:12px">暂无交易币种，请添加</div>';
      return;
    }
    el.innerHTML = '<div style="display:flex;flex-wrap:wrap;gap:10px">' +
      syms.sort().map(s => {
        const color = s.startsWith('BTC') ? '#f7931a' : s.startsWith('ETH') ? '#627eea' : s.startsWith('SOL') ? '#9945ff' : '#58a6ff';
        return '<div style="background:#21262d;border:1px solid #30363d;border-radius:8px;padding:10px 16px;display:flex;align-items:center;gap:12px">' +
          '<span style="font-size:15px;font-weight:700;color:' + color + '">' + s.replace('USDT','') + '</span>' +
          '<span style="font-size:11px;color:#8b949e">USDT 永续</span>' +
          '<button onclick="removeSymbol(\'' + s + '\')" style="background:#3d1a1a;border:1px solid #da3633;color:#f85149;border-radius:4px;padding:2px 8px;cursor:pointer;font-size:11px">删除</button>' +
          '</div>';
      }).join('') + '</div>';
  } catch(e) { console.error(e); }
}

async function addSymbol() {
  const input = document.getElementById('sym-input');
  const sym = input.value.trim().toUpperCase();
  if (!sym) { alert('请输入币种名称'); return; }
  if (!sym.endsWith('USDT')) { if (!confirm(sym + ' 不以 USDT 结尾，确定添加？')) return; }
  try {
    const res = await fetch('/api/symbols', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({symbol: sym})
    }).then(r => r.json());
    if (res.error) { alert('添加失败: ' + res.error); return; }
    alert('✅ ' + res.message);
    input.value = '';
    refreshSymbols();
  } catch(e) { alert('请求失败: ' + e); }
}

function quickAdd(sym) {
  document.getElementById('sym-input').value = sym;
  addSymbol();
}

async function removeSymbol(sym) {
  if (!confirm('确定删除 ' + sym + ' 吗？（有持仓时会失败）')) return;
  try {
    const res = await fetch('/api/symbols/' + sym, { method: 'DELETE' }).then(r => r.json());
    if (res.error) { alert('删除失败: ' + res.error); return; }
    alert('✅ ' + res.message);
    refreshSymbols();
  } catch(e) { alert('请求失败: ' + e); }
}

async function confirmCloseAll() {
  if (!confirm('⚠️ 确定要强制平仓所有持仓吗？')) return;
  if (!confirm('再次确认：将以市价立即平仓，确定继续？')) return;
  const res = await api('/close-all', 'POST');
  alert(res.message);
  setTimeout(refreshAll, 2000);
}

function switchTab(name) {
  const names = ['positions', 'strategy', 'symbols', 'config', 'logs', 'backtest'];
  document.querySelectorAll('.tab').forEach((t, i) => {
    t.className = 'tab' + (names[i] === name ? ' active' : '');
  });
  document.querySelectorAll('.tab-content').forEach(c => {
    c.className = 'tab-content' + (c.id === 'tab-' + name ? ' active' : '');
  });
  if (name === 'logs') refreshLogs();
  if (name === 'config') refreshConfig();
  if (name === 'symbols') refreshSymbols();
}

refreshAll();
refreshConfig(); // 初始化时立即加载配置，确保 MAINNET/TESTNET 标签正确显示
refreshSymbols(); // 初始化币种列表
renderPreviewTabs(); // 初始化预览币种切换按鈕
setInterval(refreshAll, 30000);



// 当前选中的回测交易对（完整交易对名称，如 ETHUSDT）
var currentBtSymbol = 'ETHUSDT';

// 快捷按钮点击：填入输入框并更新状态
function setBtSym(sym) {
  currentBtSymbol = sym.toUpperCase();
  var input = document.getElementById('bt-sym-input');
  if (input) input.value = currentBtSymbol;
  // 更新标题
  var title = document.getElementById('bt-title');
  var display = currentBtSymbol.replace('USDT', '/USDT');
  if (title) title.textContent = display;
  // 清空之前的回测结果
  var statsDiv = document.getElementById('bt-stats-grid');
  var tbodyDiv = document.getElementById('bt-tbody');
  var tableWrap = document.getElementById('bt-table-wrap');
  var status = document.getElementById('bt-status');
  if (statsDiv) { statsDiv.innerHTML = ''; statsDiv.style.display = 'none'; }
  if (tbodyDiv) tbodyDiv.innerHTML = '';
  if (tableWrap) tableWrap.style.display = 'none';
  if (status) status.textContent = '已切换到 ' + display + '，点击"运行回测"开始计算...';
}

// 输入框实时输入：更新 currentBtSymbol 和标题
function onBtSymInput(val) {
  currentBtSymbol = val.trim().toUpperCase();
  var title = document.getElementById('bt-title');
  var display = currentBtSymbol ? currentBtSymbol.replace('USDT', '/USDT') : 'ETH/USDT';
  if (title) title.textContent = display;
}

async function runBacktest() {
  var btn = document.getElementById('bt-run-btn');
  var status = document.getElementById('bt-status');
  var statsDiv = document.getElementById('bt-stats-grid');
  var tbodyDiv = document.getElementById('bt-tbody');
  var tableWrap = document.getElementById('bt-table-wrap');
  // currentBtSymbol 已是完整交易对，如 ETHUSDT、BNBUSDT
  var symbol = (currentBtSymbol || 'ETHUSDT').toUpperCase();
  if (!symbol.endsWith('USDT')) symbol = symbol + 'USDT';
  var displayName = symbol.replace('USDT', '/USDT');
  btn.disabled = true;
  btn.textContent = '\u23f3 ' + displayName + ' \u56de\u6d4b\u8fd0\u884c\u4e2d...';
  status.textContent = '\u6b63\u5728\u62c9\u53d6 ' + displayName + ' \u6700\u65b0 1500 \u6839 K \u7ebf\u5e76\u8fd0\u884c\u56de\u6d4b\uff0c\u8bf7\u7a0d\u5019\uff08\u7ea6 10-30 \u79d2\uff09...';
  if (statsDiv) statsDiv.innerHTML = '';
  if (tbodyDiv) tbodyDiv.innerHTML = '';
  try {
    var res = await fetch('/api/backtest?symbol=' + symbol);
    if (!res.ok) throw new Error('HTTP ' + res.status);
    var data = await res.json();
    var s = data.stats;
    status.textContent = '\u2705 ' + displayName + ' \u56de\u6d4b\u5b8c\u6210\uff01';
    var pnlColor = s.total_pnl >= 0 ? '#4caf50' : '#f44336';
    var retColor = s.return_pct >= 0 ? '#4caf50' : '#f44336';
    statsDiv.style.display = 'grid';
    statsDiv.innerHTML =
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u672c\u91d1</div><div style="font-size:22px;font-weight:bold;color:#e0e0e0">' + s.init_capital.toFixed(2) + ' USDT</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u603b\u4ea4\u6613\u6570</div><div style="font-size:22px;font-weight:bold;color:#e0e0e0">' + s.total_trades + '</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u80dc\u7387</div><div style="font-size:22px;font-weight:bold;color:#4caf50">' + s.win_rate.toFixed(1) + '%</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u80dc/\u8d1f</div><div style="font-size:22px;font-weight:bold;color:#e0e0e0">' + s.wins + ' / ' + s.losses + '</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u603b\u76c8\u4e8f</div><div style="font-size:22px;font-weight:bold;color:' + pnlColor + '">' + (s.total_pnl >= 0 ? '+' : '') + s.total_pnl.toFixed(2) + ' USDT</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u6536\u76ca\u7387</div><div style="font-size:22px;font-weight:bold;color:' + retColor + '">' + (s.return_pct >= 0 ? '+' : '') + s.return_pct.toFixed(2) + '%</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u6700\u7ec8\u6743\u76ca</div><div style="font-size:22px;font-weight:bold;color:#e0e0e0">' + s.final_equity.toFixed(2) + ' USDT</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u6700\u5927\u56de\u64a4</div><div style="font-size:22px;font-weight:bold;color:#f44336">' + (s.max_drawdown || 0).toFixed(2) + '%</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u6700\u5927\u5355\u7b14\u76c8\u5229</div><div style="font-size:16px;font-weight:bold;color:#4caf50">+' + s.max_win.toFixed(2) + '</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u6700\u5927\u5355\u7b14\u4e8f\u635f</div><div style="font-size:16px;font-weight:bold;color:#f44336">' + s.max_loss.toFixed(2) + '</div></div>' +
      '<div class="card" style="text-align:center"><div style="font-size:11px;color:#aaa">\u5e73\u5747\u6301\u4ed3K\u7ebf</div><div style="font-size:16px;font-weight:bold;color:#e0e0e0">' + s.avg_bars.toFixed(1) + '</div></div>';
    var rows = '';
    for (var i = 0; i < data.trades.length; i++) {
      var t = data.trades[i];
      var pc = t.pnl_usdt >= 0 ? '#4caf50' : '#f44336';
      var sc = t.side === 'LONG' ? '#4caf50' : '#f44336';
      var be = t.breakeven_activated ? '\u2705' : '';
      rows += '<tr>' +
        '<td style="text-align:center">' + t.no + '</td>' +
        '<td style="color:' + sc + ';font-weight:bold">' + t.side + '</td>' +
        '<td>' + t.tv_entry_time_utc8 + '</td>' +
        '<td style="text-align:right">' + t.entry_price.toFixed(2) + '</td>' +
        '<td>' + t.tv_exit_time_utc8 + '</td>' +
        '<td style="text-align:right">' + t.exit_price.toFixed(2) + '</td>' +
        '<td style="color:' + pc + ';text-align:right;font-weight:bold">' + (t.pnl_usdt >= 0 ? '+' : '') + t.pnl_usdt.toFixed(2) + '</td>' +
        '<td>' + t.exit_reason + '</td>' +
        '<td style="text-align:center">' + t.bars_held + '</td>' +
        '<td style="text-align:center">' + be + '</td>' +
        '</tr>';
    }
    if (tbodyDiv) tbodyDiv.innerHTML = rows;
    if (tableWrap) tableWrap.style.display = 'block';
  } catch(e) {
    status.textContent = '\u274c \u56de\u6d4b\u5931\u8d25: ' + e.message;
  } finally {
    btn.disabled = false;
    btn.textContent = '\ud83d\udd04 \u8fd0\u884c\u56de\u6d4b';
  }
}
</script>
</body>
</html>`
