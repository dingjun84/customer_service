'use strict';

/* ==========================================================================
   电商智能客服控制台 — 纯前端，无构建设计
   所有状态来自 HTTP API；页面不做任何业务判断，只负责呈现与交互。
   ========================================================================== */

/* ------------------------------------------------------------------ 基础工具 */

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

function h(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (value === null || value === undefined || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'html') node.innerHTML = value;
    else if (key === 'dataset') Object.assign(node.dataset, value);
    else if (key.startsWith('on') && typeof value === 'function') {
      node.addEventListener(key.slice(2).toLowerCase(), value);
    } else if (key === 'disabled') node.disabled = Boolean(value);
    else node.setAttribute(key, value);
  }
  append(node, children);
  return node;
}

function append(node, children) {
  for (const child of children.flat(4)) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
}

function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

const ICON_PATHS = {
  chat: '<path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v6a2.5 2.5 0 0 1-2.5 2.5H9l-4 4v-4A2.5 2.5 0 0 1 4 12.5z"/><path d="M8 9h8M8 12h5"/>',
  box: '<path d="M3.5 7.5 12 3l8.5 4.5v9L12 21l-8.5-4.5z"/><path d="M3.5 7.5 12 12l8.5-4.5M12 12v9"/>',
  search: '<circle cx="11" cy="11" r="6.5"/><path d="m16 16 4 4"/>',
  check: '<path d="m4.5 12.5 5 5 10-11"/>',
  x: '<path d="M6 6l12 12M18 6 6 18"/>',
  back: '<path d="M14.5 5.5 8 12l6.5 6.5"/>',
  caret: '<path d="m9 6 6 6-6 6"/>',
  alert: '<path d="M12 8v5M12 16.5v.5"/><path d="M10.3 3.9 2.7 17a2 2 0 0 0 1.7 3h15.2a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/>',
  shield: '<path d="M12 3 5 6v5.5c0 4.4 2.9 7.9 7 9.5 4.1-1.6 7-5.1 7-9.5V6z"/><path d="m9 12 2 2 4-4"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 5v6h-6"/>',
  bolt: '<path d="M13 3 5.5 13H11l-1 8 7.5-10H12z"/>',
  send: '<path d="M4.5 12h13M12 6.5 18 12l-6 5.5"/>',
  doc: '<path d="M6 3.5h7l5 5v12H6z"/><path d="M13 3.5v5h5M9 13h6M9 16.5h4"/>',
  route: '<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="18" r="2.5"/><path d="M8.5 6H14a4 4 0 0 1 0 8h-4a4 4 0 0 0 0 8h4"/>',
};

function svg(name, size = 14) {
  return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${ICON_PATHS[name] || ''}</svg>`;
}

function iconSpan(name, size = 14) {
  return h('span', { class: 'ic', style: 'display:inline-flex;align-items:center', html: svg(name, size) });
}

const ORDER_STATUS = {
  pending_payment: '待付款',
  paid: '已付款待发货',
  processing: '处理中',
  shipped: '已发货',
  completed: '已完成',
  cancelled: '已取消',
  closed: '已关闭',
};

const SHIPMENT_STATUS = {
  pending: '待揽收', picked_up: '已揽收', in_transit: '运输中', delivering: '派送中',
  delivered: '已签收', exception: '物流异常', returned: '已退回',
  intercepted: '已拦截', delivery_failed: '派送失败',
};

const ACTION_LABEL = {
  cancel_order: '取消订单',
  modify_order: '修改收件信息',
  submit_service_request: '提交售后申请',
  return: '退货',
  exchange: '换货',
  repair: '维修',
  refund: '退款',
  reship: '补发',
  price_protection: '申请价保',
};

const REFUND_STATUS = {
  requested: '已申请待审核', pending_review: '审核中', approved: '审核通过',
  processing: '退款处理中', succeeded: '退款成功', failed: '退款失败',
  rejected: '已拒绝', cancelled: '已取消',
};

const CASE_STATUS = {
  submitted: '已提交', pending_review: '审核中', approved: '审核通过',
  rejected: '已拒绝', awaiting_customer: '等待用户操作',
  awaiting_return_shipment: '等待寄回', processing: '处理中',
  completed: '已完成', cancelled: '已取消',
};

const PREVIEW_LABEL = {
  order_no: '订单号',
  current_status: '当前状态',
  target_status: '目标状态',
  estimated_refund_display: '预计退款',
  refund_note: '退款说明',
  quantity: '数量',
  request_type: '申请类型',
  changes: '变更内容',
};

const FIELD_LABEL = {
  receiver_name: '收件人', receiver_phone: '收件电话', shipping_province: '省',
  shipping_city: '市', shipping_district: '区县', shipping_address: '详细地址',
  shipping_postal_code: '邮编', shipping_method: '配送方式',
  requested_delivery_at: '预约送达', order_remark: '订单备注',
};

// Fallback only: the real list comes from GET /api/v1/dev/customers so the
// switcher always matches whatever the seed actually wrote.
const FALLBACK_CUSTOMERS = [
  { customer_no: 'CUST-1001', display_name: '张三', order_count: null },
  { customer_no: 'CUST-1002', display_name: '李四', order_count: null },
];

const state = {
  customer: FALLBACK_CUSTOMERS[0].customer_no,
  customers: [],
  ready: null,
  conversations: [],
  conversationId: null,
  conversation: null,
  messages: [],
  pending: [],
  results: [],
  turnMeta: {},
  draftEcho: null,
  lastError: null,
  sending: false,
  orders: [],
  orderFilter: '',
  orderDetail: null,
  products: [],
  productQuery: '',
  productDetail: null,
  trace: null,
  tab: 'orders',
};

/* ------------------------------------------------------------------ API */

async function api(path, options = {}) {
  const headers = {
    'X-Demo-Customer-ID': state.customer,
    ...(options.body === undefined ? {} : { 'Content-Type': 'application/json' }),
    ...(options.headers || {}),
  };
  let response;
  try {
    response = await fetch(path, {
      method: options.method || 'GET',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch (networkError) {
    const err = new Error('无法连接服务，请确认后端已启动');
    err.code = 'network_error';
    throw err;
  }

  const text = await response.text();
  let payload = null;
  if (text) {
    try { payload = JSON.parse(text); } catch { /* non-JSON body */ }
  }
  if (!response.ok || (payload && payload.error)) {
    const detail = payload && payload.error ? payload.error : {};
    const err = new Error(detail.message || `请求失败（HTTP ${response.status}）`);
    err.code = detail.code || 'http_error';
    err.status = response.status;
    err.requestId = detail.request_id || response.headers.get('X-Request-ID') || '';
    throw err;
  }
  return payload || {};
}

/* ------------------------------------------------------------------ 格式化 */

function fmtTime(raw, withYear = false) {
  if (!raw) return '';
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  const pad = (n) => String(n).padStart(2, '0');
  const time = `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
  return withYear ? `${date.getFullYear()}-${time}` : time;
}

