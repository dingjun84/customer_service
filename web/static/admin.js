/* ==========================================================================
   客服运营后台 —— 原生 JS，无构建步骤。
   所有用户可控文本一律走 textContent，绝不拼 innerHTML：会话正文来自终端
   用户，是最典型的注入入口。
   ========================================================================== */
'use strict';

const API = '/api/admin/v1';
const THEME_KEY = 'admin.theme';

const state = {
  user: null,
  meta: {},
  overview: null,
  route: 'overview',
  params: new URLSearchParams(),
  seq: 0,
};

/* ------------------------------------------------------------------ utils */

const $ = (sel) => document.querySelector(sel);

function h(tag, attrs, ...kids) {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [key, value] of Object.entries(attrs)) {
      if (value === null || value === undefined || value === false) continue;
      if (key === 'class') node.className = value;
      else if (key === 'html') node.innerHTML = value;
      else if (key === 'text') node.textContent = value;
      else if (key === 'value') node.value = value;
      else if (key.startsWith('on') && typeof value === 'function') {
        node.addEventListener(key.slice(2).toLowerCase(), value);
      } else if (value === true) node.setAttribute(key, '');
      else node.setAttribute(key, value);
    }
  }
  appendKids(node, kids);
  return node;
}

function appendKids(node, kids) {
  for (const kid of kids) {
    if (kid === null || kid === undefined || kid === false) continue;
    if (Array.isArray(kid)) appendKids(node, kid);
    else if (kid instanceof Node) node.appendChild(kid);
    else node.appendChild(document.createTextNode(String(kid)));
  }
}

function pad(n) { return String(n).padStart(2, '0'); }

function fmtTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fmtFull(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function fmtRelative(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const mins = Math.round((Date.now() - d.getTime()) / 60000);
  if (mins < 0) {
    const ahead = Math.abs(mins);
    if (ahead < 60) return `${ahead} 分钟后`;
    if (ahead < 1440) return `${Math.round(ahead / 60)} 小时后`;
    return `${Math.round(ahead / 1440)} 天后`;
  }
  if (mins < 1) return '刚刚';
  if (mins < 60) return `${mins} 分钟前`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} 小时前`;
  return `${Math.round(hours / 24)} 天前`;
}

function ago(hours) {
  if (!hours || hours < 0) return '—';
  if (hours < 1) return '不到 1 小时';
  if (hours < 24) return `${hours} 小时`;
  return `${Math.floor(hours / 24)} 天 ${hours % 24} 小时`;
}

function pct(v) {
  if (v === null || v === undefined || Number.isNaN(v)) return '—';
  return `${Number(v).toFixed(1)}%`;
}

function num(v) {
  if (v === null || v === undefined) return '—';
  return Number(v).toLocaleString('zh-CN');
}

function parseJSON(raw) {
  if (!raw) return null;
  try { return JSON.parse(raw); } catch { return raw; }
}

function prettyJSON(raw) {
  const v = parseJSON(raw);
  if (v === null) return '';
  return typeof v === 'string' ? v : JSON.stringify(v, null, 2);
}

function debounce(fn, ms) {
  let timer = null;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

/* ----------------------------------------------------------------- labels */

const LABELS = {
  open: '进行中', closed: '已关闭', transferred: '已转人工',
  assigned: '处理中', pending_customer: '等客户回复', resolved: '已解决',
  low: '低', normal: '普通', high: '高', urgent: '紧急',
  human_transfer: '转人工', complaint: '投诉', quality_issue: '质量问题', other: '其他',
  submitted: '已提交', pending_review: '待审核', approved: '已批准', rejected: '已拒绝',
  awaiting_customer: '等客户', awaiting_return_shipment: '等退货', processing: '处理中',
  completed: '已完成', cancelled: '已取消',
  requested: '已申请', succeeded: '已成功', failed: '失败', partially_refunded: '部分退款',
  issued: '已开具', voided: '已作废', red_flushed: '已冲红',
  pending_payment: '待支付', paid: '已支付', shipped: '已发货',
  pending: '待确认', confirmed: '已确认', expired: '已过期', executed: '已执行',
  draft: '草稿', published: '已发布', archived: '已归档',
  active: '启用', inactive: '停用', discontinued: '已下架', out_of_stock: '缺货',
  started: '执行中', available: '可用', used: '已使用', disabled: '已停用',
  assistant: 'AI 助手', customer: '客户', human_agent: '人工客服', system: '系统', admin: '管理员',
  return: '退货', exchange: '换货', repair: '维修', refund: '退款', reship: '补发',
  price_protection: '价保',
  cancel_order: '取消订单', modify_order: '修改订单', submit_service_request: '提交售后',
  delivered: '已签收', in_transit: '运输中', delivering: '派送中', picked_up: '已揽收',
  exception: '异常', returned: '已退回', intercepted: '已拦截', delivery_failed: '派送失败',
  draft_status: '草稿', none: '无', electronic: '电子', paper: '纸质', vat_special: '增值税专票',
  positive: '正面', negative: '负面', neutral: '中性',
  wechat_pay: '微信支付', alipay: '支付宝', bank_card: '银行卡', cash_on_delivery: '货到付款',
  whitebar: '白条', account_balance: '余额', mixed: '组合支付',
  coupon: '优惠券', points: '积分', freight_insurance: '退货运费险', whitebar_status: '白条状态',
  store_balance: '店铺余额', void_pending_action: '作废待确认', ticket_update: '工单更新',
  knowledge_create: '新建知识', knowledge_update: '更新知识', knowledge_status: '知识状态',
  request_order_cancellation: '申请取消订单',
};

function label(v) {
  if (v === null || v === undefined || v === '') return '—';
  return LABELS[v] || v;
}

const TONE_GOOD = new Set(['succeeded', 'resolved', 'completed', 'closed', 'executed', 'published',
  'active', 'delivered', 'available', 'issued']);
const TONE_BAD = new Set(['failed', 'rejected', 'cancelled', 'expired', 'exception', 'urgent',
  'delivery_failed', 'returned', 'intercepted', 'unresolved']);
const TONE_WARN = new Set(['pending', 'pending_review', 'processing', 'assigned', 'draft',
  'started', 'pending_customer', 'awaiting_customer', 'awaiting_return_shipment',
  'requested', 'in_transit', 'delivering', 'partially_refunded', 'out_of_stock']);

function statusTone(v) {
  if (TONE_GOOD.has(v)) return 'good';
  if (TONE_BAD.has(v)) return 'bad';
  if (TONE_WARN.has(v)) return 'warn';
  return 'muted';
}

function badge(v, tone) {
  const t = tone || statusTone(v);
  return h('span', { class: `badge tone-${t}` },
    h('span', { class: 'badge-dot' }), label(v));
}

function priorityBadge(v) {
  const tone = v === 'urgent' || v === 'high' ? 'bad' : v === 'normal' ? 'muted' : 'warn';
  return h('span', { class: `badge tone-${tone}` }, label(v));
}

/* -------------------------------------------------------------------- api */

class ApiError extends Error {
  constructor(message, detail) {
    super(message);
    this.detail = detail || {};
  }
}

async function api(path, options = {}) {
  const init = {
    method: options.method || 'GET',
    credentials: 'same-origin',
    headers: { 'Accept': 'application/json' },
  };
  if (options.body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(options.body);
  }

  let res;
  try {
    res = await fetch(API + path, init);
  } catch (err) {
    throw new ApiError('网络异常，无法连接服务');
  }

  if (res.status === 401) {
    showLogin('登录已失效，请重新登录');
    throw new ApiError('未登录');
  }

  let payload = null;
  try { payload = await res.json(); } catch { payload = null; }

  if (!res.ok) {
    const detail = (payload && payload.error) || {};
    throw new ApiError(detail.message || `请求失败（${res.status}）`, detail);
  }
  return payload || {};
}

/* ------------------------------------------------------------- login/shell */

let loginHintShown = false;

function showLogin(message) {
  $('#app').hidden = true;
  $('#login').hidden = false;
  $('#drawer').hidden = true;
  $('#modal').hidden = true;
  const err = $('#login-error');
  if (message) {
    err.textContent = message;
    err.hidden = false;
  } else {
    err.hidden = true;
  }
  const hint = $('#login-hint');
  if (state.meta && state.meta.dev) {
    hint.hidden = false;
    hint.textContent = '内置账号：admin / admin12345（仅开发环境）';
  } else {
    hint.hidden = true;
  }
  const username = $('#login-username');
  if (!username.value) username.focus(); else $('#login-password').focus();
}

function showApp() {
  $('#login').hidden = true;
  $('#app').hidden = false;
}

async function doLogin(event) {
  event.preventDefault();
  const submit = $('#login-submit');
  const err = $('#login-error');
  err.hidden = true;
  submit.disabled = true;
  submit.textContent = '登录中…';
  try {
    const res = await api('/auth/login', {
      method: 'POST',
      body: { username: $('#login-username').value.trim(), password: $('#login-password').value },
    });
    state.user = res.data.user;
    $('#login-password').value = '';
    await bootSession();
    showApp();
    navigate();
  } catch (e) {
    err.textContent = e.message;
    err.hidden = false;
  } finally {
    submit.disabled = false;
    submit.textContent = '登录';
  }
}

async function bootSession() {
  const me = await api('/auth/me');
  state.user = me.data.user;
  state.meta = me.data.meta || {};
  $('#who-name').textContent = state.user.display_name || state.user.username;
  $('#who-role').textContent = label(state.user.role);
  $('#who-avatar').textContent = (state.user.display_name || state.user.username || 'A').slice(0, 1);
  const env = state.meta.app_env || '';
  const llm = state.meta.llm_mode === 'offline' ? '离线演示模型'
    : state.meta.llm_mode === 'openai' ? '真实模型' : '模型未启用';
  $('#brand-sub').textContent = `${env} · ${llm}`;
  $('#brand-sub').title = `schema v${state.meta.schema_version} · ${llm}`;
}

async function doLogout() {
  try { await api('/auth/logout', { method: 'POST', body: {} }); } catch { /* 已失效也照样退出 */ }
  state.user = null;
  showLogin();
}

/* ------------------------------------------------------------------ theme */

function applyTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  if (saved === 'light' || saved === 'dark') {
    document.documentElement.setAttribute('data-theme', saved);
  } else {
    document.documentElement.removeAttribute('data-theme');
  }
  updateThemeButton();
}

function cycleTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  const next = saved === null ? 'dark' : saved === 'dark' ? 'light' : null;
  if (next) localStorage.setItem(THEME_KEY, next);
  else localStorage.removeItem(THEME_KEY);
  applyTheme();
}

function updateThemeButton() {
  const saved = localStorage.getItem(THEME_KEY);
  const btn = $('#theme-toggle');
  if (btn) btn.textContent = saved === null ? '跟随系统' : saved === 'dark' ? '深色' : '浅色';
}

/* ------------------------------------------------------------------ icons */

const ICONS = {
  overview: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="2" y="2" width="5" height="5" rx="1"/><rect x="9" y="2" width="5" height="5" rx="1"/><rect x="2" y="9" width="5" height="5" rx="1"/><rect x="9" y="9" width="5" height="5" rx="1"/></svg>',
  chat: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M14 8.5c0 2.5-2.7 4.5-6 4.5-.8 0-1.6-.1-2.3-.3L2 14l1.2-3.2C2.4 9.9 2 9.2 2 8.5 2 6 4.7 4 8 4s6 2 6 4.5z"/></svg>',
  ticket: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M2 5.5A1.5 1.5 0 013.5 4h9A1.5 1.5 0 0114 5.5v1a1.5 1.5 0 000 3v1A1.5 1.5 0 0112.5 12h-9A1.5 1.5 0 012 10.5v-1a1.5 1.5 0 000-3z"/><path d="M8 4v8" stroke-dasharray="1.5 1.5"/></svg>',
  shield: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M8 2l5 2v4c0 3-2 5-5 6-3-1-5-3-5-6V4z"/><path d="M5.8 8l1.5 1.5 3-3"/></svg>',
  tool: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M9.5 2.5a3 3 0 104 4L8 12l-2.5.5L5 15l-2-2 .5-2.5L9.5 2.5z"/></svg>',
  chart: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M2 13h12"/><rect x="3" y="8" width="2.5" height="4"/><rect x="7" y="3" width="2.5" height="9"/><rect x="11" y="6" width="2.5" height="6"/></svg>',
  book: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M2.5 3.5A1.5 1.5 0 014 2h9v11H4a1.5 1.5 0 00-1.5 1.5z"/><path d="M2.5 3.5v11"/></svg>',
  users: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><circle cx="6" cy="5.5" r="2.2"/><path d="M2 13c0-2 1.8-3.2 4-3.2s4 1.2 4 3.2"/><path d="M11 4.2a2 2 0 010 3.6M12.5 13c0-1.4-.5-2.4-1.4-3"/></svg>',
  box: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M8 2l5.5 2.7v6.6L8 14 2.5 11.3V4.7z"/><path d="M2.5 4.7L8 7.4l5.5-2.7M8 7.4V14"/></svg>',
  cart: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M2 3h1.6l1.6 7h7l1.3-5H5"/><circle cx="6.5" cy="12.5" r="1"/><circle cx="11.5" cy="12.5" r="1"/></svg>',
  refund: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M3 8a5 5 0 105-5H5"/><path d="M5 1.5L3 3l2 1.5"/><path d="M8 6v4M6.5 7.2h2.2a1 1 0 010 2H7"/></svg>',
  audit: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="3" y="2" width="10" height="12" rx="1.5"/><path d="M5.5 5.5h5M5.5 8h5M5.5 10.5h3"/></svg>',
  gear: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><circle cx="8" cy="8" r="2.2"/><path d="M8 1.8v1.6M8 12.6v1.6M1.8 8h1.6M12.6 8h1.6M3.6 3.6l1.1 1.1M11.3 11.3l1.1 1.1M12.4 3.6l-1.1 1.1M4.7 11.3l-1.1 1.1"/></svg>',
  search: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14"/></svg>',
};

/* --------------------------------------------------------------- components */

function loadingBlock() {
  return h('div', { class: 'loading-block' },
    h('div', { class: 'skeleton', style: 'width:38%' }),
    h('div', { class: 'skeleton', style: 'width:72%' }),
    h('div', { class: 'skeleton', style: 'width:56%' }),
    h('div', { class: 'skeleton', style: 'width:64%' }));
}

function emptyState(title, hint) {
  return h('div', { class: 'empty' },
    h('strong', null, title),
    hint ? h('span', null, hint) : null);
}

function errorBlock(message) {
  return h('div', { class: 'error-block' }, message);
}

function table(cols, rows, options = {}) {
  const head = h('thead', null, h('tr', null,
    cols.map((c) => h('th', { class: c.thClass || '' }, c.label))));

  const body = h('tbody');
  if (!rows.length) {
    body.appendChild(h('tr', null,
      h('td', { colspan: cols.length, style: 'padding:0' },
        emptyState(options.emptyTitle || '没有数据', options.emptyHint || ''))));
  } else {
    for (const row of rows) {
      const tr = h('tr', { class: options.onRow ? 'clickable' : '' });
      if (options.onRow) {
        tr.addEventListener('click', () => options.onRow(row));
      }
      for (const c of cols) {
        const value = c.render ? c.render(row) : row[c.key];
        tr.appendChild(h('td', { class: c.cellClass || '' }, value === null || value === undefined ? '—' : value));
      }
      body.appendChild(tr);
    }
  }
  return h('div', { class: 'table-wrap' }, h('table', { class: 'table' }, head, body));
}

function pager(meta, onGo) {
  if (!meta || !meta.total) return null;
  const page = meta.page || 1;
  const pages = meta.total_pages || 1;
  const from = (page - 1) * (meta.page_size || 20) + 1;
  const to = Math.min(page * (meta.page_size || 20), meta.total);

  return h('div', { class: 'pager' },
    h('span', null, `共 ${num(meta.total)} 条 · 第 ${from}–${to} 条`),
    h('div', { class: 'pager-btns' },
      h('button', {
        class: 'btn btn-ghost btn-sm', type: 'button', disabled: page <= 1,
        onclick: () => onGo(page - 1),
      }, '上一页'),
      h('span', { class: 'pager-page' }, `${page} / ${pages}`),
      h('button', {
        class: 'btn btn-ghost btn-sm', type: 'button', disabled: page >= pages,
        onclick: () => onGo(page + 1),
      }, '下一页')));
}

function searchBox(value, onSearch) {
  const input = h('input', {
    type: 'search', placeholder: '搜索…', value: value || '',
    oninput: debounce((e) => {
      state.restoreSearch = true;
      onSearch(e.target.value.trim());
    }, 320),
  });
  input.dataset.searchBox = '1';
  return h('div', { class: 'search' }, h('span', { html: ICONS.search }), input);
}

function selectFilter(value, options, onChange) {
  const sel = h('select', { class: 'filter', onchange: (e) => onChange(e.target.value) },
    options.map((o) => h('option', { value: o.value, selected: String(value || '') === String(o.value) }, o.label)));
  return sel;
}

function toolbar(...children) {
  return h('div', { class: 'toolbar' }, children);
}

function panel(title, ...children) {
  return h('section', { class: 'card' },
    title ? h('div', { class: 'card-head' }, h('h2', null, title)) : null,
    h('div', { class: 'card-body' }, ...children));
}

function kv(pairs) {
  const dl = h('dl', { class: 'kv' });
  for (const [key, value, cls] of pairs) {
    if (value === null || value === undefined || value === '') continue;
    dl.appendChild(h('dt', null, key));
    dl.appendChild(h('dd', { class: cls || '' }, value instanceof Node ? value : String(value)));
  }
  return dl;
}

function drawerSection(title, ...children) {
  return h('div', { class: 'panel' },
    h('h3', { class: 'panel-title' }, title),
    ...children);
}

/* ------------------------------------------------------------------ toast */

let toastTimer = null;
function toast(message, bad) {
  const el = $('#toast');
  el.textContent = message;
  el.className = bad ? 'toast bad' : 'toast';
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 2600);
}

/* ------------------------------------------------------------------ modal */

function openModal(title, body, foot) {
  $('#modal-title').textContent = title;
  $('#modal-body').replaceChildren(body);
  $('#modal-foot').replaceChildren(...(foot || []));
  $('#modal').hidden = false;
}

function closeModal() {
  $('#modal').hidden = true;
  $('#modal-body').replaceChildren();
  $('#modal-foot').replaceChildren();
}

function confirmModal(title, message, confirmLabel) {
  return new Promise((resolve) => {
    const done = (value) => { closeModal(); resolve(value); };
    openModal(title, h('p', { style: 'margin:0;line-height:1.7;color:var(--text-2)' }, message), [
      h('button', { class: 'btn btn-ghost', type: 'button', onclick: () => done(false) }, '取消'),
      h('button', { class: 'btn btn-danger', type: 'button', onclick: () => done(true) }, confirmLabel || '确认'),
    ]);
  });
}

/* ----------------------------------------------------------------- drawer */

function openDrawer(title, sub, body) {
  $('#drawer-title').textContent = title;
  $('#drawer-sub').textContent = sub || '';
  $('#drawer-body').replaceChildren(body);
  $('#drawer').hidden = false;
}

function closeDrawer() {
  $('#drawer').hidden = true;
  $('#drawer-body').replaceChildren();
}

// Drawer loaders are async; the list page must not embed a Promise as a child
// node. Kick them off after the page renders and surface failures as a toast.
function openDrawerAsync(loader) {
  Promise.resolve().then(loader).catch((err) => {
    if (err && err.message === '未登录') return;
    toast((err && err.message) || '加载详情失败', true);
    closeDrawer();
  });
}

// drawerSlot triggers an async drawer load from inside a render tree and always
// returns null, so nothing is appended to the DOM.
function drawerSlot(loader) {
  openDrawerAsync(loader);
  return null;
}

/* ----------------------------------------------------------------- router */

const ROUTES = [
  {
    group: '运营', items: [
      { id: 'overview', label: '概览', icon: 'overview', desc: '整体运行状况与待办积压', render: renderOverview },
      { id: 'conversations', label: '会话', icon: 'chat', desc: '跨客户查看全部对话与工具轨迹', render: renderConversations },
      { id: 'tickets', label: '工单', icon: 'ticket', desc: '转人工、投诉与质量问题处理队列', render: renderTickets },
      { id: 'pending-actions', label: '待确认动作', icon: 'shield', desc: '等待客户确认的取消/修改/售后提案', render: renderPendingActions },
    ],
  },
  {
    group: '质量', items: [
      { id: 'tools', label: '工具监控', icon: 'tool', desc: '工具调用成功率、耗时与失败码', render: renderTools },
      { id: 'analytics', label: '意图分析', icon: 'chart', desc: '意图分布、解决率与情绪', render: renderAnalytics },
      { id: 'knowledge', label: '知识库', icon: 'book', desc: '客服话术与政策知识的管理', render: renderKnowledge },
    ],
  },
  {
    group: '业务数据', items: [
      { id: 'customers', label: '客户', icon: 'users', desc: '客户档案、订单与权益', render: renderCustomers },
      { id: 'orders', label: '订单', icon: 'cart', desc: '订单、物流与金额明细', render: renderOrders },
      { id: 'products', label: '商品', icon: 'box', desc: '商品与 SKU 价格库存', render: renderProducts },
      { id: 'aftersales', label: '售后与财务', icon: 'refund', desc: '售后单、退款、发票与促销', render: renderAftersales },
    ],
  },
  {
    group: '治理', items: [
      { id: 'audit', label: '审计日志', icon: 'audit', desc: '所有写操作的前后快照', render: renderAudit },
      { id: 'system', label: '系统状态', icon: 'gear', desc: '数据规模、迁移版本与连接状态', render: renderSystem },
    ],
  },
];

const FLAT_ROUTES = ROUTES.flatMap((g) => g.items);

function buildNav() {
  const nav = $('#nav');
  nav.replaceChildren();
  for (const group of ROUTES) {
    nav.appendChild(h('div', { class: 'nav-group-label' }, group.group));
    for (const item of group.items) {
      nav.appendChild(h('a', {
        class: 'nav-item', href: `#/${item.id}`, 'data-route': item.id, title: item.label,
      }, h('span', { html: ICONS[item.icon] }), h('span', { class: 'nav-label' }, item.label)));
    }
  }
}

