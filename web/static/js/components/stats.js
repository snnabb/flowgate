// Stats Component - simple traffic overview
function renderStats() {
    const content = document.getElementById('page-content');
    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>流量统计</h2>
                    <p class="subtitle">查看各规则的流量使用情况</p>
                </div>
            </div>

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

    loadTrafficStats();
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
        const body = document.getElementById('traffic-body');

        const cards = document.getElementById('traffic-cards');

        if (rules.length === 0) {
            if (body) body.innerHTML = '<tr><td colspan="7" class="empty-state"><p>暂无数据</p></td></tr>';
            if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无数据</p>';
            return;
        }

        // Sort by total traffic descending
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
    } catch (err) {
        Toast.error('加载流量统计失败: ' + err.message);
    }
}