function statusChip(status, label) {
  return h('span', { class: 'chip', dataset: { status: status || '' } }, label || ORDER_STATUS[status] || status || '未知');
}

function money(amountMap) {
  if (!amountMap) return '—';
  return amountMap.paid_display || (amountMap.amount_display ?? '—');
}

function toast(kind, title, desc, ttl = 4200) {
  const node = h('div', { class: `toast ${kind}` },
    h('span', { class: 'bar' }),
    h('div', { class: 'body' },
      h('div', { class: 'title' }, title),
      desc ? h('div', { class: 'desc' }, desc) : null,
    ),
  );
  $('#toasts').append(node);
  setTimeout(() => {
    node.style.transition = 'opacity .2s ease';
    node.style.opacity = '0';
    setTimeout(() => node.remove(), 220);
  }, ttl);
}

function emptyPanel(title, desc) {
  return h('div', { class: 'empty-panel' }, h('strong', {}, title), desc ? h('div', {}, desc) : null);
}

/* ------------------------------------------------------------------ 启动 */

document.addEventListener('DOMContentLoaded', init);

async function init() {
  await loadCustomers();
  renderCustomerSelect();
  bindEvents();
  await loadReady();
  await Promise.all([loadConversations(), loadOrders()]);
  renderSuggestions();
  setInterval(tickCountdowns, 15000);
}

// loadCustomers pulls the seeded identities from the dev API so the switcher
// shows every customer the dataset contains, not a hard-coded pair.
async function loadCustomers() {
  try {
    const payload = await api('/api/v1/dev/customers');
    const list = payload.data || [];
    if (list.length > 0) {
      state.customers = list;
      if (!list.some((c) => c.customer_no === state.customer)) {
        state.customer = list[0].customer_no;
      }
      return;
    }
  } catch (err) {
    // Dev API disabled (APP_ENV=prod) or unreachable: keep the fallback pair.
  }
  state.customers = FALLBACK_CUSTOMERS;
}

function renderCustomerSelect() {
  const select = $('#customerSelect');
  clear(select);
  const list = state.customers.length > 0 ? state.customers : FALLBACK_CUSTOMERS;
  for (const customer of list) {
    const name = customer.display_name || '';
    const count = typeof customer.order_count === 'number' ? ` · ${customer.order_count} 单` : '';
    select.append(h('option', { value: customer.customer_no }, `${customer.customer_no} · ${name}${count}`));
  }
  select.value = state.customer;
}

function bindEvents() {
  $('#customerSelect').addEventListener('change', async (event) => {
    state.customer = event.target.value;
    resetForCustomerSwitch();
    await Promise.all([loadConversations(), loadOrders()]);
    renderSuggestions();
    toast('info', '已切换演示身份', `当前身份 ${state.customer}，只能看到该客户自己的订单与会话。`);
  });

  $('#refreshAll').addEventListener('click', async () => {
    await Promise.all([loadReady(), loadConversations(), loadOrders(), loadTrace()]);
    toast('info', '已刷新', '会话、订单与链路数据已重新拉取。');
  });

  $('#newConversation').addEventListener('click', createConversation);

  $('#closeConversation').addEventListener('click', async () => {
    if (!state.conversationId) return;
    if (!window.confirm('结束当前会话？结束后不能再发送消息。')) return;
    try {
      await api(`/api/v1/conversations/${state.conversationId}/close`, { method: 'POST' });
      await Promise.all([loadConversations(), loadConversation()]);
      toast('success', '会话已结束', '可以新建会话继续体验。');
    } catch (err) {
      toast('error', '结束会话失败', err.message);
    }
  });

  $('#sendBtn').addEventListener('click', () => sendMessage($('#composerInput').value));

  $('#composerInput').addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      sendMessage($('#composerInput').value);
    }
  });

  $('#composerInput').addEventListener('input', (event) => {
    const field = event.target;
    field.style.height = 'auto';
    field.style.height = `${Math.min(field.scrollHeight, 160)}px`;
  });

  $$('.tab').forEach((tab) => tab.addEventListener('click', () => switchTab(tab.dataset.tab)));

  $('#panelToggle').addEventListener('click', () => $('#layout').classList.toggle('show-inspector'));
}

function resetForCustomerSwitch() {
  state.conversationId = null;
  state.conversation = null;
  state.messages = [];
  state.pending = [];
  state.results = [];
  state.turnMeta = {};
  state.lastError = null;
  state.orderDetail = null;
  state.trace = null;
  state.productDetail = null;
  renderMessages();
  renderChatHeader();
  renderOrdersPanel();
  renderTracePanel();
  renderAuditPanel();
}

/* ------------------------------------------------------------------ 就绪状态 */

async function loadReady() {
  const badge = $('#modelBadge');
  const label = $('.label', badge);
  try {
    const payload = await api('/readyz');
    state.ready = payload.data || {};
    const mode = state.ready.llm_mode || 'disabled';
    badge.dataset.mode = mode;
    const modes = {
      openai: ['真实模型已启用', '对话请求会转发到已配置的 LLM'],
      offline: ['本地演示模型', '规则匹配 + 真实工具调用，无需 API key；不是大模型'],
      disabled: ['模型未启用', '聊天接口会返回 llm_unavailable'],
    };
    label.textContent = (modes[mode] || modes.disabled)[0];
    badge.title = (modes[mode] || modes.disabled)[1];

    // Show which database this process actually opened, so "I cannot see the
    // seeded data" turns into an obvious check instead of a guess.
    const ds = state.ready.dataset;
    if (ds) {
      const file = (state.ready.db_path || '').split('/').pop();
      badge.title = `${badge.title}｜${state.ready.db_path}`;
      const sub = document.querySelector('.brand-text p');
      if (sub) {
        if (!sub.dataset.original) sub.dataset.original = sub.textContent;
        sub.title = `数据源 ${state.ready.db_path}`;
        sub.textContent = `${file || 'sqlite'} · ${ds.orders} 单 / ${ds.conversations} 会话 / ${ds.messages} 消息`;
      }
    }
    if (mode === 'disabled') {
      toast('warn', '对话模型未启用',
        '设置 LLM_PROVIDER=offline 可用本地演示模型，或配置 LLM_ENABLED=true + LLM_BASE_URL 接入真实模型。', 9000);
    }
  } catch (err) {
    badge.dataset.mode = 'disabled';
    label.textContent = '服务不可用';
    badge.title = err.message;
  }
}

/* ------------------------------------------------------------------ 会话 */

