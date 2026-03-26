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
let _ruleRouteBuilderState = {};

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
                <td><span class="badge badge-${protoClass}">${rule.protocol.toUpperCase()}</span>${renderTunnelBadges(rule)}${renderRouteBadges(rule)}</td>
                <td><strong>${rule.listen_port}</strong></td>
                <td>${escHTML(rule.target_addr)}:${rule.target_port}</td>
                <td>${speedText}</td>
                <td>${trafficCell}</td>
                <td data-latency-cell="${rule.id}">${formatLatency(rule.latency_ms)} <button class="btn btn-sm btn-secondary latency-test-btn" data-latency-btn="${rule.id}" onclick="testRuleLatency(${rule.id})" title="测延迟">测</button></td>
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
                        <span class="m-card-val"><span class="badge badge-${protoClass}" style="font-size:0.7rem;padding:2px 8px;">${rule.protocol.toUpperCase()}</span>${renderTunnelBadges(rule)}${renderRouteBadges(rule)} :${rule.listen_port}</span>
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
                        <span class="m-card-val" data-latency-cell="${rule.id}">${formatLatency(rule.latency_ms)} <button class="btn btn-sm btn-secondary latency-test-btn" data-latency-btn="${rule.id}" onclick="testRuleLatency(${rule.id})">测</button></span>
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
    Promise.all([API.getNodes()]).then(([res]) => {
        const nodes = res.nodes || [];
        if (nodes.length === 0) {
            Toast.error('请先创建并连接节点');
            return;
        }

        _managedChainNodes = nodes;
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
            ${renderRouteSettings('rule')}
            ${renderTunnelSettings('rule')}
        `, async () => {
            if (!validateRouteFormSettings('rule') || !validateTunnelFormSettings('rule')) {
                return;
            }

            const routeSettings = parseRouteSettings('rule');
            if (!routeSettings) {
                return;
            }

            const rule = {
                name: document.getElementById('rule-name').value.trim(),
                node_id: parseInt(document.getElementById('rule-node').value, 10),
                protocol: document.getElementById('rule-protocol').value,
                listen_port: parseInt(document.getElementById('rule-listen-port').value, 10),
                target_addr: document.getElementById('rule-target-addr').value.trim(),
                target_port: parseInt(document.getElementById('rule-target-port').value, 10),
                speed_limit: parseInt(document.getElementById('rule-speed').value, 10) || 0,
                traffic_limit: parseTrafficLimit(document.getElementById('rule-traffic-limit').value),
                ...routeSettings,
                ...parseTunnelSettings('rule'),
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
        seedRouteBuilderState('rule');
        syncRouteMode('rule');
        syncTunnelCompatibility('rule');
    }).catch(err => {
        Toast.error('加载节点失败: ' + err.message);
    });
}

async function showEditRuleModal(id) {
    try {
        const [res, nodesRes] = await Promise.all([API.getRule(id), API.getNodes()]);
        const rule = res.rule;
        _managedChainNodes = nodesRes.nodes || [];

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
            ${renderRouteSettings('edit-rule', rule)}
            ${renderTunnelSettings('edit-rule', rule)}
        `, async () => {
            if (!validateRouteFormSettings('edit-rule') || !validateTunnelFormSettings('edit-rule')) {
                return;
            }

            const routeSettings = parseRouteSettings('edit-rule');
            if (!routeSettings) {
                return;
            }

            const update = {
                name: document.getElementById('edit-rule-name').value.trim(),
                protocol: document.getElementById('edit-rule-protocol').value,
                listen_port: parseInt(document.getElementById('edit-rule-listen').value, 10),
                target_addr: document.getElementById('edit-rule-addr').value.trim(),
                target_port: parseInt(document.getElementById('edit-rule-port').value, 10),
                speed_limit: parseInt(document.getElementById('edit-rule-speed').value, 10) || 0,
                traffic_limit: parseTrafficLimit(document.getElementById('edit-rule-traffic-limit').value),
                ...routeSettings,
                ...parseTunnelSettings('edit-rule'),
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
        seedRouteBuilderState('edit-rule', rule);
        syncRouteMode('edit-rule');
        syncTunnelCompatibility('edit-rule');
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

// Track pending latency tests for UI feedback
const _pendingLatencyTests = new Set();

async function testRuleLatency(id) {
    if (_pendingLatencyTests.has(id)) return;
    _pendingLatencyTests.add(id);

    // Set button to loading state
    const btn = document.querySelector(`[data-latency-btn="${id}"]`);
    if (btn) {
        btn.disabled = true;
        btn.innerHTML = '<span class="latency-spinner"></span>';
    }

    try {
        await API.testLatency(id);
        // Auto-timeout after 15s if no WS response
        setTimeout(() => {
            if (_pendingLatencyTests.has(id)) {
                _pendingLatencyTests.delete(id);
                if (btn) { btn.disabled = false; btn.textContent = '测'; }
                Toast.warning('延迟测试超时');
            }
        }, 15000);
    } catch (err) {
        _pendingLatencyTests.delete(id);
        if (btn) { btn.disabled = false; btn.textContent = '测'; }
        Toast.error('测延迟失败: ' + err.message);
    }
}

// Called from app.js when WS receives latency_result
function handleLatencyResults(results) {
    if (!Array.isArray(results)) return;
    for (const r of results) {
        const ruleId = r.rule_id;
        const latencyMs = r.latency_ms;
        const wasPending = _pendingLatencyTests.has(ruleId);
        _pendingLatencyTests.delete(ruleId);

        // Show toast with result
        if (latencyMs < 0) {
            Toast.error(`规则 #${ruleId} 延迟测试失败（不可达）`);
        } else {
            const color = latencyMs < 50 ? '🟢' : latencyMs < 150 ? '🟡' : '🔴';
            Toast.success(`${color} 规则 #${ruleId} 延迟: ${latencyMs.toFixed(1)}ms`);
        }

        // Update DOM in-place: find the latency cell and animate it
        const cell = document.querySelector(`[data-latency-cell="${ruleId}"]`);
        if (cell) {
            cell.innerHTML = formatLatency(latencyMs) +
                ` <button class="btn btn-sm btn-secondary latency-test-btn" data-latency-btn="${ruleId}" onclick="testRuleLatency(${ruleId})" title="测延迟">测</button>`;
            cell.classList.add('latency-flash');
            setTimeout(() => cell.classList.remove('latency-flash'), 1500);
        } else {
            // Reset button if cell not found (maybe on different page)
            const btn = document.querySelector(`[data-latency-btn="${ruleId}"]`);
            if (btn) { btn.disabled = false; btn.textContent = '测'; }
        }
    }
}

// Cached nodes list for managed chain dropdowns
let _managedChainNodes = [];

// Phase 2 route builder: ordered hops with arbitrary host:port targets.
function renderRouteSettings(prefix, rule) {
    let routeMode = 'direct';
    if (rule) {
        if (rule.route_mode === 'hop_chain' || rule.route_mode === 'group_chain') routeMode = 'hop_chain';
        else if (rule.route_mode === 'port_mux') routeMode = 'port_mux';
    }
    const chainType = rule ? (rule.chain_type || 'custom') : 'custom';
    const sniHosts = rule ? (rule.sni_hosts || '') : '';

    // Parse SNI hosts for display
    let sniHostsText = '';
    try {
        const arr = JSON.parse(sniHosts || '[]');
        sniHostsText = Array.isArray(arr) ? arr.join('\n') : '';
    } catch (e) { sniHostsText = ''; }

    return `
        <div class="tunnel-section">
            <div class="tunnel-toggle" onclick="this.parentElement.classList.toggle('open')">
                <span>链路设置</span>
                <span class="tunnel-arrow">▾</span>
            </div>
            <div class="tunnel-body">
                <div class="form-row">
                    <div class="form-group">
                        <label>路由模式</label>
                        <select class="form-select" id="${prefix}-route-mode" onchange="syncRouteMode('${prefix}')">
                            <option value="direct" ${routeMode === 'direct' ? 'selected' : ''}>直连</option>
                            <option value="hop_chain" ${routeMode === 'hop_chain' ? 'selected' : ''}>有序跳点</option>
                            <option value="port_mux" ${routeMode === 'port_mux' ? 'selected' : ''}>端口复用 (SNI)</option>
                        </select>
                    </div>
                    <div class="form-group" id="${prefix}-chain-type-group" style="display:none;">
                        <label>链路类型</label>
                        <select class="form-select" id="${prefix}-chain-type" onchange="syncChainType('${prefix}')">
                            <option value="custom" ${chainType === 'custom' ? 'selected' : ''}>自定义链路</option>
                            <option value="managed" ${chainType === 'managed' ? 'selected' : ''}>托管链路</option>
                        </select>
                    </div>
                </div>
                <div id="${prefix}-route-hop-editor"></div>
                <div id="${prefix}-sni-editor" style="display:none;">
                    <div style="color:var(--text-muted);font-size:0.82rem;margin-bottom:6px;">
                        同一监听端口可创建多条 port_mux 规则，根据 TLS SNI 主机名分发到不同后端。使用 * 作为未匹配 SNI 的默认路由。
                    </div>
                    <div class="form-group">
                        <label>SNI 主机名（每行一个）</label>
                        <textarea class="form-input" id="${prefix}-sni-hosts" rows="3" placeholder="example.com&#10;*.example.com&#10;*">${escHTML(sniHostsText)}</textarea>
                    </div>
                </div>
                <div id="${prefix}-route-note" style="display:none;font-size:0.8rem;"></div>
            </div>
        </div>
    `;
}

function createBlankRouteHop(order) {
    return {
        order,
        lb_strategy: 'none',
        targetsText: '',
        node_id: 0,
        listen_port: 0,
    };
}

function parseRouteHopsForEditor(raw) {
    if (!raw) {
        return [];
    }

    try {
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) {
            return [];
        }
        return parsed.map((hop, index) => ({
            order: Number.isInteger(hop.order) && hop.order > 0 ? hop.order : index + 1,
            lb_strategy: hop.lb_strategy || 'none',
            targetsText: Array.isArray(hop.targets)
                ? hop.targets.map(target => `${target.host || ''}:${target.port || ''}`).join('\n')
                : '',
            node_id: hop.node_id || 0,
            listen_port: hop.listen_port || 0,
        }));
    } catch (err) {
        return [];
    }
}

function seedRouteBuilderState(prefix, rule) {
    const hops = parseRouteHopsForEditor(rule?.route_hops || '[]');
    _ruleRouteBuilderState[prefix] = hops.length > 0 ? hops : [createBlankRouteHop(1)];
}

function normalizeRouteBuilderOrders(prefix) {
    const state = _ruleRouteBuilderState[prefix] || [];
    state.forEach((hop, index) => {
        hop.order = index + 1;
    });
}

function renderRouteHopCard(prefix, hop, index, total) {
    const isManaged = getChainType(prefix) === 'managed';

    const nodeOptions = _managedChainNodes.map(n =>
        `<option value="${n.id}" ${hop.node_id === n.id ? 'selected' : ''}>${escHTML(n.name)} (${n.status === 'online' ? '在线' : '离线'})</option>`
    ).join('');

    const managedFields = isManaged ? `
            <div class="form-row">
                <div class="form-group">
                    <label>中转节点</label>
                    <select class="form-select" onchange="updateRouteHopField('${prefix}', ${index}, 'node_id', parseInt(this.value, 10))">
                        <option value="0">-- 选择节点 --</option>
                        ${nodeOptions}
                    </select>
                </div>
                <div class="form-group">
                    <label>监听端口</label>
                    <input type="number" class="form-input" value="${hop.listen_port || ''}" min="1" max="65535" placeholder="10000"
                        onchange="updateRouteHopField('${prefix}', ${index}, 'listen_port', parseInt(this.value, 10) || 0)">
                </div>
            </div>` : `
            <div class="form-group">
                <label>该跳目标（每行一个 host:port）</label>
                <textarea class="form-input" rows="4" placeholder="1.2.3.4:4000&#10;1.2.3.5:40001" onchange="updateRouteHopField('${prefix}', ${index}, 'targetsText', this.value)">${escHTML(hop.targetsText || '')}</textarea>
            </div>`;

    // Only show LB strategy for custom chains (managed chains are single-target per hop)
    const lbField = isManaged ? '' : `
            <div class="form-group">
                <label>该跳负载策略</label>
                <select class="form-select" onchange="updateRouteHopField('${prefix}', ${index}, 'lb_strategy', this.value)">
                    <option value="none" ${hop.lb_strategy === 'none' ? 'selected' : ''}>关闭</option>
                    <option value="round_robin" ${hop.lb_strategy === 'round_robin' ? 'selected' : ''}>轮询</option>
                    <option value="weighted_round_robin" ${hop.lb_strategy === 'weighted_round_robin' ? 'selected' : ''}>加权轮询</option>
                    <option value="least_connections" ${hop.lb_strategy === 'least_connections' ? 'selected' : ''}>最小连接</option>
                    <option value="least_latency" ${hop.lb_strategy === 'least_latency' ? 'selected' : ''}>最小延迟</option>
                    <option value="ip_hash" ${hop.lb_strategy === 'ip_hash' ? 'selected' : ''}>IP Hash</option>
                    <option value="failover" ${hop.lb_strategy === 'failover' ? 'selected' : ''}>主备故障转移</option>
                </select>
            </div>`;

    return `
        <div class="card" style="padding:12px;margin-top:12px;border:1px dashed rgba(148,163,184,0.35);">
            <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:10px;">
                <strong>第 ${hop.order} 跳</strong>
                <div style="display:flex;gap:6px;flex-wrap:wrap;">
                    <button type="button" class="btn btn-sm btn-secondary" onclick="moveRouteHop('${prefix}', ${index}, -1)" ${index === 0 ? 'disabled' : ''}>上移</button>
                    <button type="button" class="btn btn-sm btn-secondary" onclick="moveRouteHop('${prefix}', ${index}, 1)" ${index === total - 1 ? 'disabled' : ''}>下移</button>
                    <button type="button" class="btn btn-sm btn-danger" onclick="removeRouteHop('${prefix}', ${index})">删除</button>
                </div>
            </div>
            ${lbField}
            ${managedFields}
        </div>
    `;
}

function getChainType(prefix) {
    return document.getElementById(prefix + '-chain-type')?.value || 'custom';
}

function renderRouteHopEditor(prefix) {
    const container = document.getElementById(prefix + '-route-hop-editor');
    if (!container) {
        return;
    }

    const isManaged = getChainType(prefix) === 'managed';
    const state = _ruleRouteBuilderState[prefix] || [];
    const desc = isManaged
        ? '托管链路要求中转节点必须已在面板注册并部署 Agent（节点在线时自动同步规则）。选择中转节点和端口，系统会自动创建中转规则。'
        : '每一跳填写一组 host:port，不要求中转机部署 Agent。中间跳指向中转入口，最后一跳为落地目标。';

    container.innerHTML = `
        <div style="color:var(--text-muted);font-size:0.82rem;margin-bottom:6px;">
            ${desc}
        </div>
        ${state.map((hop, index) => renderRouteHopCard(prefix, hop, index, state.length)).join('')}
        <div style="margin-top:12px;">
            <button type="button" class="btn btn-secondary" onclick="addRouteHop('${prefix}')">+ 添加跳点</button>
        </div>
    `;
}

function syncRouteMode(prefix) {
    const mode = document.getElementById(prefix + '-route-mode')?.value || 'direct';
    const routeEditor = document.getElementById(prefix + '-route-hop-editor');
    const sniEditor = document.getElementById(prefix + '-sni-editor');
    const note = document.getElementById(prefix + '-route-note');
    const chainTypeGroup = document.getElementById(prefix + '-chain-type-group');

    const isHopChain = mode === 'hop_chain';
    const isPortMux = mode === 'port_mux';

    if (chainTypeGroup) {
        chainTypeGroup.style.display = isHopChain ? '' : 'none';
    }
    if (routeEditor) {
        routeEditor.style.display = isHopChain ? 'block' : 'none';
        if (isHopChain) {
            renderRouteHopEditor(prefix);
        }
    }
    if (sniEditor) {
        sniEditor.style.display = isPortMux ? 'block' : 'none';
    }
    if (note) {
        if (isHopChain) {
            const isManaged = getChainType(prefix) === 'managed';
            note.style.display = 'block';
            note.style.color = isManaged ? 'var(--color-success, #22c55e)' : 'var(--color-warning, #e6a23c)';
            note.textContent = isManaged
                ? '托管链路：中转节点必须部署 Agent 并在面板注册在线。系统自动创建 direct 规则并同步到节点。'
                : '自定义链路：后续跳点需手动配置转发，不要求中转机部署 Agent，无法自动验证。';
        } else if (isPortMux) {
            note.style.display = 'block';
            note.style.color = 'var(--color-info, #3b82f6)';
            note.textContent = '端口复用：同端口多条规则通过 TLS SNI 主机名区分后端。非 TLS 连接会匹配 * 默认路由。';
        } else {
            note.style.display = 'none';
        }
    }
}

function syncChainType(prefix) {
    // Re-render hop cards (managed shows node dropdown, custom shows textarea)
    renderRouteHopEditor(prefix);
    // Update note text
    syncRouteMode(prefix);
}

function addRouteHop(prefix) {
    const state = _ruleRouteBuilderState[prefix] || [];
    state.push(createBlankRouteHop(state.length + 1));
    _ruleRouteBuilderState[prefix] = state;
    normalizeRouteBuilderOrders(prefix);
    renderRouteHopEditor(prefix);
}

function moveRouteHop(prefix, index, delta) {
    const state = _ruleRouteBuilderState[prefix] || [];
    const nextIndex = index + delta;
    if (index < 0 || index >= state.length || nextIndex < 0 || nextIndex >= state.length) {
        return;
    }
    [state[index], state[nextIndex]] = [state[nextIndex], state[index]];
    normalizeRouteBuilderOrders(prefix);
    renderRouteHopEditor(prefix);
}

function removeRouteHop(prefix, index) {
    const state = _ruleRouteBuilderState[prefix] || [];
    if (state.length <= 1) {
        _ruleRouteBuilderState[prefix] = [createBlankRouteHop(1)];
    } else {
        state.splice(index, 1);
        _ruleRouteBuilderState[prefix] = state;
    }
    normalizeRouteBuilderOrders(prefix);
    renderRouteHopEditor(prefix);
}

function updateRouteHopField(prefix, index, field, value) {
    const state = _ruleRouteBuilderState[prefix] || [];
    if (!state[index]) {
        return;
    }
    state[index][field] = value;
    _ruleRouteBuilderState[prefix] = state;
}

function serializeRouteHops(prefix) {
    const isManaged = getChainType(prefix) === 'managed';
    const rawState = _ruleRouteBuilderState[prefix] || [];

    if (isManaged) {
        return serializeRouteHopsManaged(rawState);
    }
    return serializeRouteHopsCustom(rawState);
}

function serializeRouteHopsManaged(rawState) {
    const hops = [];
    for (let i = 0; i < rawState.length; i++) {
        const hop = rawState[i];
        const order = i + 1;

        if (!hop.node_id || hop.node_id <= 0) {
            return { ok: false, error: `第 ${order} 跳必须选择一个中转节点` };
        }
        if (!hop.listen_port || hop.listen_port < 1 || hop.listen_port > 65535) {
            return { ok: false, error: `第 ${order} 跳的监听端口无效` };
        }

        hops.push({
            order,
            node_id: hop.node_id,
            listen_port: hop.listen_port,
            lb_strategy: 'none',
            targets: [{ host: '0.0.0.0', port: hop.listen_port }], // placeholder, backend resolves
        });
    }

    return { ok: true, value: JSON.stringify(hops), summaryStrategy: 'none' };
}

function serializeRouteHopsCustom(rawState) {
    const state = rawState.map((hop, index) => ({
        order: index + 1,
        lb_strategy: hop.lb_strategy || 'none',
        targetsText: hop.targetsText || '',
    }));

    const hops = [];
    let summaryStrategy = 'none';

    for (const hop of state) {
        const lines = hop.targetsText.split('\n').map(line => line.trim()).filter(Boolean);
        if (lines.length === 0) {
            return { ok: false, error: `第 ${hop.order} 跳至少需要一个 host:port` };
        }

        const targets = [];
        for (const line of lines) {
            const separator = line.lastIndexOf(':');
            if (separator <= 0 || separator === line.length - 1) {
                return { ok: false, error: `第 ${hop.order} 跳目标格式错误: ${line}` };
            }

            const host = line.slice(0, separator).trim();
            const port = parseInt(line.slice(separator + 1).trim(), 10);
            if (!host || !Number.isInteger(port) || port < 1 || port > 65535) {
                return { ok: false, error: `第 ${hop.order} 跳目标格式错误: ${line}` };
            }

            targets.push({ host, port });
        }

        if (summaryStrategy === 'none' && hop.lb_strategy && hop.lb_strategy !== 'none') {
            summaryStrategy = hop.lb_strategy;
        }

        hops.push({
            order: hop.order,
            lb_strategy: hop.lb_strategy || 'none',
            targets,
        });
    }

    return {
        ok: true,
        value: JSON.stringify(hops),
        summaryStrategy,
    };
}

function parseSNIHostsField(prefix) {
    const raw = (document.getElementById(prefix + '-sni-hosts')?.value || '').trim();
    const hosts = raw.split('\n').map(h => h.trim()).filter(Boolean);
    return hosts;
}

function parseRouteSettings(prefix) {
    const routeMode = document.getElementById(prefix + '-route-mode')?.value || 'direct';

    if (routeMode === 'port_mux') {
        const hosts = parseSNIHostsField(prefix);
        return {
            route_mode: 'port_mux',
            route_hops: '[]',
            entry_group: '',
            relay_groups: '',
            exit_group: '',
            lb_strategy: 'none',
            chain_type: 'custom',
            sni_hosts: JSON.stringify(hosts),
        };
    }

    if (routeMode !== 'hop_chain') {
        return {
            route_mode: 'direct',
            route_hops: '[]',
            entry_group: '',
            relay_groups: '',
            exit_group: '',
            lb_strategy: 'none',
            chain_type: 'custom',
            sni_hosts: '[]',
        };
    }

    const serialized = serializeRouteHops(prefix);
    if (!serialized.ok) {
        return null;
    }

    return {
        route_mode: 'hop_chain',
        route_hops: serialized.value,
        entry_group: '',
        relay_groups: '',
        exit_group: '',
        lb_strategy: serialized.summaryStrategy,
        chain_type: getChainType(prefix),
        sni_hosts: '[]',
    };
}

function validateRouteFormSettings(prefix) {
    const routeMode = document.getElementById(prefix + '-route-mode')?.value || 'direct';

    if (routeMode === 'port_mux') {
        const hosts = parseSNIHostsField(prefix);
        if (hosts.length === 0) {
            Toast.error('端口复用模式必须至少填写一个 SNI 主机名');
            return false;
        }
        return true;
    }

    if (routeMode !== 'hop_chain') {
        return true;
    }

    const serialized = serializeRouteHops(prefix);
    if (!serialized.ok) {
        Toast.error(serialized.error);
        return false;
    }
    return true;
}

function renderRouteBadges(rule) {
    if (!rule || !rule.route_mode || rule.route_mode === 'direct') {
        return '';
    }

    if (rule.route_mode === 'port_mux') {
        let sniCount = 0;
        try { sniCount = JSON.parse(rule.sni_hosts || '[]').length; } catch (e) {}
        let badges = '<span class="tunnel-badge" style="background:rgba(99,102,241,0.16);color:#6366f1;" title="端口复用 (SNI)">SNI Mux</span>';
        if (sniCount > 0) {
            badges += `<span class="tunnel-badge" style="background:rgba(15,118,110,0.14);color:#0f766e;" title="SNI 主机数: ${sniCount}">${sniCount}H</span>`;
        }
        return badges;
    }

    const hops = parseRouteHopsForEditor(rule.route_hops || '[]');
    const isManaged = rule.chain_type === 'managed';

    let badges = isManaged
        ? '<span class="tunnel-badge" style="background:rgba(34,197,94,0.16);color:#16a34a;" title="托管链路">MChain</span>'
        : '<span class="tunnel-badge" style="background:rgba(245,158,11,0.16);color:#b45309;" title="自定义链路">Chain</span>';

    if (hops.length > 0) {
        badges += `<span class="tunnel-badge" style="background:rgba(15,118,110,0.14);color:#0f766e;" title="跳点数量: ${hops.length}">${hops.length}H</span>`;
    }
    if (hops.some(hop => hop.lb_strategy && hop.lb_strategy !== 'none')) {
        badges += '<span class="tunnel-badge" style="background:rgba(14,165,233,0.14);color:#0369a1;" title="至少一跳启用了负载策略">LB</span>';
    }
    return badges;
}

// --- Tunnel Settings UI Helpers ---

function renderTunnelSettings(prefix, rule) {
    const pp = rule ? rule.proxy_protocol || 0 : 0;
    const bp = rule ? (rule.blocked_protos || '') : '';
    const ps = rule ? rule.pool_size || 0 : 0;
    const tm = rule ? (rule.tls_mode || 'none') : 'none';
    const ts = rule ? (rule.tls_sni || '') : '';
    const we = rule ? (rule.ws_enabled || false) : false;
    const wp = rule ? (rule.ws_path || '/ws') : '/ws';

    const hasSocks = bp.includes('socks');
    const hasHttp = bp.includes('http');
    const hasTls = bp.includes('tls');

    return `
        <div class="tunnel-section">
            <div class="tunnel-toggle" onclick="this.parentElement.classList.toggle('open')">
                <span>隧道设置</span>
                <span class="tunnel-arrow">▸</span>
            </div>
            <div class="tunnel-body">
                <div class="form-row">
                    <div class="form-group">
                        <label>PROXY Protocol</label>
                        <select class="form-select" id="${prefix}-proxy-protocol">
                            <option value="0" ${pp===0?'selected':''}>关闭</option>
                            <option value="1" ${pp===1?'selected':''}>v1 (文本)</option>
                            <option value="2" ${pp===2?'selected':''}>v2 (二进制)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>连接池大小 (0=关闭)</label>
                        <input type="number" class="form-input" id="${prefix}-pool-size" value="${ps}" min="0" max="100">
                    </div>
                </div>
                <div class="form-group">
                    <label>协议拦截</label>
                    <div style="display:flex;gap:12px;margin-top:4px;">
                        <label style="display:flex;align-items:center;gap:4px;font-size:0.85rem;cursor:pointer;">
                            <input type="checkbox" id="${prefix}-block-socks" ${hasSocks?'checked':''}> SOCKS
                        </label>
                        <label style="display:flex;align-items:center;gap:4px;font-size:0.85rem;cursor:pointer;">
                            <input type="checkbox" id="${prefix}-block-http" ${hasHttp?'checked':''}> HTTP
                        </label>
                        <label style="display:flex;align-items:center;gap:4px;font-size:0.85rem;cursor:pointer;">
                            <input type="checkbox" id="${prefix}-block-tls" ${hasTls?'checked':''}> TLS
                        </label>
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>TLS 模式</label>
                        <select class="form-select" id="${prefix}-tls-mode" onchange="toggleTlsSni('${prefix}')">
                            <option value="none" ${tm==='none'?'selected':''}>关闭</option>
                            <option value="client" ${tm==='client'?'selected':''}>客户端 (入站TLS)</option>
                            <option value="server" ${tm==='server'?'selected':''}>服务端 (出站TLS)</option>
                            <option value="both" ${tm==='both'?'selected':''}>双向 TLS</option>
                        </select>
                    </div>
                    <div class="form-group" id="${prefix}-tls-sni-group" style="display:${(tm==='server'||tm==='both')?'block':'none'}">
                        <label>TLS SNI</label>
                        <input type="text" class="form-input" id="${prefix}-tls-sni" value="${escHTML(ts)}" placeholder="example.com">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label style="display:flex;align-items:center;gap:6px;">
                            <input type="checkbox" id="${prefix}-ws-enabled" ${we?'checked':''} onchange="toggleWsPath('${prefix}')"> WebSocket 隧道
                        </label>
                    </div>
                    <div class="form-group" id="${prefix}-ws-path-group" style="display:${we?'block':'none'}">
                        <label>WS 路径</label>
                        <input type="text" class="form-input" id="${prefix}-ws-path" value="${escHTML(wp)}" placeholder="/ws">
                    </div>
                </div>
                <div id="${prefix}-ws-tls-note" style="display:none;color:var(--color-warning, #e6a23c);font-size:0.8rem;">
                    WebSocket 隧道暂不支持与入站 TLS 同时开启。
                </div>
            </div>
        </div>
    `;
}

function toggleTlsSni(prefix) {
    const mode = document.getElementById(prefix + '-tls-mode').value;
    const group = document.getElementById(prefix + '-tls-sni-group');
    if (group) group.style.display = (mode === 'server' || mode === 'both') ? 'block' : 'none';
    syncTunnelCompatibility(prefix);
}

function toggleWsPath(prefix) {
    const enabled = document.getElementById(prefix + '-ws-enabled').checked;
    const group = document.getElementById(prefix + '-ws-path-group');
    if (group) group.style.display = enabled ? 'block' : 'none';
    syncTunnelCompatibility(prefix);
}

function syncTunnelCompatibility(prefix) {
    const wsEnabled = document.getElementById(prefix + '-ws-enabled')?.checked || false;
    const tlsSelect = document.getElementById(prefix + '-tls-mode');
    const note = document.getElementById(prefix + '-ws-tls-note');

    if (!tlsSelect) return;

    const clientOption = Array.from(tlsSelect.options).find(opt => opt.value === 'client');
    const bothOption = Array.from(tlsSelect.options).find(opt => opt.value === 'both');
    if (clientOption) clientOption.disabled = wsEnabled;
    if (bothOption) bothOption.disabled = wsEnabled;

    if (wsEnabled && (tlsSelect.value === 'client' || tlsSelect.value === 'both')) {
        tlsSelect.value = 'none';
        const sniGroup = document.getElementById(prefix + '-tls-sni-group');
        if (sniGroup) sniGroup.style.display = 'none';
    }

    if (note) note.style.display = wsEnabled ? 'block' : 'none';
}

function validateTunnelFormSettings(prefix) {
    const settings = parseTunnelSettings(prefix);
    if (settings.ws_enabled && (settings.tls_mode === 'client' || settings.tls_mode === 'both')) {
        Toast.error('WebSocket 隧道暂不支持与入站 TLS 同时开启');
        return false;
    }
    return true;
}

function parseTunnelSettings(prefix) {
    const blocked = [];
    if (document.getElementById(prefix + '-block-socks')?.checked) blocked.push('socks');
    if (document.getElementById(prefix + '-block-http')?.checked) blocked.push('http');
    if (document.getElementById(prefix + '-block-tls')?.checked) blocked.push('tls');

    return {
        proxy_protocol: parseInt(document.getElementById(prefix + '-proxy-protocol')?.value || '0', 10),
        blocked_protos: blocked.join(','),
        pool_size: parseInt(document.getElementById(prefix + '-pool-size')?.value || '0', 10),
        tls_mode: document.getElementById(prefix + '-tls-mode')?.value || 'none',
        tls_sni: document.getElementById(prefix + '-tls-sni')?.value?.trim() || '',
        ws_enabled: document.getElementById(prefix + '-ws-enabled')?.checked || false,
        ws_path: document.getElementById(prefix + '-ws-path')?.value?.trim() || '/ws',
    };
}

function renderTunnelBadges(rule) {
    let badges = '';
    if (rule.tls_mode && rule.tls_mode !== 'none') {
        badges += '<span class="tunnel-badge tunnel-badge-tls" title="TLS: ' + escHTML(rule.tls_mode) + '">TLS</span>';
    }
    if (rule.ws_enabled) {
        badges += '<span class="tunnel-badge tunnel-badge-ws" title="WebSocket 隧道">WS</span>';
    }
    if (rule.blocked_protos) {
        badges += '<span class="tunnel-badge tunnel-badge-block" title="拦截: ' + escHTML(rule.blocked_protos) + '">Block</span>';
    }
    if (rule.proxy_protocol > 0) {
        badges += '<span class="tunnel-badge tunnel-badge-pp" title="PROXY Protocol v' + rule.proxy_protocol + '">PP</span>';
    }
    if (rule.pool_size > 0) {
        badges += '<span class="tunnel-badge tunnel-badge-pool" title="连接池: ' + rule.pool_size + '">Pool</span>';
    }
    return badges;
}
