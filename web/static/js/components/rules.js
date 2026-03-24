// Rules Component
let rulesRefreshTimer = null;

function renderRules() {
    const content = document.getElementById('page-content');
    const params = new URLSearchParams(window.location.search);
    const nodeFilter = params.get('node_id') || '';

    content.innerHTML = `
        <div class="fade-in">
            <div class="page-header">
                <div>
                    <h2>转发规则</h2>
                    <p class="subtitle">管理端口转发规则与节点执行状态</p>
                </div>
                <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
                    <input type="text" class="form-input" id="rule-search" placeholder="搜索规则..." style="width:140px;padding:6px 10px;font-size:0.82rem;" oninput="filterRulesBySearch()">
                    <select class="form-select" id="rule-node-filter" style="width:140px;flex-shrink:0;" onchange="filterRulesByNode()">
                        <option value="">全部节点</option>
                    </select>
                    <button class="btn btn-primary" onclick="showCreateRuleModal()">+ 添加</button>
                </div>
            </div>

            <div class="table-container desktop-only">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>名称</th>
                            <th>节点</th>
                            <th>协议</th>
                            <th>监听端口</th>
                            <th>目标地址</th>
                            <th>限速</th>
                            <th>流量</th>
                            <th>延迟</th>
                            <th>状态</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody id="rules-body">
                        <tr><td colspan="11" class="empty-state"><p>加载中...</p></td></tr>
                    </tbody>
                </table>
            </div>
            <div class="mobile-only m-card-list" id="rules-cards">
                <p style="color:var(--text-muted);text-align:center;padding:20px;">加载中...</p>
            </div>
        </div>
    `;

    loadNodeOptions(nodeFilter);
    loadRules(nodeFilter ? parseInt(nodeFilter, 10) : 0);
    startRulesAutoRefresh();
}

async function loadNodeOptions(selectedId) {
    try {
        const res = await API.getNodes();
        const select = document.getElementById('rule-node-filter');
        (res.nodes || []).forEach(node => {
            const option = document.createElement('option');
            option.value = node.id;
            option.textContent = node.name;
            if (String(node.id) === selectedId) {
                option.selected = true;
            }
            select.appendChild(option);
        });
    } catch (err) {
        Toast.error('加载节点列表失败: ' + err.message);
    }
}

function filterRulesByNode() {
    const nodeId = document.getElementById('rule-node-filter').value;
    const url = nodeId ? `/rules?node_id=${nodeId}` : '/rules';
    window.history.replaceState({}, '', url);
    loadRules(nodeId ? parseInt(nodeId, 10) : 0);
}

let _nodesCache = {};

function getRuleRuntimeMeta(rule) {
    switch (rule.runtime_status) {
        case 'running':
            return { badge: 'running', label: '运行中', note: rule.runtime_message || '节点已确认规则生效' };
        case 'error':
            return { badge: 'error', label: '失败', note: rule.runtime_message || '节点启动规则失败' };
        case 'offline':
            return { badge: 'offline', label: '待同步', note: rule.runtime_message || '节点离线，等待重连' };
        case 'stopped':
            return { badge: 'stopped', label: '已停用', note: rule.runtime_message || '规则当前未运行' };
        default:
            return { badge: 'pending', label: '待确认', note: rule.runtime_message || '规则已下发，等待节点确认' };
    }
}

function renderRuleStatus(rule) {
    const runtime = getRuleRuntimeMeta(rule);
    return `
        <div class="rule-status-cell">
            <span class="badge badge-${runtime.badge}"><span class="badge-dot"></span>${runtime.label}</span>
            <label class="toggle" title="${rule.enabled ? '启用' : '禁用'}">
                <input type="checkbox" ${rule.enabled ? 'checked' : ''} onchange="toggleRuleEnabled(${rule.id})">
                <span class="toggle-slider"></span>
            </label>
        </div>
        <div class="rule-status-note">${escHTML(runtime.note)}</div>
    `;
}

async function loadRules(nodeId, silent) {
    try {
        const [rulesRes, nodesRes] = await Promise.all([
            API.getRules(nodeId || ''),
            API.getNodes()
        ]);

        _nodesCache = {};
        (nodesRes.nodes || []).forEach(node => {
            _nodesCache[node.id] = node.name;
        });

        // Store rules globally for search filtering
        window._allRules = rulesRes.rules || [];
        renderFilteredRules(window._allRules);
    } catch (err) {
        if (!silent) {
            Toast.error('加载规则失败: ' + err.message);
        }
    }
}