async function loadConversations() {
  const panel = $('#convList');
  clear(panel);
  panel.append(h('div', { class: 'skeleton' }), h('div', { class: 'skeleton' }));
  try {
    const payload = await api('/api/v1/conversations?limit=30');
    state.conversations = payload.data || [];
    renderConversations();
    if (!state.conversationId && state.conversations.length > 0) {
      await selectConversation(state.conversations[0].conversation_id);
    }
  } catch (err) {
    clear(panel);
    panel.append(emptyPanel('会话加载失败', err.message));
  }
}

function renderConversations() {
  const panel = $('#convList');
  clear(panel);
  $('#convCount').textContent = String(state.conversations.length);

  if (state.conversations.length === 0) {
    panel.append(emptyPanel('还没有会话', '点上方「新建」开始一次对话。'));
    return;
  }
  for (const conv of state.conversations) {
    const active = conv.conversation_id === state.conversationId;
    const item = h('div', {
      class: `conv-item${active ? ' active' : ''}`,
      role: 'button',
      tabindex: '0',
      onclick: () => selectConversation(conv.conversation_id),
      onkeydown: (event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          selectConversation(conv.conversation_id);
        }
      },
    },
      h('div', { class: 'conv-item-title' },
        h('span', { class: 'no' }, conv.conversation_no),
        statusChip(conv.status, conv.status === 'open' ? '进行中' : '已结束'),
      ),
      h('div', { class: 'conv-meta' },
        h('span', {}, fmtTime(conv.created_at)),
        h('span', { class: 'sep' }, '·'),
        h('span', {}, conv.channel || 'web'),
        conv.pending_actions && conv.pending_actions.length
          ? h('span', { class: 'chip', dataset: { status: 'pending' } }, `待确认 ${conv.pending_actions.length}`)
          : null,
      ),
    );
    panel.append(item);
  }
}

async function createConversation() {
  try {
    const payload = await api('/api/v1/conversations', { method: 'POST', body: { channel: 'web_demo' } });
    const conv = payload.data;
    state.results = [];
    state.turnMeta = {};
    state.lastError = null;
    await loadConversations();
    await selectConversation(conv.conversation_id);
    $('#composerInput').focus();
    toast('success', '会话已创建', conv.conversation_no);
  } catch (err) {
    toast('error', '创建会话失败', err.message);
  }
}

async function selectConversation(id) {
  if (!id) return;
  state.conversationId = id;
  state.results = [];
  state.turnMeta = {};
  state.lastError = null;
  state.draftEcho = null;
  state.pending = [];
  renderConversations();
  await loadConversation();
  await loadMessages();
  if (state.tab === 'trace' || state.tab === 'audit') loadTrace();
  updateComposer();
}

async function loadConversation() {
  if (!state.conversationId) return;
  try {
    const payload = await api(`/api/v1/conversations/${state.conversationId}`);
    state.conversation = payload.data;
    state.pending = payload.data.pending_actions || [];
    renderChatHeader();
    renderConversations();
  } catch (err) {
    toast('error', '会话详情加载失败', err.message);
  }
}

async function loadMessages() {
  if (!state.conversationId) {
    state.messages = [];
    renderMessages();
    return;
  }
  try {
    const payload = await api(`/api/v1/conversations/${state.conversationId}/messages?limit=50`);
    state.messages = payload.data || [];
  } catch (err) {
    toast('error', '消息加载失败', err.message);
  }
  renderMessages();
}

function renderChatHeader() {
  const title = $('#chatTitle');
  const sub = $('#chatSub');
  const conv = state.conversation;
  if (!conv) {
    title.textContent = '未选择会话';
    sub.textContent = '在左侧新建一个会话，或选择已有会话继续';
    $('#closeConversation').disabled = true;
    return;
  }
  title.textContent = conv.conversation_no;
  const parts = [conv.conversation_id.slice(0, 8), conv.status === 'open' ? '进行中' : '已结束', `创建于 ${fmtTime(conv.created_at, true)}`];
  sub.textContent = parts.join(' · ');
  $('#closeConversation').disabled = conv.status !== 'open';
}

/* ------------------------------------------------------------------ 消息渲染 */

function renderMessages() {
  const host = $('#messages');
  clear(host);

  if (!state.conversationId) {
    host.append(welcomeState());
    return;
  }

  const inner = h('div', { class: 'messages-inner' });

  if (state.messages.length === 0 && !state.sending) {
    inner.append(h('div', { class: 'empty-state' },
      h('div', { class: 'glyph', html: svg('chat', 22) }),
      h('h3', {}, '会话已就绪'),
      h('p', {}, '可以直接提问，比如「我的订单到哪了」。需要取消、改地址或申请售后时，系统只会生成待确认卡片，你点确认才会真正执行。'),
    ));
  }

  for (const message of state.messages) inner.append(messageNode(message));

  for (const result of state.results) inner.append(resultCard(result));

  if (state.pending.length > 0) {
    const wrap = h('div', { class: 'pending-wrap' },
      h('div', { class: 'pending-section-label' }, iconSpan('alert', 12), `待确认操作 · ${state.pending.length}`),
    );
    for (const action of state.pending) wrap.append(pendingCard(action));
    inner.append(wrap);
  }

  if (state.draftEcho) {
    inner.append(h('div', { class: 'msg user' },
      h('div', { class: 'msg-bubble' }, state.draftEcho.content),
      h('div', { class: 'msg-meta' }, h('span', { class: 'role' }, '客户'), h('span', {}, '发送中…')),
    ));
  }

  if (state.sending && !state.draftEcho) {
    inner.append(h('div', { class: 'msg assistant' },
      h('div', { class: 'msg-bubble' }, h('span', { class: 'typing' }, h('i'), h('i'), h('i'))),
    ));
  }

  if (state.lastError) inner.append(errorNode(state.lastError));

  host.append(inner);
  scrollToBottom();
}

function welcomeState() {
  return h('div', { class: 'empty-state' },
    h('div', { class: 'glyph', html: svg('chat', 22) }),
    h('h3', {}, '开始一次客服对话'),
    h('p', {}, '先在左侧新建一个会话。演示身份 CUST-1001 名下有 12 笔不同状态的订单，覆盖取消、改地址、退货、补发、价保等场景。'),
  );
}

