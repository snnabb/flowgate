function renderDashboard() {
    const content = document.getElementById('page-content');
    const currentUser = API.getUser();
    const isAdmin = currentUser && isAdminRole(currentUser.role);

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>仪表盘</h2>
                    <p class="subtitle">${isAdmin ? '查看全局节点、规则与流量概况' : '查看你的节点授权、规则和剩余流量'}</p>
                </div>
            </div>

            <div class="stats-grid" id="stats-grid">
                <div class="stat-card">
                    <div class="stat-value" id="s-first">-</div>
                    <div class="stat-label" id="s-first-label">${isAdmin ? '节点总数' : '已授权节点'}</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value" id="s-second">-</div>
                    <div class="stat-label" id="s-second-label">${isAdmin ? '在线节点' : '启用规则'}</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value" id="s-third">-</div>
                    <div class="stat-label" id="s-third-label">${isAdmin ? '规则总览' : '剩余流量'}</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value" id="s-fourth">-</div>
                    <div class="stat-label" id="s-fourth-label">${isAdmin ? '总流量' : '我的流量'}</div>
                </div>
            </div>

            <div class="dash-grid" style="display:grid;grid-template-columns:1fr 1fr;gap:16px;">
                <div class="table-container desktop-only">
                    <div class="table-header">
                        <h3>${isAdmin ? '节点概览' : '我的节点'}</h3>
                    </div>
                    <table>
                        <thead>
                            <tr>
                                <th>名称</th>
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
                    <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">${isAdmin ? '节点概览' : '我的节点'}</h3>
                    <div class="m-card-list" id="dash-nodes-cards">
                        <p style="color:var(--text-muted);font-size:0.85rem;">加载中...</p>
                    </div>
                </div>

                <div class="table-container desktop-only">
                    <div class="table-header">
                        <h3>${isAdmin ? '最近规则' : '我的规则'}</h3>
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
                    <h3 style="font-size:0.95rem;font-weight:600;margin-bottom:10px;">${isAdmin ? '最近规则' : '我的规则'}</h3>
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
                            <th>分类</th>
                            <th>标题</th>
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
        const currentUser = API.getUser();
        const isAdmin = currentUser && isAdminRole(currentUser.role);
        const [dashRes, nodesRes, rulesRes, eventsRes] = await Promise.all([
            API.getDashboard(),
            API.getNodes(),
            API.getRules(),
            API.getEvents(8),
        ]);

        const stats = dashRes.stats || {};
        document.getElementById('s-first').textContent = isAdmin ? (stats.total_nodes ?? 0) : (stats.assigned_nodes ?? stats.total_nodes ?? 0);
        document.getElementById('s-second').textContent = isAdmin ? (stats.online_nodes ?? 0) : `${stats.active_rules ?? 0}/${stats.total_rules ?? 0}`;
        document.getElementById('s-third').textContent = isAdmin
            ? `${stats.active_rules ?? 0}/${stats.total_rules ?? 0}`
            : formatBytes(stats.remaining_traffic || 0);
        document.getElementById('s-fourth').textContent = formatBytes((stats.total_traffic_in || 0) + (stats.total_traffic_out || 0));

        renderDashboardNodes(nodesRes.nodes || []);
        renderDashboardRules(rulesRes.rules || []);
        renderDashboardEvents(eventsRes.events || []);
    } catch (error) {
        Toast.error(`加载仪表盘失败：${error.message}`);
    }
}

function renderDashboardNodes(nodes) {
    const body = document.getElementById('dash-nodes-body');
    const cards = document.getElementById('dash-nodes-cards');
    if (!nodes.length) {
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无节点</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无节点</p>';
        return;
    }

    const topNodes = nodes.slice(0, 5);
    if (body) {
        body.innerHTML = topNodes.map((node) => `
            <tr>
                <td>${escHTML(node.name)}</td>
                <td><span class="badge badge-${node.status}"><span class="badge-dot"></span>${localizeNodeStatus(node.status)}</span></td>
                <td>${Number(node.cpu_usage || 0).toFixed(1)}%</td>
                <td>${formatNodeMemory(node)}</td>
            </tr>
        `).join('');
    }
    if (cards) {
        cards.innerHTML = topNodes.map((node) => `
            <div class="m-card" style="padding:10px;">
                <div style="display:flex;align-items:center;justify-content:space-between;">
                    <strong style="font-size:0.88rem;">${escHTML(node.name)}</strong>
                    <span class="badge badge-${node.status}"><span class="badge-dot"></span>${localizeNodeStatus(node.status)}</span>
                </div>
                <div style="display:flex;gap:16px;margin-top:6px;font-size:0.78rem;color:var(--text-secondary);">
                    <span>CPU ${Number(node.cpu_usage || 0).toFixed(1)}%</span>
                    <span>内存 ${formatNodeMemory(node)}</span>
                </div>
            </div>
        `).join('');
    }
}

function renderDashboardRules(rules) {
    const body = document.getElementById('dash-rules-body');
    const cards = document.getElementById('dash-rules-cards');
    if (!rules.length) {
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无规则</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无规则</p>';
        return;
    }

    const topRules = rules.slice(0, 5);
    if (body) {
        body.innerHTML = topRules.map((rule) => `
            <tr>
                <td>${escHTML(rule.name || `规则 #${rule.id}`)}</td>
                <td>${escHTML(rule.protocol.toUpperCase())}</td>
                <td>${rule.listen_port}</td>
                <td>${formatBytes((rule.traffic_in || 0) + (rule.traffic_out || 0))}</td>
            </tr>
        `).join('');
    }
    if (cards) {
        cards.innerHTML = topRules.map((rule) => `
            <div class="m-card" style="padding:10px;">
                <div style="display:flex;align-items:center;justify-content:space-between;">
                    <strong style="font-size:0.88rem;">${escHTML(rule.name || `规则 #${rule.id}`)}</strong>
                    <span class="badge badge-both">${escHTML(rule.protocol.toUpperCase())}</span>
                </div>
                <div style="display:flex;gap:16px;margin-top:6px;font-size:0.78rem;color:var(--text-secondary);">
                    <span>:${rule.listen_port}</span>
                    <span>${formatBytes((rule.traffic_in || 0) + (rule.traffic_out || 0))}</span>
                </div>
            </div>
        `).join('');
    }
}

function renderDashboardEvents(events) {
    const body = document.getElementById('dash-events-body');
    const cards = document.getElementById('dash-events-cards');
    if (!events.length) {
        if (body) body.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无事件</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem;">暂无事件</p>';
        return;
    }

    if (body) {
        body.innerHTML = events.map((event) => `
            <tr>
                <td>${escHTML(localizeEventCategory(event.category))}</td>
                <td>${escHTML(event.title)}</td>
                <td>${escHTML(event.details || '-')}</td>
                <td>${new Date(event.created_at).toLocaleString()}</td>
            </tr>
        `).join('');
    }
    if (cards) {
        cards.innerHTML = events.map((event) => `
            <div class="m-event">
                <div class="m-event-head">
                    <span class="m-event-title">${escHTML(event.title)}</span>
                </div>
                <div class="m-event-time">${new Date(event.created_at).toLocaleString()}</div>
                <div class="m-event-detail">${escHTML(event.details || localizeEventCategory(event.category))}</div>
            </div>
        `).join('');
    }
}