function filterRulesBySearch() {
    const q = (document.getElementById('rule-search')?.value || '').toLowerCase();
    if (!window._allRules) return;
    if (!q) {
        renderFilteredRules(window._allRules);
        return;
    }
    const filtered = window._allRules.filter(r => {
        const name = (r.name || '').toLowerCase();
        const target = (r.target_addr + ':' + r.target_port).toLowerCase();
        const node = (_nodesCache[r.node_id] || '').toLowerCase();
        return name.includes(q) || target.includes(q) || node.includes(q) || String(r.listen_port).includes(q);
    });
    renderFilteredRules(filtered);
}

function renderFilteredRules(rules) {
    const body = document.getElementById('rules-body');
    const cards = document.getElementById('rules-cards');

    if (rules.length === 0) {
        if (body) body.innerHTML = '<tr><td colspan="11" class="empty-state"><p>暂无转发规则</p></td></tr>';
        if (cards) cards.innerHTML = '<p style="color:var(--text-muted);text-align:center;padding:20px;">暂无转发规则</p>';
        return;
    }

    if (body) body.innerHTML = rules.map(rule => {
        const protoClass = rule.protocol === 'tcp' ? 'tcp' : rule.protocol === 'udp' ? 'udp' : 'both';
        const speedText = rule.speed_limit > 0 ? `${rule.speed_limit} KB/s` : '无限';
        const totalTraffic = rule.traffic_in + rule.traffic_out;
        const trafficCell = rule.traffic_limit > 0
            ? formatTrafficWithLimit(totalTraffic, rule.traffic_limit)
            : `${formatBytes(rule.traffic_in)} / ${formatBytes(rule.traffic_out)}`;
        return `
            <tr>
                <td>#${rule.id}</td>
                <td>${escHTML(rule.name || `规则 #${rule.id}`)}</td>
                <td>${escHTML(_nodesCache[rule.node_id] || `#${rule.node_id}`)}</td>
                <td><span class="badge badge-${protoClass}">${rule.protocol.toUpperCase()}</span></td>
                <td><strong>${rule.listen_port}</strong></td>
                <td>${escHTML(rule.target_addr)}:${rule.target_port}</td>
                <td>${speedText}</td>
                <td>${trafficCell}</td>
                <td>${formatLatency(rule.latency_ms)}</td>
                <td>${renderRuleStatus(rule)}</td>
                <td>
                    <div class="action-group">
                        <button class="btn btn-sm btn-secondary" onclick="showEditRuleModal(${rule.id})" title="编辑">✎</button>
                        <button class="btn btn-sm btn-secondary" onclick="confirmResetTraffic(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')" title="重置流量">↺</button>
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteRule(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')" title="删除">🗑</button>
                    </div>
                </td>
            </tr>
        `;
    }).join('');

    if (cards) cards.innerHTML = rules.map(rule => {
        const protoClass = rule.protocol === 'tcp' ? 'tcp' : rule.protocol === 'udp' ? 'udp' : 'both';
        const runtime = getRuleRuntimeMeta(rule);
        const totalTraffic = rule.traffic_in + rule.traffic_out;
        const trafficDisplay = rule.traffic_limit > 0
            ? formatTrafficWithLimit(totalTraffic, rule.traffic_limit)
            : `<span style="color:var(--color-info);">↓${formatBytes(rule.traffic_in)}</span> <span style="color:var(--color-success);">↑${formatBytes(rule.traffic_out)}</span>`;
        return `
            <div class="m-card">
                <div class="m-card-head">
                    <span class="m-card-title">${escHTML(rule.name || `规则 #${rule.id}`)}</span>
                    <span class="m-card-id">#${rule.id}</span>
                </div>
                <div class="m-card-body">
                    <div class="m-card-row">
                        <span class="m-card-label">节点</span>
                        <span class="m-card-val">${escHTML(_nodesCache[rule.node_id] || `#${rule.node_id}`)}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">协议 / 端口</span>
                        <span class="m-card-val"><span class="badge badge-${protoClass}" style="font-size:0.7rem;padding:2px 8px;">${rule.protocol.toUpperCase()}</span> :${rule.listen_port}</span>
                    </div>
                    <div class="m-card-row full">
                        <span class="m-card-label">目标</span>
                        <span class="m-card-val">${escHTML(rule.target_addr)}:${rule.target_port}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">流量</span>
                        <span class="m-card-val">${trafficDisplay}</span>
                    </div>
                    <div class="m-card-row">
                        <span class="m-card-label">延迟</span>
                        <span class="m-card-val">${formatLatency(rule.latency_ms)}</span>
                    </div>
                </div>
                <div class="m-card-foot">
                    <div class="m-card-status">
                        <span class="badge badge-${runtime.badge}"><span class="badge-dot"></span>${runtime.label}</span>
                        <label class="toggle" title="${rule.enabled ? '启用' : '禁用'}">
                            <input type="checkbox" ${rule.enabled ? 'checked' : ''} onchange="toggleRuleEnabled(${rule.id})">
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="action-group" style="display:flex;gap:6px;">
                        <button class="btn btn-sm btn-secondary" onclick="showEditRuleModal(${rule.id})">编辑</button>
                        <button class="btn btn-sm btn-secondary" onclick="confirmResetTraffic(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')">重置</button>
                        <button class="btn btn-sm btn-danger" onclick="confirmDeleteRule(${rule.id}, '${escHTML(rule.name || `规则 #${rule.id}`)}')">删除</button>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

function startRulesAutoRefresh() {
    if (rulesRefreshTimer) {
        clearInterval(rulesRefreshTimer);
    }

    rulesRefreshTimer = setInterval(() => {
        if (Router.currentPath !== '/rules') {
            clearInterval(rulesRefreshTimer);
            rulesRefreshTimer = null;
            return;
        }

        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId, 10) : 0, true);
    }, 5000);
}

async function toggleRuleEnabled(id) {
    try {
        const res = await API.toggleRule(id);
        Toast.success(res.enabled ? '规则已启用' : '规则已禁用');
        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId, 10) : 0);
    } catch (err) {
        Toast.error('操作失败: ' + err.message);
        const nodeId = document.getElementById('rule-node-filter')?.value || '';
        loadRules(nodeId ? parseInt(nodeId, 10) : 0);
    }
}