function setActiveNav(id) {
  document.querySelectorAll('.nav-item').forEach((el) => {
    el.classList.toggle('active', el.dataset.route === id);
  });
}

function parseHash() {
  const raw = location.hash.replace(/^#\/?/, '');
  const [path, qs] = raw.split('?');
  return { id: path || 'overview', params: new URLSearchParams(qs || '') };
}

function setParam(key, value, { replace } = {}) {
  const { id, params } = parseHash();
  if (value === null || value === undefined || value === '') params.delete(key);
  else params.set(key, String(value));
  if (key !== 'page') params.delete('page');
  const qs = params.toString();
  const next = `#/${id}${qs ? '?' + qs : ''}`;
  if (replace) history.replaceState(null, '', next);
  else location.hash = next;
  if (replace) navigate();
}

async function navigate() {
  const { id, params } = parseHash();
  state.params = params;
  const route = FLAT_ROUTES.find((r) => r.id === id) || FLAT_ROUTES[0];
  state.route = route.id;

  setActiveNav(route.id);
  $('#page-title').textContent = route.label;
  $('#page-desc').textContent = route.desc;
  document.title = `${route.label} · 客服运营后台`;

  const seq = ++state.seq;
  const view = $('#view');
  view.replaceChildren(loadingBlock());

  try {
    const node = await route.render(params);
    if (seq !== state.seq) return;
    view.replaceChildren(node);
    restoreSearchFocus();
  } catch (err) {
    if (seq !== state.seq) return;
    if (err.message === '未登录') return;
    view.replaceChildren(errorBlock(err.message));
  }
}

// Typing in the search box re-renders the page, which replaces the input node.
// Restore focus only when the navigation was caused by typing, otherwise opening
// a detail drawer would yank the caret back into the search field.
function restoreSearchFocus() {
  if (!state.restoreSearch) return;
  state.restoreSearch = false;
  const box = document.querySelector('[data-search-box]');
  if (!box) return;
  box.focus();
  const end = box.value.length;
  box.setSelectionRange(end, end);
}

/* --------------------------------------------------------------- overview */

async function renderOverview() {
  const res = await api('/overview');
  const ov = res.data;
  state.overview = ov;
  $('#generated-at').textContent = `生成于 ${fmtTime(ov.generated_at)}`;

  const metrics = h('div', { class: 'metrics' },
    ov.metrics.map((m) => h('a', {
      class: `metric tone-${m.tone || 'neutral'}`, href: m.link || '#/overview',
    },
      h('div', { class: 'metric-label' }, m.label),
      h('div', { class: 'metric-value' }, m.value),
      h('div', { class: 'metric-hint' }, m.hint || ''))));

  const queues = h('div', { class: 'queue' },
    ov.queues.map((q) => h('a', { class: 'queue-row', href: q.link || '#/overview' },
      h('span', { class: 'queue-label' }, q.label),
      q.overdue ? h('span', { class: 'queue-flag' }, `${q.overdue} 需关注`) : null,
      h('span', { class: `queue-num tone-${q.severity || ''}`, style: q.severity === 'bad' ? 'color:var(--danger)' : q.severity === 'warn' ? 'color:var(--warning)' : '' }, num(q.count)))));

  const activity = renderActivity(ov.activity);

  const intents = table([
    { key: 'intent_primary', label: '意图', cellClass: 'strong' },
    { label: '样本', cellClass: 'num', render: (r) => num(r.samples) },
    { label: '解决率', cellClass: 'num', render: (r) => pct(r.resolve_rate) },
    { label: '未解决', cellClass: 'num', render: (r) => num(r.unresolved) },
  ], ov.intents.slice(0, 6), { emptyTitle: '暂无标注数据' });

  const tools = table([
    { key: 'tool_name', label: '工具', cellClass: 'mono truncate' },
    { label: '调用', cellClass: 'num', render: (r) => num(r.calls) },
    { label: '成功率', cellClass: 'num', render: (r) => pct(r.success_rate) },
    { label: 'P95', cellClass: 'num', render: (r) => `${num(r.p95_latency_ms)}ms` },
  ], ov.top_tools, { emptyTitle: '暂无调用' });

  const failures = table([
    { label: '时间', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
    { key: 'tool_name', label: '工具', cellClass: 'mono' },
    { label: '状态', render: (r) => badge(r.status) },
    { key: 'error_code', label: '错误码', cellClass: 'mono' },
  ], ov.recent_failures, { emptyTitle: '近期没有失败调用' });

  return h('div', null,
    metrics,
    h('div', { class: 'grid-2 section-gap' },
      panel('近 14 天会话量', activity),
      panel('待办队列', queues)),
    h('div', { class: 'grid-2 section-gap' },
      panel('意图分布 Top 6', intents),
      panel('高频工具', tools)),
    h('div', { class: 'section-gap' },
      panel('最近的失败调用', failures)));
}

function renderActivity(points) {
  if (!points || !points.length) return emptyState('暂无数据');
  const max = Math.max(...points.map((p) => p.count), 1);
  const bars = points.map((p) => {
    const height = p.count === 0 ? 3 : Math.max(6, Math.round((p.count / max) * 66));
    return h('div', {
      class: `spark-bar${p.count === 0 ? ' zero' : ''}`,
      style: `height:${height}px`,
      title: `${p.date} · ${p.count} 个会话`,
    });
  });
  return h('div', null,
    h('div', { class: 'spark' }, bars),
    h('div', { class: 'spark-axis' },
      h('span', null, points[0].date),
      h('span', null, points[points.length - 1].date)));
}

/* ---------------------------------------------------------- conversations */

async function renderConversations(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const status = params.get('status') || '';
  const channel = params.get('channel') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (status) query.set('status', status);
  if (channel) query.set('channel', channel);

  const res = await api(`/conversations?${query}`);
  const rows = res.data;
  const meta = res.meta;

  const cols = [
    { key: 'conversation_no', label: '会话号', cellClass: 'mono strong nowrap' },
    {
      label: '客户', render: (r) => h('div', null,
        h('div', { class: 'row-title' }, r.customer_name || '未命名'),
        h('div', { class: 'muted', style: 'font-size:11px' }, r.customer_no)),
    },
    { label: '状态', render: (r) => badge(r.status) },
    { key: 'channel', label: '渠道', cellClass: 'nowrap' },
    { label: '消息', cellClass: 'num', render: (r) => num(r.message_count) },
    { label: '工具', cellClass: 'num', render: (r) => num(r.tool_count) },
    {
      label: '待确认', cellClass: 'num',
      render: (r) => r.pending_count > 0 ? h('span', { class: 'badge tone-bad' }, num(r.pending_count)) : '—',
    },
    { label: '最后消息', cellClass: 'nowrap', render: (r) => fmtRelative(r.last_message_at || r.created_at) },
  ];

  const openId = params.get('id');

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'open', label: '进行中' },
        { value: 'closed', label: '已关闭' },
        { value: 'transferred', label: '已转人工' },
      ], (v) => setParam('status', v)),
      selectFilter(channel, [
        { value: '', label: '全部渠道' },
        { value: 'web_demo', label: 'web_demo' },
      ], (v) => setParam('channel', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(meta.total)} 个会话`)),
    table(cols, rows, {
      emptyTitle: '没有匹配的会话',
      emptyHint: '试试调整筛选条件，或先在客户控制台发起一次对话',
      onRow: (r) => setParam('id', r.id),
    }),
    pager(meta, (p) => setParam('page', p)),
    openId ? drawerSlot(() => conversationDrawer(openId)) : null);
}

async function conversationDrawer(id) {
  const res = await api(`/conversations/${encodeURIComponent(id)}`);
  const d = res.data;
  const c = d.conversation;

  openDrawer(c.conversation_no, `${c.customer_name || c.customer_no} · ${label(c.status)} · ${c.channel}`, h('div', null,
    drawerSection('概览', kv([
      ['客户', `${c.customer_name || '未命名'}（${c.customer_no}）`],
      ['状态', badge(c.status)],
      ['渠道', c.channel],
      ['消息数', num(c.message_count)],
      ['工具调用', num(c.tool_count)],
      ['待确认', num(c.pending_count)],
      ['创建时间', fmtFull(c.created_at)],
      ['最后消息', fmtFull(c.last_message_at)],
      ['摘要', c.summary || '—'],
    ])),
    drawerSection(`对话记录（${d.messages.length}）`, renderMessages(d.messages)),
    d.tool_calls.length ? drawerSection(`工具调用（${d.tool_calls.length}）`, renderToolCalls(d.tool_calls)) : null,
    d.pending_actions.length ? drawerSection('待确认动作', renderPendingList(d.pending_actions)) : null,
    d.orders.length ? drawerSection('关联订单', renderOrderMini(d.orders)) : null,
    d.audit_logs.length ? drawerSection('审计记录', renderAuditMini(d.audit_logs)) : null));

  return null;
}

function renderMessages(messages) {
  if (!messages.length) return emptyState('没有消息');
  return h('div', null, messages.map((m) => {
    const tags = [];
    if (m.intent_primary) tags.push(badge(m.intent_primary, 'info'));
    if (m.intent_secondary) tags.push(badge(m.intent_secondary, 'muted'));
    if (m.sentiment) tags.push(badge(m.sentiment, m.sentiment === 'negative' ? 'bad' : m.sentiment === 'positive' ? 'good' : 'muted'));
    if (m.resolution_status) tags.push(badge(m.resolution_status));
    if (m.confidence !== undefined && m.confidence !== null) {
      tags.push(h('span', { class: 'badge tone-muted' }, `置信 ${Number(m.confidence).toFixed(2)}`));
    }

    const content = m.role === 'tool'
      ? h('div', null,
        h('div', { style: 'font-family:var(--mono);font-size:11px;color:var(--muted);margin-bottom:4px' },
          m.tool_name || 'tool'),
        h('div', { style: 'white-space:pre-wrap' }, m.content || '（无文本，见入参/结果）'),
        m.tool_arguments ? h('details', { class: 'raw' },
          h('summary', null, '入参'),
          h('pre', { class: 'code' }, prettyJSON(m.tool_arguments))) : null,
        m.tool_result ? h('details', { class: 'raw' },
          h('summary', null, '结果'),
          h('pre', { class: 'code' }, prettyJSON(m.tool_result))) : null)
      : h('div', null, m.content || '（空消息）');

    return h('div', { class: `msg ${m.role}` },
      h('div', { class: 'msg-role' }, m.role),
      h('div', null,
        h('div', { class: 'msg-bubble' }, content),
        h('div', { class: 'row', style: 'margin-top:4px' },
          h('span', { class: 'faint', style: 'font-size:10px' }, fmtFull(m.created_at)),
          tags.length ? h('div', { class: 'msg-tags' }, tags) : null)));
  }));
}

function renderToolCalls(calls) {
  return table([
    { label: '时间', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
    { key: 'tool_name', label: '工具', cellClass: 'mono truncate' },
    { label: '状态', render: (r) => badge(r.status) },
    { label: '耗时', cellClass: 'num', render: (r) => r.latency_ms === null ? '—' : `${num(r.latency_ms)}ms` },
    { key: 'error_code', label: '错误码', cellClass: 'mono' },
  ], calls, { emptyTitle: '没有工具调用' });
}

function renderPendingList(items) {
  return h('div', { class: 'stack' }, items.map((a) => h('div', { class: 'card', style: 'padding:10px 12px' },
    h('div', { class: 'row' },
      h('span', { class: 'mono', style: 'font-size:11px;color:var(--muted)' }, a.action_no),
      badge(a.status),
      a.is_overdue ? h('span', { class: 'badge tone-bad' }, '已过期') : null,
      h('span', { class: 'spacer' }),
      h('span', { class: 'faint', style: 'font-size:11px' }, fmtTime(a.created_at))),
    h('div', { style: 'margin-top:6px;font-size:12px' }, label(a.action_type)),
    a.summary ? h('div', { class: 'muted', style: 'margin-top:4px;font-size:12px' }, a.summary) : null)));
}

function renderOrderMini(orders) {
  return table([
    { key: 'order_no', label: '订单号', cellClass: 'mono' },
    { label: '状态', render: (r) => badge(r.order_status) },
    { label: '金额', cellClass: 'num', render: (r) => r.paid_display },
    { label: '下单', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
  ], orders, { emptyTitle: '没有订单' });
}

function renderAuditMini(logs) {
  return h('div', { class: 'timeline' }, logs.map((l) => h('div', {
    class: `tl-item ${l.result === 'succeeded' ? 'good' : l.result === 'failed' ? 'bad' : ''}`,
  },
    h('div', { class: 'tl-meta' }, `${fmtFull(l.created_at)} · ${label(l.actor_type)}`),
    h('div', { class: 'tl-body' }, `${label(l.action_type)} → ${l.target_type} ${l.target_id.slice(0, 8)}`,
      l.reason ? `\n${l.reason}` : ''))));
}

/* --------------------------------------------------------------- tickets */

const TICKET_NEXT = {
  open: ['assigned', 'pending_customer', 'resolved', 'closed'],
  assigned: ['pending_customer', 'resolved', 'closed'],
  pending_customer: ['assigned', 'resolved', 'closed'],
  resolved: ['closed', 'open'],
  closed: ['open'],
};

async function renderTickets(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const status = params.get('status') || '';
  const type = params.get('type') || '';
  const priority = params.get('priority') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (status) query.set('status', status);
  if (type) query.set('type', type);
  if (priority) query.set('priority', priority);

  const res = await api(`/tickets?${query}`);
  const openId = params.get('id');

  const cols = [
    { key: 'ticket_no', label: '工单号', cellClass: 'mono strong nowrap' },
    { label: '类型', render: (r) => label(r.ticket_type) },
    { label: '优先级', render: (r) => priorityBadge(r.priority) },
    { label: '状态', render: (r) => badge(r.status) },
    { label: '客户', render: (r) => h('div', null,
        h('div', { class: 'row-title' }, r.customer_name || r.customer_no),
        h('div', { class: 'muted', style: 'font-size:11px' }, r.order_no || '未关联订单')) },
    { key: 'summary', label: '摘要', cellClass: 'truncate' },
    { label: '已等待', cellClass: 'nowrap', render: (r) => ago(r.age_hours) },
    { key: 'assigned_to', label: '处理人', cellClass: 'nowrap' },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'open', label: '待处理' },
        { value: 'assigned', label: '处理中' },
        { value: 'pending_customer', label: '等客户' },
        { value: 'resolved', label: '已解决' },
        { value: 'closed', label: '已关闭' },
      ], (v) => setParam('status', v)),
      selectFilter(type, [
        { value: '', label: '全部类型' },
        { value: 'human_transfer', label: '转人工' },
        { value: 'complaint', label: '投诉' },
        { value: 'quality_issue', label: '质量问题' },
        { value: 'other', label: '其他' },
      ], (v) => setParam('type', v)),
      selectFilter(priority, [
        { value: '', label: '全部优先级' },
        { value: 'urgent', label: '紧急' },
        { value: 'high', label: '高' },
        { value: 'normal', label: '普通' },
        { value: 'low', label: '低' },
      ], (v) => setParam('priority', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 张`)),
    table(cols, res.data, {
      emptyTitle: '队列是空的', emptyHint: '没有符合筛选条件的工单',
      onRow: (r) => setParam('id', r.ticket_no),
    }),
    pager(res.meta, (p) => setParam('page', p)),
    openId ? drawerSlot(() => ticketDrawer(openId)) : null);
}