function messageNode(message) {
  const isUser = message.role === 'user';
  const meta = state.turnMeta[message.id];
  const wrap = h('div', { class: `msg ${isUser ? 'user' : 'assistant'}` });

  // Demo explainability: the model writes one line of "why" before every reply
  // (and before every tool round). The server strips it out of the text and sends
  // it separately, so it is rendered here as a note above the bubble.
  if (!isUser) {
    for (const note of reasonNotes(message, meta)) wrap.append(note);
  }

  wrap.append(h('div', { class: 'msg-bubble' }, message.content));

  if (meta && meta.traces && meta.traces.length > 0) {
    const strip = h('div', { class: 'tool-strip' },
      h('span', { class: 'strip-label' }, iconSpan('bolt', 11), `调用工具 ${meta.traces.length} 个`),
    );
    for (const trace of meta.traces) {
      strip.append(h('span', { class: 'tool-chip', dataset: { status: trace.status || 'succeeded' }, title: trace.error_code || '' },
        h('span', { class: 'dot' }),
        trace.tool_name,
        h('span', { class: 'ms' }, `${trace.latency_ms ?? 0}ms`),
      ));
    }
    wrap.append(strip);
  }

  const metaRow = h('div', { class: 'msg-meta' },
    h('span', { class: 'role' }, isUser ? '客户' : '客服助手'),
    h('span', {}, fmtTime(message.created_at)),
  );
  if (meta && meta.idempotent) metaRow.append(h('span', { class: 'chip' }, '幂等重放'));
  if (meta && meta.degraded) metaRow.append(h('span', { class: 'chip', dataset: { status: 'pending' } }, '降级回复'));
  wrap.append(metaRow);
  return wrap;
}

function reasonNotes(message, meta) {
  const rows = [];
  const list = meta && Array.isArray(meta.reasons) ? meta.reasons : [];

  if (list.length > 0) {
    for (const entry of list) {
      const isAnswer = entry.kind === 'answer';
      rows.push(reasonRow(
        entry.reason,
        isAnswer ? '备注' : `备注 · 第${entry.round}轮`,
        isAnswer ? null : entry.tools,
      ));
    }
    return rows;
  }
  // History reloaded from the server: only the note of the visible reply survives.
  if (message.reason) rows.push(reasonRow(message.reason, '备注', null));
  return rows;
}

function reasonRow(text, tag, tools) {
  const note = String(text || '').trim();
  return h('div', { class: `reason${note ? '' : ' missing'}` },
    h('span', { class: 'reason-tag' }, tag),
    h('span', { class: 'reason-text' }, note || '本轮模型没有给出理由'),
    tools && tools.length ? h('span', { class: 'reason-tools' }, tools.join(' · ')) : null,
  );
}

function errorNode(lastError) {
  const { error } = lastError;
  const hint = {
    llm_unavailable: '用户消息已保存，点击重试不会重复插入消息。',
    action_expired: '这张卡片已过期，请重新发起申请。',
    conflict: '服务端状态已变化，界面已刷新。',
  }[error.code] || '';

  return h('div', { class: 'msg assistant' },
    h('div', { class: 'msg-bubble error' },
      h('div', { style: 'font-weight:650;margin-bottom:3px' }, `请求失败 · ${error.code}`),
      error.message,
      error.requestId ? h('div', { style: 'margin-top:6px;font-family:var(--mono);font-size:10.5px;color:var(--muted)' }, `request_id: ${error.requestId}`) : null,
    ),
    hint ? h('div', { class: 'msg-meta' }, h('span', {}, hint)) : null,
    h('div', { class: 'msg-actions' },
      h('button', { class: 'btn sm', onclick: () => sendMessage(lastError.content, lastError.clientMessageId) }, '重试'),
      h('button', { class: 'btn ghost sm', onclick: () => { state.lastError = null; renderMessages(); } }, '忽略'),
    ),
  );
}

function pendingCard(action) {
  const title = ACTION_LABEL[action.action_type] || action.action_type;
  const preview = action.preview || {};
  const dl = h('dl', { class: 'pending-preview' });

  // Only fields a human needs to approve the change; raw booleans and internal
  // target statuses stay out of the card.
  const skip = new Set(['summary', 'target_status', 'refund_expected', 'estimated_refund_minor']);
  for (const [key, value] of Object.entries(preview)) {
    if (skip.has(key)) continue;
    if (key === 'changes') {
      const diffs = Array.isArray(value) ? value : [];
      for (const diff of diffs) {
        dl.append(
          h('dt', {}, FIELD_LABEL[diff.field] || diff.field),
          h('dd', {}, `${diff.before || '（空）'} → ${diff.after || '（空）'}`),
        );
      }
      continue;
    }
    const shown = key === 'current_status' ? (ORDER_STATUS[value] || value) : value;
    dl.append(h('dt', {}, PREVIEW_LABEL[key] || key), h('dd', {}, String(shown)));
  }

  const expire = h('span', { class: 'pending-expire', dataset: { expires: action.expires_at } },
    iconSpan('alert', 11), countdownText(action.expires_at));

  const confirmBtn = h('button', { class: 'btn primary sm' }, '确认执行');
  const rejectBtn = h('button', { class: 'btn ghost sm' }, '拒绝');
  confirmBtn.addEventListener('click', () => confirmAction(action, confirmBtn));
  rejectBtn.addEventListener('click', () => rejectAction(action, rejectBtn));

  return h('div', { class: 'pending-card' },
    h('div', { class: 'pending-head' },
      h('div', {},
        h('div', { class: 'pending-title' }, `待确认 · ${title}`),
        h('div', { class: 'pending-sub' }, action.action_no),
      ),
      statusChip('pending', '等待确认'),
    ),
    h('div', { class: 'pending-summary' }, action.summary || ''),
    dl.childElementCount ? dl : null,
    h('div', { class: 'pending-foot' },
      expire,
      h('div', { class: 'pending-buttons' }, rejectBtn, confirmBtn),
    ),
  );
}

function countdownText(expiresAt) {
  const ms = new Date(expiresAt).getTime() - Date.now();
  if (!Number.isFinite(ms)) return '';
  if (ms <= 0) return '已过期';
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.floor((ms % 60000) / 1000);
  return minutes > 0 ? `${minutes} 分 ${seconds} 秒后过期` : `${seconds} 秒后过期`;
}

function tickCountdowns() {
  for (const node of $$('[data-expires]')) {
    const ms = new Date(node.dataset.expires).getTime() - Date.now();
    node.textContent = countdownText(node.dataset.expires);
    node.classList.toggle('urgent', Number.isFinite(ms) && ms > 0 && ms < 60000);
    node.prepend(iconSpan('alert', 11));
  }
}

