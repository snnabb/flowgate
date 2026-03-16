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

            <div style="display:grid; grid-template-columns: 1fr 1fr; gap:16px;">
                <div class="table-container">
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

                <div class="table-container">
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
            </div>
        </div>
    `;

    loadDashboardData();
}

async function loadDashboardData() {
    try {
        const [dashRes, nodesRes, rulesRes] = await Promise.all([
            API.getDashboard(),
            API.getNodes(),
            API.getRules()
        ]);

        const s = dashRes.stats;
        document.getElementById('s-nodes').textContent = s.total_nodes;
        document.getElementById('s-online').textContent = s.online_nodes;
        document.getElementById('s-rules').textContent = `${s.active_rules}/${s.total_rules}`;
        document.getElementById('s-traffic').textContent = formatBytes(s.total_traffic_in + s.total_traffic_out);

        // Nodes quick view
        const nodesBody = document.getElementById('dash-nodes-body');
        const nodes = nodesRes.nodes || [];
        if (nodes.length === 0) {
            nodesBody.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无节点</p></td></tr>';
        } else {
            nodesBody.innerHTML = nodes.slice(0, 5).map(n => `
                <tr>
                    <td>${escHTML(n.name)}</td>
                    <td><span class="badge badge-${n.status}"><span class="badge-dot"></span>${n.status === 'online' ? '在线' : '离线'}</span></td>
                    <td>${n.cpu_usage.toFixed(1)}%</td>
                    <td>${n.mem_usage.toFixed(1)} MB</td>
                </tr>
            `).join('');
        }

        // Rules quick view
        const rulesBody = document.getElementById('dash-rules-body');
        const rules = rulesRes.rules || [];
        if (rules.length === 0) {
            rulesBody.innerHTML = '<tr><td colspan="4" class="empty-state"><p>暂无规则</p></td></tr>';
        } else {
            rulesBody.innerHTML = rules.slice(0, 5).map(r => {
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
        }
    } catch (err) {
        Toast.error('加载仪表盘数据失败: ' + err.message);
    }
}

function escHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}