async function ticketDrawer(ticketNo) {
  const res = await api(`/tickets/${encodeURIComponent(ticketNo)}`);
  const d = res.data;
  const t = d.ticket;

  const noteInput = h('textarea', { rows: 3, placeholder: '记录这次处理的结论、承诺或下一步…' });

  const statusButtons = (TICKET_NEXT[t.status] || []).map((next) =>
    h('button', {
      class: 'btn btn-ghost btn-sm', type: 'button',
      onclick: async () => {
        try {
          await api(`/tickets/${encodeURIComponent(ticketNo)}`, {
            method: 'POST', body: { status: next },
          });
          toast(`已变更为「${label(next)}」`);
          ticketDrawer(ticketNo);
          navigate();
        } catch (e) { toast(e.message, true); }
      },
    }, `→ ${label(next)}`));

  const saveNote = async () => {
    const note = noteInput.value.trim();
    if (!note) { toast('请先填写处理记录', true); return; }
    try {
      await api(`/tickets/${encodeURIComponent(ticketNo)}/notes`, { method: 'POST', body: { note } });
      toast('已添加处理记录');
      ticketDrawer(ticketNo);
    } catch (e) { toast(e.message, true); }
  };

  openDrawer(t.ticket_no, `${label(t.ticket_type)} · ${label(t.status)} · ${t.customer_name || t.customer_no}`,
    h('div', null,
      drawerSection('工单信息', kv([
        ['编号', t.ticket_no, 'mono'],
        ['类型', label(t.ticket_type)],
        ['状态', badge(t.status)],
        ['优先级', priorityBadge(t.priority)],
        ['客户', `${t.customer_name || '未命名'}（${t.customer_no}）`],
        ['关联订单', t.order_no || '—'],
        ['处理人', t.assigned_to || '未指派'],
        ['创建时间', fmtFull(t.created_at)],
        ['更新时间', fmtFull(t.updated_at)],
        ['已等待', ago(t.age_hours)],
        ['摘要', t.summary],
      ])),
      drawerSection('状态流转', h('div', { class: 'row' },
        statusButtons.length ? statusButtons : h('span', { class: 'muted' }, '当前状态没有可用的流转'))),
      drawerSection('添加处理记录', h('div', { class: 'stack' },
        noteInput,
        h('div', { class: 'row' },
          h('span', { class: 'faint', style: 'font-size:11px' }, '写入后不可修改，作为处理留痕'),
          h('span', { class: 'spacer' }),
          h('button', { class: 'btn btn-primary btn-sm', type: 'button', onclick: saveNote }, '保存记录')))),
      drawerSection(`处理记录（${d.notes.length}）`, d.notes.length
        ? h('div', { class: 'timeline' }, d.notes.map((n) => h('div', { class: 'tl-item' },
            h('div', { class: 'tl-meta' }, `${fmtFull(n.created_at)} · ${n.author_name}` +
              (n.from_status !== n.to_status ? ` · ${label(n.from_status)} → ${label(n.to_status)}` : '')),
            h('div', { class: 'tl-body' }, n.note))))
        : emptyState('还没有处理记录')),
      d.audit_logs.length ? drawerSection('审计记录', renderAuditMini(d.audit_logs)) : null,
      t.conversation_id ? drawerSection('来源会话', h('button', {
        class: 'btn btn-ghost btn-sm', type: 'button',
        onclick: () => {
          closeDrawer();
          location.hash = `#/conversations?id=${t.conversation_id}`;
        },
      }, '打开对应对话')) : null));

  return null;
}

/* -------------------------------------------------------- pending actions */

