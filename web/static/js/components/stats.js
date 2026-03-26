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

            <!-- Chart Controls -->
            <div class="card" style="padding:16px;margin-bottom:20px;">
                <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap;margin-bottom:12px;">
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
                <div id="stats-chart-wrap" style="width:100%;min-height:240px;"></div>
            </div>

            <!-- Traffic Table -->
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

        renderUPlotChart(wrap, logs);
    } catch (err) {
        wrap.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:40px;">加载图表失败</p>';
    }
}

function renderUPlotChart(container, logs) {
    // Destroy previous chart
    if (_statsChart) {
        _statsChart.destroy();
        _statsChart = null;
    }
    container.innerHTML = '';

    if (!logs || logs.length === 0) {
        container.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:40px;">暂无流量数据</p>';
        return;
    }

    // Build data arrays
    const timestamps = logs.map(l => Math.floor(new Date(l.recorded_at).getTime() / 1000));
    const trafficIn = logs.map(l => l.traffic_in || 0);
    const trafficOut = logs.map(l => l.traffic_out || 0);

    const isDark = document.documentElement.classList.contains('dark') ||
        window.matchMedia('(prefers-color-scheme: dark)').matches;

    const axisColor = isDark ? 'rgba(148,163,184,0.3)' : 'rgba(100,116,139,0.2)';
    const textColor = isDark ? '#94a3b8' : '#64748b';

    const opts = {
        width: container.clientWidth,
        height: 260,
        cursor: { drag: { x: true, y: false } },
        scales: {
            x: { time: true },
            y: { auto: true },
        },
        axes: [
            {
                stroke: textColor,
                grid: { stroke: axisColor, width: 1 },
                ticks: { stroke: axisColor, width: 1 },
                values: (u, vals) => vals.map(v => {
                    const d = new Date(v * 1000);
                    return (d.getMonth() + 1) + '/' + d.getDate() + ' ' +
                        String(d.getHours()).padStart(2, '0') + ':00';
                }),
            },
            {
                stroke: textColor,
                grid: { stroke: axisColor, width: 1 },
                ticks: { stroke: axisColor, width: 1 },
                values: (u, vals) => vals.map(v => formatBytesShort(v)),
                size: 60,
            },
        ],
        series: [
            {},
            {
                label: '入站',
                stroke: '#3b82f6',
                fill: 'rgba(59,130,246,0.12)',
                width: 2,
                paths: uPlot.paths.bars({ size: [0.6, 100] }),
            },
            {
                label: '出站',
                stroke: '#22c55e',
                fill: 'rgba(34,197,94,0.12)',
                width: 2,
                paths: uPlot.paths.bars({ size: [0.6, 100] }),
            },
        ],
    };

    const data = [timestamps, trafficIn, trafficOut];
    _statsChart = new uPlot(opts, data, container);
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
