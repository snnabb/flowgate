// Dashboard Component
function renderDashboard() {
    const content = document.getElementById('page-content');
    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>仪表盘</h2>
                    <p class="subtitle">系统总览</p>
                </div>
            </div>

            <div class="stats-grid" id="stats-grid">
                <div class="stat-card">
                    <div class="stat-icon blue">🖥</div>
                    <div class="stat-value" id="s-nodes">-</div>
                    <div class="stat-label">节点总数</div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon green">✓</div>
                    <div class="stat-value" id="s-online">-</div>
                    <div class="stat-label">在线节点</div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon purple">⇄</div>
                    <div class="stat-value" id="s-rules">-</div>
                    <div class="stat-label">转发规则</div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon yellow">📊</div>
                    <div class="stat-value" id="s-traffic">-</div>
                    <div class="stat-label">总流量</div>
                </div>
            </div>

            <div class="dash-grid" style="display:grid; grid-template-columns: 1fr 1fr; gap:16px;">
                <div class="table-container desktop-only">
                    <div class="table-header">
                        <h3>节点状态</h3>
                    </div>
                    <table>
                        <thead>
                            <tr>
                                <th>节点</th>
                                <th>状态</th>
                                <th>CPU</th>
                                <th>内存</th>
                            </tr>
                        </thead>
                        <tbody id="dash-nodes-body">
                            <tr><td colspan="4" class="empty-state"><p>加载中...</p></td></tr>
                        </tbody>
                    </table>
                </div>
                <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                    <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">节点状态</h3>
                    <div class="m-card-list" id="dash-nodes-cards">
                        <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                    </div>
                </div>

                <div class="table-container desktop-only">
                    <div class="table-header">
                        <h3>最近规则</h3>
                    </div>
                    <table>
                        <thead>
                            <tr>
                                <th>名称</th>
                                <th>协议</th>
                                <th>端口</th>
                                <th>流量</th>
                            </tr>
                        </thead>
                        <tbody id="dash-rules-body">
                            <tr><td colspan="4" class="empty-state"><p>加载中...</p></td></tr>
                        </tbody>
                    </table>
                </div>
                <div class="mobile-only" style="background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                    <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">最近规则</h3>
                    <div class="m-card-list" id="dash-rules-cards">
                        <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                    </div>
                </div>
            </div>

            <div class="table-container desktop-only" style="margin-top:16px;">
                <div class="table-header">
                    <h3>最近事件</h3>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>类别</th>
                            <th>事件</th>
                            <th>详情</th>
                            <th>时间</th>
                        </tr>
                    </thead>
                    <tbody id="dash-events-body">
                        <tr><td colspan="4" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only" style="margin-top:12px;background:var(--bg-card);border:1px solid var(--border-color);border-radius:var(--radius-md);padding:14px;">
                <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">最近事件</h3>
                <div class="m-card-list" id="dash-events-cards">
                    <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                </div>
            </div>
        </div>
    `;

    loadDashboardData();
}

async function loadDashboardData() {
    try {
        const [dashRes, nodesRes, rulesRes, eventsRes] = await Promise.all([
            API.getDashboard(),
            API.getNodes(),
            API.getRules(),
            API.getEvents(8)
        ]);

        const s = dashRes.stats;
        document.getElementById('s-nodes').textContent = s.total_nodes;
        document.getElementById('s-online').textContent = s.online_nodes;
        document.getElementById('s-rules').textContent = `${s.active_rules}/${s.total_rules}`;
        document.getElementById('s-traffic').textContent = formatBytes(s.total_traffic_in + s.total_traffic_out);

        // Nodes quick view
        const nodesBody = document.getElementById('dash-nodes-body');
        const nodesCards = document.getElementById('dash-nodes-cards');
        const nodes = nodesRes.nodes || [];
        if (nodes.length === 0) {
            if (nodesBody) nodesBody.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无节点</p></td></tr>';
            if (nodesCards) nodesCards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无节点</p>';
        } else {
            const topNodes = nodes.slice(0, 5);
            if (nodesBody) nodesBody.innerHTML = topNodes.map(n => `
                <tr>
                    <td>${escHTML(n.name)}</td>
                    <td><span class="badge badge-${n.status}"><span class="badge-dot"></span>${n.status === 'online' ? '在线' : '离线'}</span></td>
                    <td>${n.cpu_usage.toFixed(1)}%</td>
                    <td>${formatNodeMemory(n)}</td>
                </tr>
            `).join('');
            if (nodesCards) nodesCards.innerHTML = topNodes.map(n => `
                <div class="m-card" style="padding:10px;">
                    <div style="display:flex;align-items:center;justify-content:space-between;">
                        <strong style="font-size:0.88rem;">${escHTML(n.name)}</strong>
                        <span class="badge badge-${n.status}"><span class="badge-dot"></span>${n.status === 'online' ? '在线' : '离线'}</span>
                    </div>
                    <div style="display:flex;gap:16px;margin-top:6px;font-size:0.78rem;color:var(--text-secondary);">
                        <span>CPU ${n.cpu_usage.toFixed(1)}%</span>
                        <span>内存 ${formatNodeMemory(n)}</span>
                    </div>
                </div>
            `).join('');
        }

        // Rules quick view
        const rulesBody = document.getElementById('dash-rules-body');
        const rulesCards = document.getElementById('dash-rules-cards');
        const rules = rulesRes.rules || [];
        if (rules.length === 0) {
            if (rulesBody) rulesBody.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无规则</p></td></tr>';
            if (rulesCards) rulesCards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无规则</p>';
        } else {
            const topRules = rules.slice(0, 5);
            if (rulesBody) rulesBody.innerHTML = topRules.map(r => {
                const protoClass = r.protocol === 'tcp' ? 'tcp' : r.protocol === 'udp' ? 'udp' : 'both';
                return `
                    <tr>
                        <td>${escHTML(r.name || `规则 #${r.id}`)}</td>
                        <td><span class="badge badge-${protoClass}">${r.protocol.toUpperCase()}</span></td>
                        <td>${r.listen_port}</td>
                        <td>${formatBytes(r.traffic_in + r.traffic_out)}</td>
                    </tr>
                `;
            }).join('');
            if (rulesCards) rulesCards.innerHTML = topRules.map(r => {
                const protoClass = r.protocol === 'tcp' ? 'tcp' : r.protocol === 'udp' ? 'udp' : 'both';
                return `
                <div class="m-card" style="padding:10px;">
                    <div style="display:flex;align-items:center;justify-content:space-between;">
                        <strong style="font-size:0.88rem;">${escHTML(r.name || `规则 #${r.id}`)}</strong>
                        <span class="badge badge-${protoClass}">${r.protocol.toUpperCase()}</span>
                    </div>
                    <div style="display:flex;gap:16px;margin-top:6px;font-size:0.78rem;color:var(--text-secondary);">
                        <span>:${r.listen_port}</span>
                        <span>流量 ${formatBytes(r.traffic_in + r.traffic_out)}</span>
                    </div>
                </div>
                `;
            }).join('');
        }

        // Events
        const eventsBody = document.getElementById('dash-events-body');
        const eventsCards = document.getElementById('dash-events-cards');
        const events = eventsRes.events || [];
        if (events.length === 0) {
            if (eventsBody) eventsBody.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无事件</p></td></tr>';
            if (eventsCards) eventsCards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无事件</p>';
        } else {
            if (eventsBody) eventsBody.innerHTML = events.map(event => `
                <tr>
                    <td><span class="badge badge-${getEventBadgeClass(event.category)}">${escHTML(event.category)}</span></td>
                    <td>${escHTML(event.title)}</td>
                    <td>${escHTML(event.details || '-')}</td>
                    <td>${new Date(event.created_at).toLocaleString()}</td>
                </tr>
            `).join('');
            if (eventsCards) eventsCards.innerHTML = events.map(event => `
                <div class="m-event">
                    <div class="m-event-head">
                        <span class="m-event-title">
                            <span class="badge badge-${getEventBadgeClass(event.category)}" style="font-size:0.68rem;padding:2px 6px;margin-right:6px;">${escHTML(event.category)}</span>
                            ${escHTML(event.title)}
                        </span>
                    </div>
                    <div class="m-event-time">${new Date(event.created_at).toLocaleString()}</div>
                    ${event.details ? `<div class="m-event-detail">${escHTML(event.details)}</div>` : ''}
                </div>
            `).join('');
        }
    } catch (err) {
        Toast.error('加载仪表盘数据失败: ' + err.message);
    }
}

function getEventBadgeClass(category) {
    if (category === 'node') return 'online';
    if (category === 'rule') return 'both';
    return 'pending';
}

function escHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}