async function renderPendingActions(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const status = params.get('status') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (status) query.set('status', status);

  const res = await api(`/pending-actions?${query}`);
  const openId = params.get('id');

  const cols = [
    { key: 'action_no', label: '动作号', cellClass: 'mono strong nowrap' },
    { label: '类型', render: (r) => label(r.action_type) },
    {
      label: '客户', render: (r) => h('div', null,
        h('div', { class: 'row-title' }, r.customer_name || r.customer_no),
        h('div', { class: 'muted', style: 'font-size:11px' }, r.target_type)),
    },
    { label: '状态', render: (r) => h('span', { class: 'row' }, badge(r.status),
        r.is_overdue ? h('span', { class: 'badge tone-bad' }, '已过期') : null) },
    { key: 'summary', label: '摘要', cellClass: 'truncate' },
    { label: '创建', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
    { label: '到期', cellClass: 'nowrap', render: (r) => fmtRelative(r.expires_at) },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'pending', label: '待确认' },
        { value: 'executed', label: '已执行' },
        { value: 'expired', label: '已过期' },
        { value: 'rejected', label: '已拒绝' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 条`)),
    table(cols, res.data, {
      emptyTitle: '没有待确认动作',
      emptyHint: 'AI 提出取消/修改/售后时会在这里生成一条待确认记录',
      onRow: (r) => setParam('id', r.id),
    }),
    pager(res.meta, (p) => setParam('page', p)),
    openId ? drawerSlot(() => pendingDrawer(openId)) : null);
}

async function pendingDrawer(id) {
  const res = await api(`/pending-actions/${encodeURIComponent(id)}`);
  const item = res.data;
  if (!item) { closeDrawer(); return; }

  const voidBtn = item.status === 'pending'
    ? h('button', {
        class: 'btn btn-danger btn-sm', type: 'button',
        onclick: async () => {
          const ok = await confirmModal('作废这条待确认动作？',
            '动作会被标记为已过期，客户将无法再确认执行。系统不会代替客户确认，也不会产生任何业务变更。', '作废');
          if (!ok) return;
          try {
            await api(`/pending-actions/${encodeURIComponent(id)}/void`, {
              method: 'POST', body: { reason: '运营后台手工作废' },
            });
            toast('已作废');
            closeDrawer();
            navigate();
          } catch (e) { toast(e.message, true); }
        },
      }, '作废该动作')
    : null;

  openDrawer(item.action_no, `${label(item.action_type)} · ${label(item.status)}`, h('div', null,
    drawerSection('动作信息', kv([
      ['动作号', item.action_no, 'mono'],
      ['类型', label(item.action_type)],
      ['状态', badge(item.status)],
      ['客户', `${item.customer_name || '未命名'}（${item.customer_no}）`],
      ['目标', `${item.target_type} / ${item.target_id}`, 'mono'],
      ['创建时间', fmtFull(item.created_at)],
      ['到期时间', fmtFull(item.expires_at)],
      ['确认时间', item.confirmed_at ? fmtFull(item.confirmed_at) : '—'],
      ['执行时间', item.executed_at ? fmtFull(item.executed_at) : '—'],
    ])),
    drawerSection('预览（客户看到的摘要）', h('pre', { class: 'code' }, prettyJSON(item.preview_json) || '—')),
    drawerSection('执行载荷', h('pre', { class: 'code' }, prettyJSON(item.payload_json) || '—')),
    item.result_json ? drawerSection('执行结果', h('pre', { class: 'code' }, prettyJSON(item.result_json))) : null,
    voidBtn ? drawerSection('运营操作', h('div', { class: 'stack' }, voidBtn,
      h('div', { class: 'faint', style: 'font-size:11px' },
        '后台只能作废，不能代替客户确认：确认动作代表客户本人的同意，伪造它会破坏「提案 + 确认」这条安全约束。'))) : null));

  return null;
}

/* ----------------------------------------------------------------- audit */

async function renderAudit(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const result = params.get('result') || '';
  const actor = params.get('actor_type') || '';

  const query = new URLSearchParams({ page, page_size: 25 });
  if (q) query.set('q', q);
  if (result) query.set('result', result);
  if (actor) query.set('actor_type', actor);

  const res = await api(`/audit-logs?${query}`);

  const cols = [
    { label: '时间', cellClass: 'nowrap mono', render: (r) => fmtTime(r.created_at) },
    { label: '操作', render: (r) => label(r.action_type) },
    { label: '目标', cellClass: 'mono truncate', render: (r) => `${r.target_type} ${r.target_id.slice(0, 8)}` },
    { label: '操作人', render: (r) => `${label(r.actor_type)}${r.actor_id ? ' ' + r.actor_id.slice(0, 6) : ''}` },
    { label: '结果', render: (r) => badge(r.result) },
    { key: 'reason', label: '原因', cellClass: 'truncate' },
    { label: '客户', cellClass: 'mono', render: (r) => r.customer_no || '—' },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(result, [
        { value: '', label: '全部结果' },
        { value: 'succeeded', label: '成功' },
        { value: 'failed', label: '失败' },
        { value: 'rejected', label: '已拒绝' },
      ], (v) => setParam('result', v)),
      selectFilter(actor, [
        { value: '', label: '全部操作人' },
        { value: 'assistant', label: 'AI 助手' },
        { value: 'customer', label: '客户' },
        { value: 'admin', label: '管理员' },
        { value: 'human_agent', label: '人工客服' },
        { value: 'system', label: '系统' },
      ], (v) => setParam('actor_type', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 条`)),
    table(cols, res.data, {
      emptyTitle: '没有审计记录',
      onRow: (r) => openAuditDrawer(r),
    }),
    pager(res.meta, (p) => setParam('page', p)));
}

function openAuditDrawer(row) {
  const before = prettyJSON(row.before_json);
  const after = prettyJSON(row.after_json);
  openDrawer(`审计 ${row.id.slice(0, 8)}`, `${label(row.action_type)} · ${fmtFull(row.created_at)}`, h('div', null,
    drawerSection('基本信息', kv([
      ['动作', label(row.action_type)],
      ['目标', `${row.target_type} / ${row.target_id}`, 'mono'],
      ['操作人', `${label(row.actor_type)}${row.actor_id ? ' / ' + row.actor_id : ''}`],
      ['结果', badge(row.result)],
      ['原因', row.reason || '—'],
      ['客户', row.customer_no || '—'],
      ['会话', row.conversation_id || '—', 'mono'],
      ['请求 ID', row.request_id || '—', 'mono'],
    ])),
    before ? drawerSection('变更前', h('pre', { class: 'code' }, before)) : null,
    after ? drawerSection('变更后', h('pre', { class: 'code' }, after)) : null));
}

/* ----------------------------------------------------------------- tools */