function resultCard(result) {
  const data = result.result || {};
  const rows = [];
  const push = (label, value) => { if (value) rows.push([label, value]); };

  push('订单号', data.order_no);
  push('订单状态', ORDER_STATUS[data.order_status] || data.order_status);
  push('退款单号', data.refund_no);
  push('退款金额', data.refund_amount_display);
  push('退款状态', REFUND_STATUS[data.refund_status] || data.refund_status);
  push('售后单号', data.case_no);
  push('售后状态', CASE_STATUS[data.case_status] || data.case_status);
  push('变更字段', Array.isArray(data.changed_fields) ? data.changed_fields.join('、') : '');

  const dl = h('dl', { class: 'pending-preview' });
  for (const [label, value] of rows) dl.append(h('dt', {}, label), h('dd', {}, String(value)));

  return h('div', { class: 'result-card' },
    h('div', { class: 'result-head' }, iconSpan('check', 15), `已执行 · ${ACTION_LABEL[result.action_type] || result.action_type}`),
    h('div', { class: 'result-body' },
      dl.childElementCount ? dl : h('div', { style: 'font-size:12px;color:var(--muted)' }, '服务端已返回执行结果。'),
      result.idempotent ? h('div', { style: 'margin-top:6px' }, h('span', { class: 'chip' }, '幂等重放：本次没有重复执行')) : null,
    ),
  );
}

function scrollToBottom() {
  const host = $('#messages');
  host.scrollTop = host.scrollHeight;
}

/* ------------------------------------------------------------------ 发送 / 确认 */

