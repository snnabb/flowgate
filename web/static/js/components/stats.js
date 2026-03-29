// Stats Component - traffic charts + table overview
let _statsChart = null;

function renderStats() {
    const content = document.getElementById('page-content');
    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>流量统计</h2>
                    <p class="subtitle">查看流量趋势和使用情况</p>
                </div>
            </div>

            <!-- 图表控制 -->
            <div class="card" style="padding:16px;margin-bottom:20px;">
                <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap;margin-bottom:16px;">
                    <select class="form-select" id="stats-rule-select" onchange="loadTrafficChart()" style="max-width:260px;">
                        <option value="aggregate">全部规则（汇总）</option>
                    </select>
                    <div style="display:flex;gap:6px;">
                        <button class="btn btn-sm btn-secondary stats-hours-btn" data-hours="24" onclick="setStatsHours(24)">24h</button>
                        <button class="btn btn-sm btn-secondary stats-hours-btn" data-hours="48" onclick="setStatsHours(48)">48h</button>
                        <button class="btn btn-sm btn-secondary stats-hours-btn active" data-hours="168" onclick="setStatsHours(168)">7d</button>
                        <button class="btn btn-sm btn-secondary stats-hours-btn" data-hours="720" onclick="setStatsHours(720)">30d</button>
                    </div>
                </div>
                <div id="stats-summary" class="stats-summary" style="display:none;"></div>
                <div id="stats-chart-wrap" style="width:100%;min-height:300px;"></div>
            </div>

            <!-- 流量表格 -->
            <div class="table-container desktop-only">
                <table>
                    <thead>
                        <tr>
                            <th>规则</th>
                            <th>节点</th>
                            <th>协议</th>
                            <th>监听端口</th>
                            <th>入站流量</th>
                            <th>出站流量</th>
                            <th>总流量</th>
                        </tr>
                    </thead>
                    <tbody id="traffic-body">
                        <tr><td colspan="7" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only m-card-list" id="traffic-cards">
                <p style="color:var(--text-muted);text-align:center;padding:20px;">加载中...</p>
            </div>
        </div>
    `;

    _statsSelectedHours = 168;
    loadTrafficStats();
}

let _statsSelectedHours = 168;

function setStatsHours(h) {
    _statsSelectedHours = h;
    document.querySelectorAll('.stats-hours-btn').forEach(btn => {
        btn.classList.toggle('active', parseInt(btn.dataset.hours, 10) === h);
    });
    loadTrafficChart();
}

async function loadTrafficStats() {
    try {
        const [rulesRes, nodesRes] = await Promise.all([
            API.getRules(),
            API.getNodes()
        ]);

        const nodesMap = {};
        (nodesRes.nodes || []).forEach(n => { nodesMap[n.id] = n.name; });

        const rules = rulesRes.rules || [];

        // Populate rule dropdown
        const sel = document.getElementById('stats-rule-select');
        if (sel) {
            rules.forEach(r => {
                const opt = document.createElement('option');
                opt.value = r.id;
                opt.textContent = (r.name || '规则 #' + r.id) + ' (:' + r.listen_port + ')';
                sel.appendChild(opt);
            });
        }

        // Render table
        const body = document.getElementById('traffic-body');
        const cards = document.getElementById('traffic-cards');

        if (rules.length === 0) {
            if (body) body.innerHTML = '<tr><td colspan="7" class="empty-state"><p>暂无数据</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无数据</p>';
        } else {
            rules.sort((a, b) => (b.traffic_in + b.traffic_out) - (a.traffic_in + a.traffic_out));

            if (body) body.innerHTML = rules.map(r => {
                const total = r.traffic_in + r.traffic_out;
                const protoClass = r.protocol === 'tcp' ? 'tcp' : r.protocol === 'udp' ? 'udp' : 'both';
                return `
                    <tr>
                        <td><strong>${escHTML(r.name || '规则 #' + r.id)}</strong></td>
                        <td>${escHTML(nodesMap[r.node_id] || '#' + r.node_id)}</td>
                        <td><span class="badge badge-${protoClass}">${r.protocol.toUpperCase()}</span></td>
                        <td>${r.listen_port}</td>
                        <td style="color:var(--color-info)">${formatBytes(r.traffic_in)}</td>
                        <td style="color:var(--color-success)">${formatBytes(r.traffic_out)}</td>
                        <td><strong>${formatBytes(total)}</strong></td>
                    </tr>
                `;
            }).join('');

            if (cards) cards.innerHTML = rules.map(r => {
                const total = r.traffic_in + r.traffic_out;
                const protoClass = r.protocol === 'tcp' ? 'tcp' : r.protocol === 'udp' ? 'udp' : 'both';
                return `
                    <div class="m-card">
                        <div class="m-card-head">
                            <span class="m-card-title">${escHTML(r.name || '规则 #' + r.id)}</span>
                            <span class="badge badge-${protoClass}" style="font-size:0.7rem;padding:2px 8px;">${r.protocol.toUpperCase()}</span>
                        </div>
                        <div class="m-card-body cols-3">
                            <div class="m-card-row">
                                <span class="m-card-label">节点</span>
                                <span class="m-card-val">${escHTML(nodesMap[r.node_id] || '#' + r.node_id)}</span>
                            </div>
                            <div class="m-card-row">
                                <span class="m-card-label">端口</span>
                                <span class="m-card-val">${r.listen_port}</span>
                            </div>
                            <div class="m-card-row">
                                <span class="m-card-label">总流量</span>
                                <span class="m-card-val"><strong>${formatBytes(total)}</strong></span>
                            </div>
                        </div>
                        <div style="display:flex;justify-content:space-between;margin-top:8px;padding-top:8px;border-top:1px solid var(--border-color);font-size:0.8rem;">
                            <span style="color:var(--color-info);">↓ ${formatBytes(r.traffic_in)}</span>
                            <span style="color:var(--color-success);">↑ ${formatBytes(r.traffic_out)}</span>
                        </div>
                    </div>
                `;
            }).join('');
        }

        // Load chart
        loadTrafficChart();
    } catch (err) {
        Toast.error('加载流量统计失败: ' + err.message);
    }
}

async function loadTrafficChart() {
    const wrap = document.getElementById('stats-chart-wrap');
    if (!wrap) return;

    const ruleSelect = document.getElementById('stats-rule-select');
    const ruleVal = ruleSelect ? ruleSelect.value : 'aggregate';
    const hours = _statsSelectedHours;

    try {
        let logs;
        if (ruleVal === 'aggregate') {
            const res = await API.getAggregateTraffic(hours);
            logs = res.logs || [];
        } else {
            const res = await API.getTraffic(parseInt(ruleVal, 10), hours);
            logs = res.logs || [];
        }

        renderStatsSummary(logs);
        renderUPlotChart(wrap, logs);
    } catch (err) {
        wrap.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:40px;">加载图表失败</p>';
    }
}

function renderStatsSummary(logs) {
    const el = document.getElementById('stats-summary');
    if (!el) return;

    if (!logs || logs.length === 0) {
        el.style.display = 'none';
        return;
    }

    let totalIn = 0, totalOut = 0, peakIn = 0, peakOut = 0;
    for (const l of logs) {
        const vi = l.traffic_in || 0;
        const vo = l.traffic_out || 0;
        totalIn += vi;
        totalOut += vo;
        if (vi > peakIn) peakIn = vi;
        if (vo > peakOut) peakOut = vo;
    }

    el.style.display = 'grid';
    el.innerHTML = `
        <div class="stats-summary-item">
            <div class="stats-val" style="color:var(--color-info);">${formatBytes(totalIn)}</div>
            <div class="stats-label">总入站</div>
        </div>
        <div class="stats-summary-item">
            <div class="stats-val" style="color:var(--color-success);">${formatBytes(totalOut)}</div>
            <div class="stats-label">总出站</div>
        </div>
        <div class="stats-summary-item">
            <div class="stats-val" style="color:var(--text-primary);">${formatBytes(totalIn + totalOut)}</div>
            <div class="stats-label">合计</div>
        </div>
        <div class="stats-summary-item">
            <div class="stats-val" style="color:var(--color-warning);">${formatBytes(peakIn > peakOut ? peakIn : peakOut)}</div>
            <div class="stats-label">峰值/小时</div>
        </div>
    `;
}

function renderUPlotChart(container, logs) {
    if (_statsChart) {
        _statsChart.destroy();
        _statsChart = null;
    }
    container.innerHTML = '';

    if (!logs || logs.length === 0) {
        container.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:40px;">暂无流量数据</p>';
        return;
    }

    const timestamps = logs.map(l => Math.floor(new Date(l.recorded_at).getTime() / 1000));
    const trafficIn = logs.map(l => l.traffic_in || 0);
    const trafficOut = logs.map(l => l.traffic_out || 0);

    const isDark = document.documentElement.classList.contains('dark') ||
        window.matchMedia('(prefers-color-scheme: dark)').matches;

    const axisColor = isDark ? 'rgba(148,163,184,0.18)' : 'rgba(100,116,139,0.15)';
    const textColor = isDark ? '#94a3b8' : '#64748b';

    // Gradient fill factory using uPlot bbox (with fallback for initial render)
    function gradientFill(r, g, b) {
        return (u) => {
            const dpr = devicePixelRatio || 1;
            const h = u.bbox.height / dpr;
            const top = u.bbox.top / dpr;
            if (!h || !isFinite(h) || !isFinite(top)) {
                return `rgba(${r},${g},${b},0.15)`;
            }
            const grad = u.ctx.createLinearGradient(0, top, 0, top + h);
            grad.addColorStop(0, `rgba(${r},${g},${b},0.3)`);
            grad.addColorStop(1, `rgba(${r},${g},${b},0.01)`);
            return grad;
        };
    }

    // Custom tooltip plugin
    function tooltipPlugin() {
        let tooltip;

        function init(u) {
            tooltip = document.createElement('div');
            tooltip.style.cssText = 'display:none;position:absolute;pointer-events:none;z-index:100;' +
                'background:var(--bg-card);border:1px solid var(--border-color);border-radius:8px;' +
                'padding:10px 14px;font-size:0.8rem;box-shadow:var(--shadow-md);line-height:1.6;' +
                'color:var(--text-primary);white-space:nowrap;';
            u.over.appendChild(tooltip);
        }

        function setCursor(u) {
            const idx = u.cursor.idx;
            if (idx == null) {
                tooltip.style.display = 'none';
                return;
            }

            const ts = u.data[0][idx];
            const vIn = u.data[1][idx];
            const vOut = u.data[2][idx];
            const d = new Date(ts * 1000);
            const timeStr = `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:00`;

            tooltip.innerHTML =
                `<div style="font-weight:600;margin-bottom:4px;color:var(--text-secondary);">${timeStr}</div>` +
                `<div><span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#3b82f6;margin-right:6px;"></span>入站: <strong>${formatBytes(vIn)}</strong></div>` +
                `<div><span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#22c55e;margin-right:6px;"></span>出站: <strong>${formatBytes(vOut)}</strong></div>` +
                `<div style="margin-top:4px;padding-top:4px;border-top:1px solid var(--border-color);color:var(--text-muted);">合计: ${formatBytes(vIn + vOut)}</div>`;

            tooltip.style.display = 'block';

            const left = u.valToPos(ts, 'x');
            const chartW = u.over.clientWidth;
            const tipW = tooltip.offsetWidth;
            const xPos = left + tipW + 20 > chartW ? left - tipW - 10 : left + 10;
            tooltip.style.left = xPos + 'px';
            tooltip.style.top = '10px';
        }

        return { hooks: { init, setCursor } };
    }

    const opts = {
        width: container.clientWidth,
        height: 320,
        plugins: [tooltipPlugin()],
        cursor: {
            drag: { x: true, y: false },
            points: { size: 8, fill: (u, si) => si === 1 ? '#3b82f6' : '#22c55e' },
        },
        scales: {
            x: { time: true },
            y: { auto: true, range: (u, min, max) => [0, max * 1.1] },
        },
        axes: [
            {
                stroke: textColor,
                font: '11px Inter, sans-serif',
                grid: { stroke: axisColor, width: 1, dash: [4, 4] },
                ticks: { show: false },
                gap: 8,
                values: (u, vals) => vals.map(v => {
                    const d = new Date(v * 1000);
                    return (d.getMonth() + 1) + '/' + d.getDate() + ' ' +
                        String(d.getHours()).padStart(2, '0') + ':00';
                }),
            },
            {
                stroke: textColor,
                font: '11px Inter, sans-serif',
                grid: { stroke: axisColor, width: 1, dash: [4, 4] },
                ticks: { show: false },
                gap: 8,
                values: (u, vals) => vals.map(v => formatBytesShort(v)),
                size: 55,
            },
        ],
        legend: { show: true },
        series: [
            { label: '时间' },
            {
                label: '入站',
                stroke: '#3b82f6',
                width: 2,
                fill: gradientFill(59, 130, 246),
                paths: uPlot.paths.spline(),
            },
            {
                label: '出站',
                stroke: '#22c55e',
                width: 2,
                fill: gradientFill(34, 197, 94),
                paths: uPlot.paths.spline(),
            },
        ],
    };

    const data = [timestamps, trafficIn, trafficOut];
    _statsChart = new uPlot(opts, data, container);

    // Responsive resize
    if (!window._statsResizeObserver) {
        window._statsResizeObserver = new ResizeObserver(() => {
            if (_statsChart && container.clientWidth > 0) {
                _statsChart.setSize({ width: container.clientWidth, height: 320 });
            }
        });
    }
    window._statsResizeObserver.disconnect();
    window._statsResizeObserver.observe(container);
}

function formatBytesShort(bytes) {
    if (bytes === 0 || bytes == null) return '0';
    const units = ['B', 'K', 'M', 'G', 'T'];
    let i = 0;
    let val = Math.abs(bytes);
    while (val >= 1024 && i < units.length - 1) {
        val /= 1024;
        i++;
    }
    return (bytes < 0 ? '-' : '') + val.toFixed(i === 0 ? 0 : 1) + units[i];
}