async function renderTools(params) {
  const tab = params.get('tab') === 'calls' ? 'calls' : 'stats';

  const tabs = h('div', { class: 'tabs' },
    h('button', { class: `tab ${tab === 'stats' ? 'active' : ''}`, type: 'button', onclick: () => setParam('tab', 'stats') }, '工具质量'),
    h('button', { class: `tab ${tab === 'calls' ? 'active' : ''}`, type: 'button', onclick: () => setParam('tab', 'calls') }, '调用明细'));

  if (tab === 'stats') {
    const res = await api('/analytics/tools');
    const rows = res.data;
    const cols = [
      { key: 'tool_name', label: '工具', cellClass: 'mono strong' },
      { label: '调用', cellClass: 'num', render: (r) => num(r.calls) },
      { label: '成功', cellClass: 'num', render: (r) => num(r.succeeded) },
      { label: '失败', cellClass: 'num', render: (r) => r.failed ? h('span', { style: 'color:var(--danger)' }, num(r.failed)) : '0' },
      { label: '被拒', cellClass: 'num', render: (r) => num(r.rejected) },
      {
        label: '成功率', cellClass: 'num',
        render: (r) => h('span', { style: r.success_rate < 90 ? 'color:var(--warning);font-weight:500' : '' }, pct(r.success_rate)),
      },
      { label: '平均耗时', cellClass: 'num', render: (r) => `${num(r.avg_latency_ms)}ms` },
      { label: 'P95', cellClass: 'num', render: (r) => `${num(r.p95_latency_ms)}ms` },
      { key: 'top_error_code', label: '主要错误码', cellClass: 'mono' },
    ];
    return h('section', { class: 'card' }, tabs,
      table(cols, rows, { emptyTitle: '还没有工具调用' }),
      h('div', { class: 'pager' }, h('span', null, `共 ${num(rows.length)} 个工具`)));
  }

  const page = params.get('page') || 1;
  const toolName = params.get('tool_name') || '';
  const status = params.get('status') || '';
  const query = new URLSearchParams({ page, page_size: 25 });
  if (toolName) query.set('tool_name', toolName);
  if (status) query.set('status', status);

  const res = await api(`/tool-executions?${query}`);

  const cols = [
    { label: '时间', cellClass: 'nowrap mono', render: (r) => fmtTime(r.created_at) },
    { key: 'tool_name', label: '工具', cellClass: 'mono' },
    { label: '状态', render: (r) => badge(r.status) },
    { label: '耗时', cellClass: 'num', render: (r) => r.latency_ms === null ? '—' : `${num(r.latency_ms)}ms` },
    { key: 'error_code', label: '错误码', cellClass: 'mono' },
    { key: 'error_message_safe', label: '错误信息', cellClass: 'truncate' },
    { label: '会话', cellClass: 'mono', render: (r) => r.conversation_id.slice(0, 8) },
  ];

  return h('section', { class: 'card' }, tabs,
    toolbar(
      selectFilter(toolName, [
        { value: '', label: '全部工具' },
        ...['get_order_detail', 'check_action_eligibility', 'search_knowledge', 'get_payment_refund_status',
          'get_service_status', 'get_logistics_status', 'get_invoice_status', 'get_product_detail',
          'transfer_to_human', 'create_complaint_ticket', 'request_order_cancellation',
          'get_customer_benefits_status', 'get_promotion_rules', 'list_my_orders', 'modify_order',
          'submit_service_request', 'search_products'].map((n) => ({ value: n, label: n })),
      ], (v) => setParam('tool_name', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'succeeded', label: '成功' },
        { value: 'failed', label: '失败' },
        { value: 'rejected', label: '被拒' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 次`)),
    table(cols, res.data, {
      emptyTitle: '没有调用记录',
      onRow: (r) => openDrawer(`工具调用 ${r.tool_name}`, fmtFull(r.created_at), h('div', null,
        drawerSection('结果', kv([
          ['工具', r.tool_name, 'mono'],
          ['状态', badge(r.status)],
          ['耗时', r.latency_ms === null ? '—' : `${num(r.latency_ms)}ms`],
          ['调用 ID', r.tool_call_id, 'mono'],
          ['错误码', r.error_code || '—', 'mono'],
          ['错误信息', r.error_message_safe || '—'],
        ])),
        drawerSection('入参', h('pre', { class: 'code' }, prettyJSON(r.input_json) || '—')),
        r.output_json ? drawerSection('结果', h('pre', { class: 'code' }, prettyJSON(r.output_json))) : null)),
    }),
    pager(res.meta, (p) => setParam('page', p)));
}

/* ------------------------------------------------------------- analytics */

async function renderAnalytics() {
  const res = await api('/analytics/intents');
  const rows = res.data;
  const total = rows.reduce((sum, r) => sum + r.samples, 0);
  const maxSamples = Math.max(...rows.map((r) => r.samples), 1);

  if (!rows.length) {
    return h('section', { class: 'card' }, emptyState('还没有意图标注', '标注数据来自离线评测与人工质检'));
  }

  const cols = [
    { key: 'intent_primary', label: '意图', cellClass: 'strong' },
    {
      label: '样本量', cellClass: 'num',
      render: (r) => h('div', { style: 'display:flex;align-items:center;gap:8px;justify-content:flex-end' },
        h('div', { style: `height:6px;width:${Math.round((r.samples / maxSamples) * 90)}px;background:var(--accent);border-radius:3px;opacity:.75` }),
        h('span', null, num(r.samples))),
    },
    { label: '占比', cellClass: 'num', render: (r) => pct((r.samples / total) * 100) },
    { label: '已解决', cellClass: 'num', render: (r) => num(r.resolved) },
    { label: '未解决', cellClass: 'num', render: (r) => r.unresolved ? h('span', { style: 'color:var(--warning)' }, num(r.unresolved)) : '0' },
    { label: '转人工', cellClass: 'num', render: (r) => num(r.transferred) },
    {
      label: '解决率', cellClass: 'num',
      render: (r) => h('span', { style: r.resolve_rate < 60 ? 'color:var(--danger);font-weight:500' : '' }, pct(r.resolve_rate)),
    },
    { label: '置信度', cellClass: 'num', render: (r) => Number(r.avg_confidence || 0).toFixed(2) },
    {
      label: '情绪', render: (r) => h('div', { class: 'row', style: 'gap:4px' },
        r.negative ? h('span', { class: 'badge tone-bad' }, `负 ${r.negative}`) : null,
        r.positive ? h('span', { class: 'badge tone-good' }, `正 ${r.positive}`) : null,
        r.neutral ? h('span', { class: 'badge tone-muted' }, `中 ${r.neutral}`) : null),
    },
  ];

  return h('section', { class: 'card' },
    h('div', { class: 'card-head' },
      h('h2', null, '意图与解决情况'),
      h('span', { class: 'hint' }, `共 ${num(total)} 条标注 · ${rows.length} 个意图`)),
    table(cols, rows, { emptyTitle: '没有标注数据' }));
}

/* ------------------------------------------------------------- knowledge */

async function renderKnowledge(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const category = params.get('category') || '';
  const status = params.get('status') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (category) query.set('category', category);
  if (status) query.set('status', status);

  const [res, cats] = await Promise.all([
    api(`/knowledge?${query}`),
    api('/knowledge/categories'),
  ]);

  const cols = [
    { key: 'article_code', label: '编码', cellClass: 'mono nowrap' },
    { key: 'title', label: '标题', cellClass: 'strong truncate' },
    { label: '分类', render: (r) => label(r.category) },
    { label: '状态', render: (r) => badge(r.status) },
    { label: '版本', cellClass: 'num', render: (r) => `v${r.version}` },
    { label: '更新', cellClass: 'nowrap', render: (r) => fmtRelative(r.updated_at) },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(category, [
        { value: '', label: '全部分类' },
        ...cats.data.map((c) => ({ value: c.category, label: `${label(c.category)}（${c.count}）` })),
      ], (v) => setParam('category', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'published', label: '已发布' },
        { value: 'draft', label: '草稿' },
        { value: 'archived', label: '已归档' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('button', {
        class: 'btn btn-primary btn-sm', type: 'button',
        onclick: () => openKnowledgeEditor(null),
      }, '新建文章')),
    table(cols, res.data, {
      emptyTitle: '还没有知识文章',
      emptyHint: '知识库会被 search_knowledge 工具检索，直接决定 AI 的答复口径',
      onRow: (r) => openKnowledgeDrawer(r.id),
    }),
    pager(res.meta, (p) => setParam('page', p)));
}

async function openKnowledgeDrawer(id) {
  const res = await api(`/knowledge/${encodeURIComponent(id)}`);
  const k = res.data;
  const keywords = parseJSON(k.keywords_json) || [];

  openDrawer(k.title, `${k.article_code} · ${label(k.category)} · v${k.version}`, h('div', null,
    drawerSection('操作', h('div', { class: 'row' },
      h('button', { class: 'btn btn-primary btn-sm', type: 'button', onclick: () => openKnowledgeEditor(k) }, '编辑'),
      k.status !== 'published' ? h('button', {
        class: 'btn btn-ghost btn-sm', type: 'button',
        onclick: () => changeKnowledgeStatus(k.id, 'published'),
      }, '发布') : null,
      k.status !== 'archived' ? h('button', {
        class: 'btn btn-ghost btn-sm', type: 'button',
        onclick: () => changeKnowledgeStatus(k.id, 'archived'),
      }, '归档') : null,
      k.status !== 'draft' ? h('button', {
        class: 'btn btn-quiet btn-sm', type: 'button',
        onclick: () => changeKnowledgeStatus(k.id, 'draft'),
      }, '转草稿') : null)),
    drawerSection('元信息', kv([
      ['编码', k.article_code, 'mono'],
      ['分类', label(k.category)],
      ['状态', badge(k.status)],
      ['版本', `v${k.version}`],
      ['来源', k.source || '—'],
      ['创建', fmtFull(k.created_at)],
      ['更新', fmtFull(k.updated_at)],
    ])),
    drawerSection('关键词', keywords.length
      ? h('div', { class: 'tag-list' }, keywords.map((w) => h('span', { class: 'badge tone-muted' }, w)))
      : h('span', { class: 'muted' }, '未设置关键词')),
    drawerSection('正文', h('div', { style: 'white-space:pre-wrap;font-size:12px;line-height:1.75' }, k.content))));
}

async function changeKnowledgeStatus(id, status) {
  try {
    await api(`/knowledge/${encodeURIComponent(id)}/status`, { method: 'POST', body: { status } });
    toast(`已${label(status)}`);
    closeDrawer();
    navigate();
  } catch (e) { toast(e.message, true); }
}

function openKnowledgeEditor(article) {
  const isEdit = !!article;
  const titleInput = h('input', { type: 'text', value: article ? article.title : '', maxlength: 200 });
  const categoryInput = h('input', {
    type: 'text', value: article ? article.category : '', maxlength: 40,
    placeholder: '如 order / logistics / after_sales',
  });
  const sourceInput = h('input', { type: 'text', value: article ? article.source || '' : 'admin_console', maxlength: 100 });
  const keywordsInput = h('input', {
    type: 'text', maxlength: 400,
    value: article ? (parseJSON(article.keywords_json) || []).join(', ') : '',
    placeholder: '用逗号分隔，如：退款,到账时间',
  });
  const statusSelect = h('select', null,
    [['draft', '草稿'], ['published', '发布'], ['archived', '归档']].map(([v, t]) =>
      h('option', { value: v, selected: article ? article.status === v : v === 'draft' }, t)));
  const contentInput = h('textarea', { rows: 12, value: article ? article.content : '' });

  const body = h('div', { class: 'form-grid' },
    h('label', { class: 'field' }, h('span', null, '标题'), titleInput),
    h('div', { class: 'row-2' },
      h('label', { class: 'field' }, h('span', null, '分类'), categoryInput),
      h('label', { class: 'field' }, h('span', null, '状态'), statusSelect)),
    h('div', { class: 'row-2' },
      h('label', { class: 'field' }, h('span', null, '来源'), sourceInput),
      h('label', { class: 'field' }, h('span', null, '关键词'), keywordsInput)),
    h('label', { class: 'field' }, h('span', null, '正文'), contentInput));

  const save = async () => {
    const payload = {
      title: titleInput.value.trim(),
      category: categoryInput.value.trim(),
      content: contentInput.value,
      source: sourceInput.value.trim(),
      status: statusSelect.value,
      keywords: keywordsInput.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
    };
    try {
      if (isEdit) {
        await api(`/knowledge/${encodeURIComponent(article.id)}`, { method: 'PUT', body: payload });
      } else {
        await api('/knowledge', { method: 'POST', body: payload });
      }
      toast(isEdit ? '已保存' : '已创建');
      closeModal();
      closeDrawer();
      navigate();
    } catch (e) { toast(e.message, true); }
  };

  openModal(isEdit ? '编辑知识文章' : '新建知识文章', body, [
    h('button', { class: 'btn btn-ghost', type: 'button', onclick: closeModal }, '取消'),
    h('button', { class: 'btn btn-primary', type: 'button', onclick: save }, isEdit ? '保存' : '创建'),
  ]);
}

/* ------------------------------------------------------------- customers */

async function renderCustomers(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const status = params.get('status') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (status) query.set('status', status);

  const res = await api(`/customers?${query}`);
  const openId = params.get('id');

  const cols = [
    { key: 'customer_no', label: '客户号', cellClass: 'mono strong nowrap' },
    { label: '姓名', render: (r) => h('div', null,
        h('div', { class: 'row-title' }, r.display_name || '未命名'),
        h('div', { class: 'muted', style: 'font-size:11px' }, r.phone_masked || '—')) },
    { label: '状态', render: (r) => badge(r.status) },
    { label: '订单', cellClass: 'num', render: (r) => num(r.order_count) },
    { label: '累计消费', cellClass: 'num', render: (r) => r.paid_display },
    { label: '会话', cellClass: 'num', render: (r) => num(r.conversation_count) },
    { label: '工单', cellClass: 'num', render: (r) => num(r.ticket_count) },
    { label: '注册', cellClass: 'nowrap', render: (r) => fmtFull(r.created_at) },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'active', label: '正常' },
        { value: 'disabled', label: '已停用' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 位`)),
    table(cols, res.data, {
      emptyTitle: '没有客户',
      onRow: (r) => setParam('id', r.customer_no),
    }),
    pager(res.meta, (p) => setParam('page', p)),
    openId ? drawerSlot(() => customerDrawer(openId)) : null);
}

async function customerDrawer(customerNo) {
  const res = await api(`/customers/${encodeURIComponent(customerNo)}`);
  const d = res.data;
  const c = d.customer;

  openDrawer(c.customer_no, `${c.display_name || '未命名'} · ${c.phone_masked || ''}`, h('div', null,
    drawerSection('档案', kv([
      ['客户号', c.customer_no, 'mono'],
      ['姓名', c.display_name || '未命名'],
      ['手机', c.phone_masked || '—'],
      ['状态', badge(c.status)],
      ['订单数', num(c.order_count)],
      ['累计消费', c.paid_display],
      ['会话数', num(c.conversation_count)],
      ['工单数', num(c.ticket_count)],
      ['注册时间', fmtFull(c.created_at)],
    ])),
    drawerSection(`订单（${d.orders.length}）`, renderOrderMini(d.orders)),
    drawerSection(`会话（${d.conversations.length}）`, d.conversations.length
      ? h('div', { class: 'stack' }, d.conversations.map((x) => h('button', {
          class: 'btn btn-ghost', type: 'button', style: 'justify-content:space-between',
          onclick: () => { closeDrawer(); location.hash = `#/conversations?id=${x.id}`; },
        },
          h('span', { class: 'mono', style: 'font-size:11px' }, x.conversation_no),
          h('span', { class: 'row', style: 'gap:6px' }, badge(x.status),
            h('span', { class: 'muted', style: 'font-size:11px' }, fmtRelative(x.last_message_at || x.created_at))))))
      : emptyState('没有会话')),
    drawerSection(`工单（${d.tickets.length}）`, d.tickets.length
      ? h('div', { class: 'stack' }, d.tickets.map((x) => h('button', {
          class: 'btn btn-ghost', type: 'button', style: 'justify-content:space-between',
          onclick: () => { closeDrawer(); location.hash = `#/tickets?id=${x.ticket_no}`; },
        },
          h('span', null, `${x.ticket_no} · ${label(x.ticket_type)}`),
          h('span', { class: 'row', style: 'gap:6px' }, priorityBadge(x.priority), badge(x.status)))))
      : emptyState('没有工单')),
    drawerSection('权益', d.benefits.length
      ? table([
          { label: '类型', render: (r) => label(r.benefit_type) },
          { label: '状态', render: (r) => badge(r.status) },
          { label: '面额', cellClass: 'num', render: (r) => r.amount_display || (r.points ? `${r.points} 分` : '—') },
          { label: '到期', cellClass: 'nowrap', render: (r) => r.expires_at ? fmtFull(r.expires_at) : '—' },
        ], d.benefits)
      : emptyState('没有权益记录'))));

  return null;
}

/* ---------------------------------------------------------------- orders */

async function renderOrders(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const status = params.get('status') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (status) query.set('status', status);

  const res = await api(`/orders?${query}`);
  const openId = params.get('id');

  const cols = [
    { key: 'order_no', label: '订单号', cellClass: 'mono strong nowrap' },
    { label: '客户', render: (r) => h('div', null,
        h('div', { class: 'row-title' }, r.customer_name || r.customer_no),
        h('div', { class: 'muted', style: 'font-size:11px' }, r.customer_no)) },
    { label: '状态', render: (r) => badge(r.order_status) },
    { label: '商品数', cellClass: 'num', render: (r) => num(r.item_count) },
    { label: '实付', cellClass: 'num', render: (r) => r.paid_display },
    { label: '已退款', cellClass: 'num', render: (r) => r.refunded ? r.refunded_display : '—' },
    { label: '渠道', cellClass: 'nowrap', render: (r) => r.order_channel },
    { label: '下单时间', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'pending_payment', label: '待支付' },
        { value: 'paid', label: '已支付' },
        { value: 'processing', label: '处理中' },
        { value: 'shipped', label: '已发货' },
        { value: 'completed', label: '已完成' },
        { value: 'cancelled', label: '已取消' },
        { value: 'closed', label: '已关闭' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 单`)),
    table(cols, res.data, {
      emptyTitle: '没有订单',
      onRow: (r) => setParam('id', r.order_no),
    }),
    pager(res.meta, (p) => setParam('page', p)),
    openId ? drawerSlot(() => orderDrawer(openId)) : null);
}

async function orderDrawer(orderNo) {
  const res = await api(`/orders/${encodeURIComponent(orderNo)}`);
  const d = res.data;
  const o = d.order;

  openDrawer(o.order_no, `${o.customer_name || o.customer_no} · ${label(o.order_status)}`, h('div', null,
    drawerSection('订单', kv([
      ['订单号', o.order_no, 'mono'],
      ['客户', `${o.customer_name || '未命名'}（${o.customer_no}）`],
      ['状态', badge(o.order_status)],
      ['渠道', o.order_channel],
      ['商品数', num(o.item_count)],
      ['实付', o.paid_display],
      ['已退款', o.refunded ? o.refunded_display : '—'],
      ['下单', fmtFull(o.created_at)],
      ['支付', o.paid_at ? fmtFull(o.paid_at) : '—'],
      ['预售', o.is_presale ? '是' : '否'],
    ])),
    drawerSection('收货信息', kv([
      ['收件人', d.receiver.name || '—'],
      ['电话', d.receiver.phone || '—'],
      ['地址', d.receiver.address || '—'],
      ['配送', label(d.receiver.method)],
      ['备注', d.receiver.remark || '—'],
    ])),
    drawerSection('商品明细', table([
      { key: 'product_name', label: '商品', cellClass: 'strong' },
      { key: 'sku_name', label: '规格' },
      { label: '数量', cellClass: 'num', render: (r) => num(r.quantity) },
      { label: '单价', cellClass: 'num', render: (r) => r.unit_display },
      { label: '已退', cellClass: 'num', render: (r) => r.refunded_quantity || '—' },
      { label: '状态', render: (r) => badge(r.item_status) },
    ], d.items)),
    drawerSection(`包裹（${d.shipments.length}）`, d.shipments.length
      ? table([
          { key: 'shipment_no', label: '包裹号', cellClass: 'mono' },
          { key: 'carrier_name', label: '承运商' },
          { key: 'tracking_no', label: '运单号', cellClass: 'mono' },
          { label: '状态', render: (r) => badge(r.shipment_status) },
          { key: 'latest_event', label: '最新节点', cellClass: 'truncate' },
        ], d.shipments)
      : emptyState('还没有包裹')),
    d.payments.length ? drawerSection('支付', table([
      { key: 'payment_no', label: '支付单号', cellClass: 'mono' },
      { label: '方式', render: (r) => label(r.payment_method) },
      { label: '状态', render: (r) => badge(r.payment_status) },
      { label: '金额', cellClass: 'num', render: (r) => r.amount_display },
      { label: '时间', cellClass: 'nowrap', render: (r) => fmtFull(r.paid_at || r.created_at) },
    ], d.payments)) : null,
    d.refunds.length ? drawerSection('退款', table([
      { key: 'refund_no', label: '退款号', cellClass: 'mono' },
      { label: '状态', render: (r) => badge(r.refund_status) },
      { label: '金额', cellClass: 'num', render: (r) => r.amount_display },
      { key: 'reason', label: '原因', cellClass: 'truncate' },
      { label: '申请', cellClass: 'nowrap', render: (r) => fmtTime(r.requested_at) },
    ], d.refunds)) : null,
    d.service_cases.length ? drawerSection('售后单', table([
      { key: 'case_no', label: '单号', cellClass: 'mono' },
      { label: '类型', render: (r) => label(r.case_type) },
      { label: '状态', render: (r) => badge(r.case_status) },
      { label: '数量', cellClass: 'num', render: (r) => num(r.requested_quantity) },
      { label: '创建', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
    ], d.service_cases)) : null,
    d.invoices.length ? drawerSection('发票', table([
      { label: '发票号', cellClass: 'mono', render: (r) => r.invoice_no || '—' },
      { label: '类型', render: (r) => label(r.invoice_type) },
      { label: '状态', render: (r) => badge(r.invoice_status) },
      { label: '金额', cellClass: 'num', render: (r) => r.amount_display },
      { key: 'title', label: '抬头', cellClass: 'truncate' },
    ], d.invoices)) : null,
    d.audit_logs.length ? drawerSection('审计记录', renderAuditMini(d.audit_logs)) : null));

  return null;
}

/* -------------------------------------------------------------- products */

async function renderProducts(params) {
  const page = params.get('page') || 1;
  const q = params.get('q') || '';
  const status = params.get('status') || '';

  const query = new URLSearchParams({ page, page_size: 20 });
  if (q) query.set('q', q);
  if (status) query.set('status', status);

  const res = await api(`/products?${query}`);
  const openId = params.get('id');

  const cols = [
    { key: 'product_code', label: '编码', cellClass: 'mono nowrap' },
    { key: 'name', label: '商品', cellClass: 'strong truncate' },
    { key: 'brand', label: '品牌' },
    { key: 'category', label: '分类', cellClass: 'nowrap' },
    { label: '状态', render: (r) => badge(r.status) },
    { label: 'SKU', cellClass: 'num', render: (r) => num(r.sku_count) },
    { label: '库存', cellClass: 'num', render: (r) => r.stock_total === 0
        ? h('span', { style: 'color:var(--danger)' }, '0')
        : num(r.stock_total) },
    { label: '价格', cellClass: 'num nowrap', render: (r) => r.price_range_display },
  ];

  return h('section', { class: 'card' },
    toolbar(
      searchBox(q, (v) => setParam('q', v)),
      selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'active', label: '在售' },
        { value: 'inactive', label: '停用' },
        { value: 'discontinued', label: '已下架' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 个商品`)),
    table(cols, res.data, {
      emptyTitle: '没有商品',
      onRow: (r) => setParam('id', r.id),
    }),
    pager(res.meta, (p) => setParam('page', p)),
    openId ? drawerSlot(() => productDrawer(openId)) : null);
}