function showCreateRuleModal() {
    API.getNodes().then(res => {
        const nodes = res.nodes || [];
        if (nodes.length === 0) {
            Toast.error('请先创建并连接节点');
            return;
        }

        const nodeOptions = nodes.map(node => `<option value="${node.id}">${escHTML(node.name)}</option>`).join('');

        showModal('添加转发规则', `
            <div class="form-group">
                <label>规则名称</label>
                <input type="text" class="form-input" id="rule-name" placeholder="例如: Web 转发" autofocus>
            </div>
            <div class="form-group">
                <label>节点</label>
                <select class="form-select" id="rule-node">${nodeOptions}</select>
            </div>
            <div class="form-group">
                <label>协议</label>
                <select class="form-select" id="rule-protocol">
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                    <option value="tcp+udp">TCP+UDP</option>
                </select>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>监听端口</label>
                    <input type="number" class="form-input" id="rule-listen-port" placeholder="10000" min="1" max="65535">
                </div>
                <div class="form-group">
                    <label>限速 (KB/s, 0=无限)</label>
                    <input type="number" class="form-input" id="rule-speed" placeholder="0" min="0" value="0">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>目标地址</label>
                    <input type="text" class="form-input" id="rule-target-addr" placeholder="1.2.3.4">
                </div>
                <div class="form-group">
                    <label>目标端口</label>
                    <input type="number" class="form-input" id="rule-target-port" placeholder="443" min="1" max="65535">
                </div>
            </div>
            <div class="form-group">
                <label>流量限额 (0=无限, 例如: 100GB, 500MB)</label>
                <input type="text" class="form-input" id="rule-traffic-limit" placeholder="例如: 100GB 或 0" value="0">
            </div>
        `, async () => {
            const rule = {
                name: document.getElementById('rule-name').value.trim(),
                node_id: parseInt(document.getElementById('rule-node').value, 10),
                protocol: document.getElementById('rule-protocol').value,
                listen_port: parseInt(document.getElementById('rule-listen-port').value, 10),
                target_addr: document.getElementById('rule-target-addr').value.trim(),
                target_port: parseInt(document.getElementById('rule-target-port').value, 10),
                speed_limit: parseInt(document.getElementById('rule-speed').value, 10) || 0,
                traffic_limit: parseTrafficLimit(document.getElementById('rule-traffic-limit').value),
            };

            if (!rule.listen_port || !rule.target_addr || !rule.target_port) {
                Toast.error('请填写完整的端口和目标地址');
                return;
            }

            try {
                await API.createRule(rule);
                closeModal();
                Toast.success('规则已创建，等待节点确认');
                const nodeId = document.getElementById('rule-node-filter')?.value || '';
                loadRules(nodeId ? parseInt(nodeId, 10) : 0);
            } catch (err) {
                Toast.error('创建失败: ' + err.message);
            }
        });
    }).catch(err => {
        Toast.error('加载节点失败: ' + err.message);
    });
}