async function sendMessage(text, clientMessageId) {
  const content = String(text || '').trim();
  if (!content || state.sending) return;

  if (!state.conversationId) {
    await createConversation();
    if (!state.conversationId) return;
  }
  if (state.conversation && state.conversation.status === 'closed') {
    toast('warn', '会话已结束', '请新建会话后再发送消息。');
    return;
  }

  const cmid = clientMessageId || `cm-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  state.sending = true;
  state.lastError = null;
  state.draftEcho = { content };
  $('#composerInput').value = '';
  $('#composerInput').style.height = '';
  updateComposer();
  renderMessages();

  try {
    const payload = await api(`/api/v1/conversations/${state.conversationId}/messages`, {
      method: 'POST',
      body: { message: content, client_message_id: cmid },
    });
    const data = payload.data || {};
    state.draftEcho = null;

    if (data.assistant_message) {
      state.turnMeta[data.assistant_message.id] = {
        traces: data.tool_executions || [],
        reasons: data.reasons || [],
        degraded: Boolean(data.degraded),
        idempotent: Boolean(data.idempotent),
      };
    }
    state.pending = data.pending_actions || [];

    await loadMessages();
    await loadConversation();
    loadTrace();

    if (data.idempotent) {
      toast('info', '幂等重放', '相同 client_message_id 命中了缓存，本轮没有重复写消息或重复调用工具。');
    }
    if (data.degraded) {
      toast('warn', '已降级回复', '本轮触发了工具轮次/次数上限，返回了安全兜底话术。');
    }
  } catch (err) {
    state.draftEcho = null;
    state.lastError = { content, clientMessageId: cmid, error: err };
    await loadMessages().catch(() => {});
    toast('error', '发送失败', err.message, 6000);
  } finally {
    state.sending = false;
    updateComposer();
    renderMessages();
  }
}

async function confirmAction(action, button) {
  const original = button.innerHTML;
  button.disabled = true;
  button.innerHTML = '<span class="spinner"></span> 执行中';

  try {
    const payload = await api(
      `/api/v1/conversations/${state.conversationId}/actions/${action.action_id}/confirm`,
      { method: 'POST' },
    );
    state.results.push(payload.data);
    state.pending = state.pending.filter((item) => item.action_id !== action.action_id);
    toast('success', '操作已执行', summaryOf(payload.data), 6000);
    await Promise.all([loadConversation(), loadMessages(), loadOrders(), loadTrace()]);
    // The order opened in the inspector may have just changed status.
    if (state.orderDetail) {
      const { orderNo, tab } = state.orderDetail;
      await openOrder(orderNo);
      if (tab !== 'detail' && state.orderDetail) await setOrderTab(tab);
    }
  } catch (err) {
    toast('error', '操作未执行', err.message, 7000);
    await loadConversation();
    renderMessages();
  } finally {
    button.disabled = false;
    button.innerHTML = original;
  }
}

async function rejectAction(action, button) {
  button.disabled = true;
  try {
    await api(`/api/v1/conversations/${state.conversationId}/actions/${action.action_id}/reject`, { method: 'POST' });
    state.pending = state.pending.filter((item) => item.action_id !== action.action_id);
    toast('info', '已拒绝', '订单没有任何变化，本次操作已记录审计。');
    await Promise.all([loadConversation(), loadMessages(), loadTrace()]);
  } catch (err) {
    toast('error', '拒绝失败', err.message);
  } finally {
    button.disabled = false;
  }
}

function summaryOf(data) {
  const result = data.result || {};
  const parts = [];
  if (result.refund_no) parts.push(`退款单 ${result.refund_no}`);
  if (result.refund_amount_display) parts.push(result.refund_amount_display);
  if (result.case_no) parts.push(`售后单 ${result.case_no}`);
  if (result.order_status) parts.push(`订单状态 ${ORDER_STATUS[result.order_status] || result.order_status}`);
  return parts.join(' · ') || '业务数据已更新';
}

function updateComposer() {
  const input = $('#composerInput');
  const send = $('#sendBtn');
  const closed = state.conversation && state.conversation.status === 'closed';
  const disabled = !state.conversationId || closed || state.sending;
  input.disabled = disabled;
  send.disabled = disabled;
  input.placeholder = closed
    ? '会话已结束，请新建会话'
    : '描述你的问题，例如：我的订单 ORD-10003 到哪了？';
}

/* ------------------------------------------------------------------ 快捷提示 */

function renderSuggestions() {
  const host = $('#suggestions');
  clear(host);
  if (!state.conversationId) return;

  const list = [];
  const orders = state.orders || [];
  const shipped = orders.find((o) => o.order_status === 'shipped');
  const cancelable = orders.find((o) => ['pending_payment', 'paid', 'processing'].includes(o.order_status));
  const completed = orders.find((o) => o.order_status === 'completed');

  list.push('我的订单现在什么状态？');
  if (shipped) list.push(`查一下 ${shipped.order_no} 的物流`);
  if (cancelable) list.push(`我要取消 ${cancelable.order_no}`);
  if (completed) list.push(`${completed.order_no} 的发票开好了吗`);
  if (completed) list.push(`我要给 ${completed.order_no} 申请退货`);
  list.push('退款一般几天到账？');

  const seen = new Set();
  for (const text of list) {
    if (seen.has(text)) continue;
    seen.add(text);
    host.append(h('button', { class: 'chip-suggest', onclick: () => sendMessage(text) }, text));
  }
}

/* ------------------------------------------------------------------ 右侧面板 */

function switchTab(tab) {
  state.tab = tab;
  $$('.tab').forEach((node) => node.classList.toggle('active', node.dataset.tab === tab));
  $$('.panel').forEach((node) => node.classList.toggle('active', node.id === `panel-${tab}`));
  if (tab === 'trace' || tab === 'audit') loadTrace();
  if (tab === 'products' && state.products.length === 0) searchProducts('');
  if (tab === 'orders') renderOrdersPanel();
}

function panelHead(label, extra) {
  return h('div', { class: 'panel-head' },
    h('span', { class: 'label' }, label),
    extra ? h('span', { class: 'label' }, extra) : null,
  );
}

/* ---- 订单 ---- */

async function loadOrders() {
  const status = state.orderFilter;
  try {
    const payload = await api(`/api/v1/orders?limit=30${status ? `&status=${encodeURIComponent(status)}` : ''}`);
    state.orders = payload.data || [];
  } catch (err) {
    toast('error', '订单加载失败', err.message);
  }
  renderOrdersPanel();
  renderSuggestions();
}

function orderFilters() {
  const options = [
    ['', '全部'],
    ['pending_payment', '待付款'],
    ['paid', '待发货'],
    ['shipped', '已发货'],
    ['completed', '已完成'],
    ['cancelled', '已取消'],
  ];
  const row = h('div', { class: 'filter-row' });
  for (const [value, label] of options) {
    row.append(h('button', {
      class: `chip-filter${state.orderFilter === value ? ' active' : ''}`,
      onclick: async () => {
        state.orderFilter = value;
        state.orderDetail = null;
        await loadOrders();
      },
    }, label));
  }
  return row;
}

function renderOrdersPanel() {
  const panel = $('#panel-orders');
  clear(panel);

  if (state.orderDetail) {
    renderOrderDetail(panel);
    return;
  }

  panel.append(panelHead('我的订单', `${state.orders.length} 条`), orderFilters());

  if (state.orders.length === 0) {
    panel.append(emptyPanel('没有符合条件的订单', '换个状态筛选，或在右上角「演示身份」切换到其他客户。'));
    return;
  }
  for (const order of state.orders) {
    panel.append(h('button', { class: 'order-row', onclick: () => openOrder(order.order_no) },
      h('div', { class: 'order-row-top' },
        h('span', { class: 'order-no' }, order.order_no),
        statusChip(order.order_status),
      ),
      h('div', { class: 'order-row-bottom' },
        h('span', { class: 'order-amount' }, order.amounts ? order.amounts.paid_display : '—'),
        h('span', { class: 'order-date' }, fmtTime(order.created_at, true)),
      ),
    ));
  }
}

async function openOrder(orderNo) {
  state.orderDetail = { orderNo, tab: 'detail', order: null, shipments: null, cases: null, invoices: null };
  renderOrdersPanel();
  try {
    const payload = await api(`/api/v1/orders/${encodeURIComponent(orderNo)}`);
    state.orderDetail.order = payload.data;
  } catch (err) {
    state.orderDetail = null;
    toast('error', '订单详情加载失败', err.message);
  }
  renderOrdersPanel();
}

async function setOrderTab(tab) {
  const detail = state.orderDetail;
  if (!detail) return;
  detail.tab = tab;
  renderOrdersPanel();
  try {
    if (tab === 'logistics' && detail.shipments === null) {
      const payload = await api(`/api/v1/orders/${encodeURIComponent(detail.orderNo)}/shipments`);
      detail.shipments = payload.data || [];
    }
    if (tab === 'cases' && detail.cases === null) {
      const payload = await api(`/api/v1/orders/${encodeURIComponent(detail.orderNo)}/service-cases`);
      detail.cases = payload.data || [];
    }
    if (tab === 'invoices' && detail.invoices === null) {
      const payload = await api(`/api/v1/invoices?order_no=${encodeURIComponent(detail.orderNo)}`);
      detail.invoices = payload.data || [];
    }
  } catch (err) {
    toast('error', '加载失败', err.message);
  }
  renderOrdersPanel();
}

function renderOrderDetail(panel) {
  const detail = state.orderDetail;
  const order = detail.order;

  panel.append(h('div', { class: 'detail-bar' },
    h('button', { class: 'back-link', onclick: () => { state.orderDetail = null; renderOrdersPanel(); } },
      iconSpan('back', 13), '返回列表'),
    h('span', { class: 'detail-title' }, detail.orderNo),
    order ? statusChip(order.order_status) : null,
  ));

  if (!order) {
    panel.append(h('div', { class: 'skeleton' }), h('div', { class: 'skeleton' }));
    return;
  }

  const tabs = h('div', { class: 'sub-tabs' });
  for (const [value, label] of [['detail', '详情'], ['logistics', '物流'], ['cases', '售后'], ['invoices', '发票']]) {
    tabs.append(h('button', {
      class: `sub-tab${detail.tab === value ? ' active' : ''}`,
      onclick: () => setOrderTab(value),
    }, label));
  }
  panel.append(tabs);

  if (detail.tab === 'detail') panel.append(orderDetailSection(order));
  if (detail.tab === 'logistics') panel.append(shipmentSection(detail.shipments));
  if (detail.tab === 'cases') panel.append(caseSection(detail.cases));
  if (detail.tab === 'invoices') panel.append(invoiceSection(detail.invoices));

  panel.append(h('div', { class: 'detail-section' },
    h('button', {
      class: 'btn block sm',
      onclick: () => {
        const text = detail.tab === 'logistics'
          ? `查一下 ${order.order_no} 的物流`
          : `查一下 ${order.order_no} 的订单状态`;
        $('#composerInput').value = text;
        $('#composerInput').focus();
        $('#layout').classList.remove('show-inspector');
      },
    }, '在对话中询问这个订单'),
  ));
}

function discountText(minor) {
  if (minor === undefined || minor === null) return '—';
  if (minor <= 0) return '¥0.00';
  return `-¥${(minor / 100).toFixed(2)}`;
}

function orderDetailSection(order) {
  const amounts = order.amounts || {};
  const receiver = order.receiver || {};
  const box = h('div', {},
    h('div', { class: 'detail-section' },
      h('h3', {}, '金额'),
      h('div', { class: 'amount-lg' }, amounts.paid_display || '—'),
      h('dl', { class: 'kv' },
        h('dt', {}, '商品金额'), h('dd', {}, amounts.item_amount_minor !== undefined ? `¥${(amounts.item_amount_minor / 100).toFixed(2)}` : '—'),
        h('dt', {}, '优惠'), h('dd', {}, discountText(amounts.discount_amount_minor)),
        h('dt', {}, '运费'), h('dd', {}, amounts.shipping_fee_minor !== undefined ? `¥${(amounts.shipping_fee_minor / 100).toFixed(2)}` : '—'),
        h('dt', {}, '已退款'), h('dd', {}, amounts.refunded_display || '¥0.00'),
      ),
    ),
    h('div', { class: 'detail-section' },
      h('h3', {}, '商品'),
      h('div', { class: 'items-list' },
        ...(order.items || []).map((item) => h('div', { class: 'item-row' },
          h('div', {},
            h('div', { class: 'name' }, item.product_name),
            h('div', { class: 'qty' }, `${item.sku_name || '默认规格'} × ${item.quantity}${item.refunded_quantity ? ` · 已退 ${item.refunded_quantity}` : ''}`),
          ),
          h('div', { class: 'price' }, item.unit_price_display || ''),
        )),
      ),
    ),
    h('div', { class: 'detail-section' },
      h('h3', {}, '收件信息'),
      h('dl', { class: 'kv' },
        h('dt', {}, '收件人'), h('dd', {}, receiver.name || '—'),
        h('dt', {}, '电话'), h('dd', { class: 'mono' }, receiver.phone_masked || '—'),
        h('dt', {}, '地址'), h('dd', {}, [receiver.province, receiver.city, receiver.district, receiver.address].filter(Boolean).join(' ') || '—'),
        h('dt', {}, '配送方式'), h('dd', {}, order.shipping_method || '—'),
      ),
    ),
    h('div', { class: 'detail-section' },
      h('h3', {}, '时间'),
      h('dl', { class: 'kv' },
        h('dt', {}, '下单'), h('dd', { class: 'mono' }, fmtTime(order.created_at, true)),
        h('dt', {}, '付款'), h('dd', { class: 'mono' }, order.paid_at ? fmtTime(order.paid_at, true) : '—'),
        h('dt', {}, '发货'), h('dd', { class: 'mono' }, order.shipped_at ? fmtTime(order.shipped_at, true) : '—'),
        h('dt', {}, '完成'), h('dd', { class: 'mono' }, order.delivered_at ? fmtTime(order.delivered_at, true) : '—'),
      ),
    ),
  );
  return box;
}

function shipmentSection(shipments) {
  if (shipments === null) return h('div', { class: 'skeleton' });
  if (shipments.length === 0) return emptyPanel('还没有物流记录', '订单发货后这里会显示包裹与轨迹节点。');

  const box = h('div', {});
  for (const shipment of shipments) {
    box.append(h('div', { class: 'detail-section' },
      h('div', { class: 'detail-bar', style: 'margin-bottom:6px' },
        h('span', { class: 'detail-title' }, shipment.tracking_no || shipment.shipment_no),
        statusChip(shipment.status, SHIPMENT_STATUS[shipment.status] || shipment.status),
      ),
      h('dl', { class: 'kv' },
        h('dt', {}, '承运商'), h('dd', {}, shipment.carrier_name || '—'),
        h('dt', {}, '预计送达'), h('dd', { class: 'mono' }, shipment.estimated_delivery_at ? fmtTime(shipment.estimated_delivery_at, true) : '未提供'),
      ),
      h('div', { style: 'height:10px' }),
      h('div', { class: 'timeline' },
        ...(shipment.events || []).map((event, index) => h('div', { class: `timeline-item${index === 0 ? ' latest' : ''}` },
          h('div', { class: 't-time' }, fmtTime(event.time, true)),
          h('div', { class: 't-desc' }, event.description),
          event.location ? h('div', { class: 't-loc' }, event.location) : null,
        )),
      ),
    ));
  }
  return box;
}

function caseSection(cases) {
  if (cases === null) return h('div', { class: 'skeleton' });
  if (cases.length === 0) return emptyPanel('没有售后单', '退货、换货、维修、补发、价保申请确认后会出现在这里。');

  const box = h('div', {});
  for (const item of cases) {
    box.append(h('div', { class: 'detail-section' },
      h('div', { class: 'detail-bar', style: 'margin-bottom:6px' },
        h('span', { class: 'detail-title' }, item.case_no),
        statusChip(item.case_status, item.case_status),
      ),
      h('dl', { class: 'kv' },
        h('dt', {}, '类型'), h('dd', {}, ACTION_LABEL[item.case_type] || item.case_type),
        h('dt', {}, '数量'), h('dd', {}, String(item.requested_quantity)),
        h('dt', {}, '原因'), h('dd', {}, item.reason || '—'),
        h('dt', {}, '创建'), h('dd', { class: 'mono' }, fmtTime(item.created_at, true)),
      ),
    ));
  }
  return box;
}

function invoiceSection(invoices) {
  if (invoices === null) return h('div', { class: 'skeleton' });
  if (invoices.length === 0) return emptyPanel('没有开票记录', '演示环境不生成真实发票文件。');

  const box = h('div', {});
  for (const invoice of invoices) {
    box.append(h('div', { class: 'detail-section' },
      h('div', { class: 'detail-bar', style: 'margin-bottom:6px' },
        h('span', { class: 'detail-title' }, invoice.invoice_no || '待开票'),
        statusChip(invoice.invoice_status, invoice.invoice_status),
      ),
      h('dl', { class: 'kv' },
        h('dt', {}, '类型'), h('dd', {}, invoice.invoice_type),
        h('dt', {}, '抬头'), h('dd', {}, invoice.title || '—'),
        h('dt', {}, '金额'), h('dd', {}, invoice.amount_display || '—'),
        h('dt', {}, '申请'), h('dd', { class: 'mono' }, fmtTime(invoice.requested_at, true)),
        h('dt', {}, '开票'), h('dd', { class: 'mono' }, invoice.issued_at ? fmtTime(invoice.issued_at, true) : '—'),
      ),
    ));
  }
  return box;
}

/* ---- 商品 ---- */

async function searchProducts(query) {
  state.productQuery = query || '';
  try {
    const payload = await api(`/api/v1/products?limit=20${state.productQuery ? `&q=${encodeURIComponent(state.productQuery)}` : ''}`);
    state.products = payload.data || [];
  } catch (err) {
    toast('error', '商品加载失败', err.message);
    state.products = [];
  }
  renderProductsPanel();
}

function renderProductsPanel() {
  const panel = $('#panel-products');
  clear(panel);

  if (state.productDetail) {
    renderProductDetail(panel);
    return;
  }

  const input = h('input', {
    type: 'search',
    placeholder: '搜索商品，例如 咖啡机',
    value: state.productQuery,
  });
  let timer = null;
  input.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(() => searchProducts(input.value.trim()), 250);
  });

  panel.append(panelHead('在售商品', `${state.products.length} 条`),
    h('label', { class: 'search-box' }, iconSpan('search', 14), input));

  if (state.products.length === 0) {
    panel.append(emptyPanel('没有匹配的商品', '试试「咖啡机」「空气炸锅」这类关键词。'));
    return;
  }

  for (const product of state.products) {
    panel.append(h('button', { class: 'product-row', onclick: () => openProduct(product.product_id) },
      h('div', { class: 'product-name' }, product.name),
      h('div', { class: 'product-sub' }, `${product.product_code} · ${product.category || '未分类'}${product.brand ? ` · ${product.brand}` : ''}`),
      product.description ? h('div', { class: 'product-desc' }, product.description) : null,
    ));
  }
}

async function openProduct(productId) {
  state.productDetail = { loading: true };
  renderProductsPanel();
  try {
    const payload = await api(`/api/v1/products/${encodeURIComponent(productId)}`);
    state.productDetail = payload.data;
  } catch (err) {
    state.productDetail = null;
    toast('error', '商品详情加载失败', err.message);
  }
  renderProductsPanel();
}

function renderProductDetail(panel) {
  panel.append(h('div', { class: 'detail-bar' },
    h('button', { class: 'back-link', onclick: () => { state.productDetail = null; renderProductsPanel(); } },
      iconSpan('back', 13), '返回'),
    h('span', { class: 'detail-title' }, state.productDetail.loading ? '加载中…' : state.productDetail.name),
  ));

  if (state.productDetail.loading) {
    panel.append(h('div', { class: 'skeleton' }), h('div', { class: 'skeleton' }));
    return;
  }

  const product = state.productDetail;
  panel.append(h('div', { class: 'detail-section' },
    h('dl', { class: 'kv' },
      h('dt', {}, '编码'), h('dd', { class: 'mono' }, product.product_code),
      h('dt', {}, '品牌'), h('dd', {}, product.brand || '—'),
      h('dt', {}, '分类'), h('dd', {}, product.category || '—'),
    ),
    product.description ? h('div', { style: 'margin-top:8px;font-size:12px;color:var(--text-2);line-height:1.6' }, product.description) : null,
  ));

  const skus = product.skus || [];
  panel.append(h('div', { class: 'detail-section' },
    h('h3', {}, `SKU · ${skus.length}`),
    ...skus.map((sku) => h('div', { class: 'sku-row' },
      h('div', {},
        h('div', {}, sku.sku_name),
        h('div', { style: 'font-size:10.5px;color:var(--muted)' }, `${sku.sku_code} · 库存 ${sku.stock_quantity}`),
      ),
      h('div', { style: 'font-weight:600' }, sku.list_price_display || ''),
    )),
  ));
}

/* ---- 链路 / 审计 ---- */

async function loadTrace() {
  if (!state.conversationId) {
    state.trace = null;
    renderTracePanel();
    renderAuditPanel();
    return;
  }
  try {
    const payload = await api(`/api/v1/dev/conversations/${state.conversationId}/trace`);
    state.trace = payload.data;
  } catch (err) {
    state.trace = { error: err };
  }
  renderTracePanel();
  renderAuditPanel();
}

function renderTracePanel() {
  const panel = $('#panel-trace');
  clear(panel);

  if (!state.conversationId) {
    panel.append(emptyPanel('先选择一个会话', '这里会显示本轮对话触发过的全部工具调用。'));
    return;
  }
  if (!state.trace) {
    panel.append(h('div', { class: 'skeleton' }), h('div', { class: 'skeleton' }));
    return;
  }
  if (state.trace.error) {
    const err = state.trace.error;
    panel.append(emptyPanel(
      err.code === 'not_found' ? '诊断接口未开放' : '链路加载失败',
      err.code === 'not_found' ? '需要 APP_ENV=dev 且 DEV_API_ENABLED=true。' : err.message,
    ));
    return;
  }

  const executions = state.trace.tool_executions || [];
  panel.append(panelHead('工具调用', `${executions.length} 次`));
  if (executions.length === 0) {
    panel.append(emptyPanel('这个会话还没有工具调用', '发一条消息后，这里会出现模型请求的每个工具及其入参/结果（个人数据已脱敏）。'));
    return;
  }

  executions.forEach((execution, index) => {
    const card = h('div', { class: 'trace-card' + (index === 0 ? ' open' : '') });
    const head = h('div', { class: 'trace-head', onclick: () => card.classList.toggle('open') },
      h('span', { class: 'trace-caret', html: svg('caret', 12) }),
      h('span', { class: 'trace-name' }, execution.tool_name),
      statusChip(execution.status, execution.status),
      h('span', { class: 'trace-latency' }, `${execution.latency_ms ?? 0}ms`),
    );
    const body = h('div', { class: 'trace-body' },
      h('div', { class: 'label' }, '请求时间'),
      h('pre', { style: 'margin-bottom:8px' }, `${fmtTime(execution.created_at, true)} · call_id ${execution.tool_call_id}`),
      h('div', { class: 'label' }, '入参（已脱敏）'),
      h('pre', {}, prettyJSON(execution.input)),
      h('div', { class: 'label' }, execution.status === 'succeeded' ? '结果' : '错误'),
      h('pre', {}, execution.status === 'succeeded'
        ? prettyJSON(execution.output)
        : `${execution.error_code || 'error'}: ${execution.error_message || ''}`),
    );
    card.append(head, body);
    panel.append(card);
  });
}

function renderAuditPanel() {
  const panel = $('#panel-audit');
  clear(panel);

  if (!state.conversationId) {
    panel.append(emptyPanel('先选择一个会话', '确认/拒绝写操作后，这里会记录变更前后的快照。'));
    return;
  }
  if (!state.trace) {
    panel.append(h('div', { class: 'skeleton' }));
    return;
  }
  if (state.trace.error) {
    panel.append(emptyPanel('审计接口未开放', '需要 APP_ENV=dev 且 DEV_API_ENABLED=true。'));
    return;
  }

  const logs = state.trace.audit_logs || [];
  panel.append(panelHead('审计记录', `${logs.length} 条`));
  if (logs.length === 0) {
    panel.append(emptyPanel('还没有审计记录', '确认一次写操作后，这里会出现 before/after 快照与执行结果。'));
    return;
  }

  for (const log of logs) {
    const diff = h('div', { class: 'audit-diff' });
    if (log.before) diff.append(h('div', {}, h('div', { class: 'tag' }, 'before'), h('pre', {}, prettyJSON(log.before))));
    if (log.after) diff.append(h('div', {}, h('div', { class: 'tag' }, 'after'), h('pre', {}, prettyJSON(log.after))));

    panel.append(h('div', { class: 'audit-row' },
      h('div', { class: 'audit-top' },
        h('span', { class: 'audit-action' }, log.action_type),
        statusChip(log.result, log.result),
      ),
      h('div', { class: 'audit-meta' }, `${log.actor_type} · ${log.target_type} · ${fmtTime(log.created_at, true)}`),
      log.reason ? h('div', { class: 'audit-meta' }, `原因：${log.reason}`) : null,
      diff.childElementCount ? diff : null,
    ));
  }
}

function prettyJSON(raw) {
  if (!raw) return '{}';
  if (typeof raw === 'object') return JSON.stringify(raw, null, 2);
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return String(raw);
  }
}