async function productDrawer(id) {
  const res = await api(`/products/${encodeURIComponent(id)}`);
  const d = res.data;
  const p = d.product;

  openDrawer(p.name, `${p.product_code} · ${label(p.category)}`, h('div', null,
    drawerSection('商品', kv([
      ['编码', p.product_code, 'mono'],
      ['名称', p.name],
      ['品牌', p.brand || '—'],
      ['分类', label(p.category)],
      ['状态', badge(p.status)],
      ['SKU 数', num(p.sku_count)],
      ['库存合计', num(p.stock_total)],
      ['价格区间', p.price_range_display],
      ['创建', fmtFull(p.created_at)],
    ])),
    d.description ? drawerSection('描述', h('div', { style: 'white-space:pre-wrap;font-size:12px;line-height:1.7' }, d.description)) : null,
    drawerSection(`SKU（${d.skus.length}）`, table([
      { key: 'sku_code', label: 'SKU 编码', cellClass: 'mono' },
      { key: 'sku_name', label: '规格名', cellClass: 'strong' },
      { label: '价格', cellClass: 'num', render: (r) => r.list_price_display },
      { label: '库存', cellClass: 'num', render: (r) => r.stock_quantity === 0
          ? h('span', { style: 'color:var(--danger)' }, '0') : num(r.stock_quantity) },
      { label: '状态', render: (r) => badge(r.status) },
    ], d.skus)),
    drawerSection('原始属性', h('pre', { class: 'code' }, prettyJSON(d.attributes_json) || '{}'))));

  return null;
}

/* ------------------------------------------------------------ aftersales */