async function showEditRuleModal(id) {
    try {
        const res = await API.getRule(id);
        const rule = res.rule;

        showModal('编辑转发规则', `
            <div class="form-group">
                <label>规则名称</label>
                <input type="text" class="form-input" id="edit-rule-name" value="${escHTML(rule.name)}">
            </div>
            <div class="form-group">
                <label>协议</label>
                <select class="form-select" id="edit-rule-protocol">
                    <option value="tcp" ${rule.protocol === 'tcp' ? 'selected' : ''}>TCP</option>
                    <option value="udp" ${rule.protocol === 'udp' ? 'selected' : ''}>UDP</option>
                    <option value="tcp+udp" ${rule.protocol === 'tcp+udp' ? 'selected' : ''}>TCP+UDP</option>
                </select>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>监听端口</label>
                    <input type="number" class="form-input" id="edit-rule-listen" value="${rule.listen_port}">
                </div>
                <div class="form-group">
                    <label>限速 (KB/s, 0=无限)</label>
                    <input type="number" class="form-input" id="edit-rule-speed" value="${rule.speed_limit}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>目标地址</label>
                    <input type="text" class="form-input" id="edit-rule-addr" value="${escHTML(rule.target_addr)}">
                </div>
                <div class="form-group">
                    <label>目标端口</label>
                    <input type="number" class="form-input" id="edit-rule-port" value="${rule.target_port}">
                </div>
            </div>
            <div class="form-group">
                <label>流量限额 (0=无限, 例如: 100GB, 500MB)</label>
                <input type="text" class="form-input" id="edit-rule-traffic-limit" value="${rule.traffic_limit > 0 ? formatTrafficLimitInput(rule.traffic_limit) : '0'}">
            </div>
        `, async () => {
            const update = {
                name: document.getElementById('edit-rule-name').value.trim(),
                protocol: document.getElementById('edit-rule-protocol').value,
                listen_port: parseInt(document.getElementById('edit-rule-listen').value, 10),
                target_addr: document.getElementById('edit-rule-addr').value.trim(),
                target_port: parseInt(document.getElementById('edit-rule-port').value, 10),
                speed_limit: parseInt(document.getElementById('edit-rule-speed').value, 10) || 0,
                traffic_limit: parseTrafficLimit(document.getElementById('edit-rule-traffic-limit').value),
            };

            try {
                await API.updateRule(id, update);
                closeModal();
                Toast.success('规则已更新，等待节点确认');
                const nodeId = document.getElementById('rule-node-filter')?.value || '';
                loadRules(nodeId ? parseInt(nodeId, 10) : 0);
            } catch (err) {
                Toast.error('更新失败: ' + err.message);
            }
        });
    } catch (err) {
        Toast.error('加载规则失败: ' + err.message);
    }
}

async function confirmDeleteRule(id, name) {
    showModal('删除规则', `
        <p style="color:var(--color-danger);">确定要删除 <strong>${name}</strong> 吗？</p>
    `, async () => {
        try {
            await API.deleteRule(id);
            closeModal();
            Toast.success('规则已删除');
            const nodeId = document.getElementById('rule-node-filter')?.value || '';
            loadRules(nodeId ? parseInt(nodeId, 10) : 0);
        } catch (err) {
            Toast.error('删除失败: ' + err.message);
        }
    }, '取消', '确认删除');
}

async function confirmResetTraffic(id, name) {
    showModal('重置流量', `
        <p>确定要重置 <strong>${name}</strong> 的流量计数器吗？</p>
        <p style="color:var(--text-muted);font-size:0.85rem;margin-top:8px;">此操作将清零入站和出站流量统计。</p>
    `, async () => {
        try {
            await API.resetTraffic(id);
            closeModal();
            Toast.success('流量已重置');
            const nodeId = document.getElementById('rule-node-filter')?.value || '';
            loadRules(nodeId ? parseInt(nodeId, 10) : 0);
        } catch (err) {
            Toast.error('重置失败: ' + err.message);
        }
    }, '取消', '确认重置');
}