async function renderAftersales(params) {
  const tab = params.get('tab') || 'cases';
  const page = params.get('page') || 1;
  const status = params.get('status') || '';
  const query = new URLSearchParams({ page, page_size: 20 });
  if (status) query.set('status', status);

  const tabs = h('div', { class: 'tabs' },
    [['cases', '售后单'], ['refunds', '退款'], ['invoices', '发票'], ['promotions', '促销']].map(([id, text]) =>
      h('button', {
        class: `tab ${tab === id ? 'active' : ''}`, type: 'button',
        onclick: () => setParam('tab', id),
      }, text)));

  if (tab === 'refunds') {
    const res = await api(`/refunds?${query}`);
    return h('section', { class: 'card' }, tabs,
      toolbar(selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'requested', label: '已申请' },
        { value: 'pending_review', label: '待审核' },
        { value: 'processing', label: '处理中' },
        { value: 'succeeded', label: '已成功' },
        { value: 'failed', label: '失败' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 笔`)),
      table([
        { key: 'refund_no', label: '退款号', cellClass: 'mono strong nowrap' },
        { key: 'order_no', label: '订单号', cellClass: 'mono' },
        { key: 'customer_no', label: '客户', cellClass: 'mono' },
        { label: '状态', render: (r) => badge(r.refund_status) },
        { label: '金额', cellClass: 'num', render: (r) => r.amount_display },
        { key: 'reason', label: '原因', cellClass: 'truncate' },
        { label: '申请', cellClass: 'nowrap', render: (r) => fmtTime(r.requested_at) },
      ], res.data, { emptyTitle: '没有退款记录' }),
      pager(res.meta, (p) => setParam('page', p)));
  }

  if (tab === 'invoices') {
    const res = await api(`/invoices?${query}`);
    return h('section', { class: 'card' }, tabs,
      toolbar(selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'requested', label: '已申请' },
        { value: 'processing', label: '处理中' },
        { value: 'issued', label: '已开具' },
        { value: 'rejected', label: '已拒绝' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 张`)),
      table([
        { label: '发票号', cellClass: 'mono', render: (r) => r.invoice_no || '—' },
        { key: 'order_no', label: '订单号', cellClass: 'mono' },
        { key: 'customer_no', label: '客户', cellClass: 'mono' },
        { label: '类型', render: (r) => label(r.invoice_type) },
        { label: '状态', render: (r) => badge(r.invoice_status) },
        { key: 'title', label: '抬头', cellClass: 'truncate' },
        { label: '金额', cellClass: 'num', render: (r) => r.amount_display },
        { label: '申请', cellClass: 'nowrap', render: (r) => fmtTime(r.requested_at) },
      ], res.data, { emptyTitle: '没有发票记录' }),
      pager(res.meta, (p) => setParam('page', p)));
  }

  if (tab === 'promotions') {
    const res = await api(`/promotions?${query}`);
    return h('section', { class: 'card' }, tabs,
      toolbar(selectFilter(status, [
        { value: '', label: '全部状态' },
        { value: 'active', label: '进行中' },
        { value: 'draft', label: '草稿' },
        { value: 'expired', label: '已过期' },
        { value: 'disabled', label: '已停用' },
      ], (v) => setParam('status', v)),
      h('span', { class: 'spacer' }),
      h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 个活动`)),
      table([
        { key: 'promotion_code', label: '编码', cellClass: 'mono strong' },
        { key: 'title', label: '活动', cellClass: 'strong truncate' },
        { key: 'promotion_type', label: '类型' },
        { label: '状态', render: (r) => badge(r.status) },
        { label: '开始', cellClass: 'nowrap', render: (r) => r.starts_at ? fmtTime(r.starts_at) : '—' },
        { label: '结束', cellClass: 'nowrap', render: (r) => r.ends_at ? fmtTime(r.ends_at) : '—' },
      ], res.data, {
        emptyTitle: '没有促销活动',
        onRow: (r) => openDrawer(r.title, r.promotion_code, h('div', null,
          drawerSection('活动', kv([
            ['编码', r.promotion_code, 'mono'],
            ['类型', r.promotion_type],
            ['状态', badge(r.status)],
            ['开始', r.starts_at ? fmtFull(r.starts_at) : '—'],
            ['结束', r.ends_at ? fmtFull(r.ends_at) : '—'],
            ['描述', r.description || '—'],
          ])),
          drawerSection('规则', h('pre', { class: 'code' }, prettyJSON(r.rules_json) || '{}')))),
      }),
      pager(res.meta, (p) => setParam('page', p)));
  }

  const res = await api(`/service-cases?${query}`);
  return h('section', { class: 'card' }, tabs,
    toolbar(selectFilter(status, [
      { value: '', label: '全部状态' },
      { value: 'submitted', label: '已提交' },
      { value: 'pending_review', label: '待审核' },
      { value: 'approved', label: '已批准' },
      { value: 'awaiting_return_shipment', label: '等退货' },
      { value: 'processing', label: '处理中' },
      { value: 'completed', label: '已完成' },
      { value: 'rejected', label: '已拒绝' },
    ], (v) => setParam('status', v)),
    h('span', { class: 'spacer' }),
    h('span', { class: 'toolbar-meta' }, `共 ${num(res.meta.total)} 单`)),
    table([
      { key: 'case_no', label: '售后单号', cellClass: 'mono strong nowrap' },
      { key: 'order_no', label: '订单号', cellClass: 'mono' },
      { key: 'customer_no', label: '客户', cellClass: 'mono' },
      { label: '类型', render: (r) => label(r.case_type) },
      { label: '状态', render: (r) => badge(r.case_status) },
      { label: '数量', cellClass: 'num', render: (r) => num(r.requested_quantity) },
      { key: 'description', label: '描述', cellClass: 'truncate' },
      { label: '创建', cellClass: 'nowrap', render: (r) => fmtTime(r.created_at) },
    ], res.data, {
      emptyTitle: '没有售后单',
      onRow: (r) => openDrawer(r.case_no, `${label(r.case_type)} · ${label(r.case_status)}`, h('div', null,
        drawerSection('售后单', kv([
          ['单号', r.case_no, 'mono'],
          ['订单', r.order_no, 'mono'],
          ['客户', r.customer_no, 'mono'],
          ['类型', label(r.case_type)],
          ['状态', badge(r.case_status)],
          ['原因码', r.reason_code || '—'],
          ['申请数量', num(r.requested_quantity)],
          ['处理结果', r.resolution || '—'],
          ['创建', fmtFull(r.created_at)],
          ['关闭', r.closed_at ? fmtFull(r.closed_at) : '—'],
        ])),
        r.description ? drawerSection('描述', h('div', { style: 'white-space:pre-wrap;font-size:12px' }, r.description)) : null,
        drawerSection('跳转', h('button', {
          class: 'btn btn-ghost btn-sm', type: 'button',
          onclick: () => { closeDrawer(); location.hash = `#/orders?id=${r.order_no}`; },
        }, '查看关联订单')))),
    }),
    pager(res.meta, (p) => setParam('page', p)));
}

/* ---------------------------------------------------------------- system */

async function renderSystem() {
  const res = await api('/health');
  const hdata = res.data;
  const total = hdata.tables.reduce((sum, t) => sum + Math.max(t.count, 0), 0);

  const cols = [
    { key: 'table', label: '数据表', cellClass: 'mono strong' },
    { label: '行数', cellClass: 'num', render: (r) => r.count < 0 ? h('span', { style: 'color:var(--danger)' }, '读取失败') : num(r.count) },
    {
      label: '占比', render: (r) => {
        const w = total > 0 ? Math.max(1, Math.round((Math.max(r.count, 0) / total) * 100)) : 0;
        return h('div', { style: 'display:flex;align-items:center;gap:8px' },
          h('div', { style: `height:5px;width:${w}%;min-width:2px;max-width:180px;background:var(--accent);border-radius:3px;opacity:.55` }),
          h('span', { class: 'faint', style: 'font-size:11px' }, `${w}%`));
      },
    },
  ];

  return h('div', null,
    h('div', { class: 'metrics' },
      h('div', { class: 'metric' },
        h('div', { class: 'metric-label' }, '迁移版本'),
        h('div', { class: 'metric-value' }, `v${hdata.schema_version}`),
        h('div', { class: 'metric-hint' }, 'schema_migrations 最高版本')),
      h('div', { class: 'metric' },
        h('div', { class: 'metric-label' }, '运行环境'),
        h('div', { class: 'metric-value', style: 'font-size:17px' }, hdata.app_env),
        h('div', { class: 'metric-hint' }, hdata.llm_mode === 'offline' ? '离线演示模型' : hdata.llm_mode === 'openai' ? '真实模型' : '模型未启用')),
      h('div', { class: 'metric' },
        h('div', { class: 'metric-label' }, '活跃会话（后台）'),
        h('div', { class: 'metric-value' }, num(hdata.active_sessions)),
        h('div', { class: 'metric-hint' }, '未过期且未撤销')),
      h('div', { class: 'metric' },
        h('div', { class: 'metric-label' }, '幂等键'),
        h('div', { class: 'metric-value' }, num(hdata.idempotency_keys)),
        h('div', { class: 'metric-hint' }, '重试去重记录'))),
    h('div', { class: 'section-gap' },
      panel('数据库', h('div', { class: 'stack' },
        hdata.db_path ? kv([['文件', hdata.db_path, 'mono']]) : h('div', { class: 'muted' }, '生产环境不暴露数据库路径'),
        h('div', { class: 'muted', style: 'font-size:12px' },
          '行数用于核对「当前进程打开的是不是预期那份库」。若与 seed 结果不一致，先检查 DB_PATH 与启动目录。')))),
    h('div', { class: 'section-gap' },
      h('section', { class: 'card' },
        h('div', { class: 'card-head' },
          h('h2', null, '各表行数'),
          h('span', { class: 'hint' }, `合计 ${num(total)} 行`)),
        table(cols, hdata.tables))));
}

/* ------------------------------------------------------------------- boot */

function init() {
  applyTheme();
  buildNav();

  $('#login-form').addEventListener('submit', doLogin);
  $('#logout').addEventListener('click', doLogout);
  $('#theme-toggle').addEventListener('click', cycleTheme);
  $('#refresh').addEventListener('click', () => {
    navigate();
    toast('已刷新');
  });

  document.querySelectorAll('[data-close="drawer"]').forEach((el) =>
    el.addEventListener('click', () => {
      closeDrawer();
      if (state.params.get('id')) setParam('id', null, { replace: true });
    }));
  document.querySelectorAll('[data-close="modal"]').forEach((el) =>
    el.addEventListener('click', closeModal));

  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    if (!$('#modal').hidden) closeModal();
    else if (!$('#drawer').hidden) {
      closeDrawer();
      if (state.params.get('id')) setParam('id', null, { replace: true });
    }
  });

  window.addEventListener('hashchange', navigate);

  boot();
}

async function boot() {
  // Ask the server whether this is a dev process before attempting /auth/me:
  // the login screen uses it to decide whether to show the built-in hint.
  try {
    const mode = await api('/auth/mode');
    state.meta = { ...state.meta, dev: !!mode.data.dev, app_env: mode.data.app_env };
  } catch { /* 服务不可用时继续走 /auth/me，由它报错 */ }

  try {
    await bootSession();
    showApp();
    if (!location.hash) location.hash = '#/overview';
    else navigate();
  } catch (err) {
    if (err.message === '未登录') showLogin();
    else showLogin(err.message);
  }
}

init();
