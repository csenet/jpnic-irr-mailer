'use strict';

// ---- 状態 ----------------------------------------------------------------

const state = {
  cfg: {},
  authorized: false,
  account: '',
  items: [],              // { id, orig, cur, action }
  filter: '',
  classFilter: 'routes',  // 既定は route/route6 のみ。key-cert などは隠す
  staleOnly: false,       // 更新期限が近いものだけを表示
  editing: null,          // 編集中の item id
  draft: [],              // 編集モーダル内の属性配列
};

let nextId = 1;
let editModal = null;
let sendModal = null;

// 属性ごとの説明。編集モーダルの入力欄の下と「属性を追加」の選択肢に出す。
// required は route/route6 で必須のもの、multiline は継続行を書ける欄。
const ATTR_INFO = {
  'route':     { desc: 'IPv4 プレフィックス。ネットワークアドレスで書く (例: 192.0.2.0/24)', ph: '192.0.2.0/24', required: true },
  'route6':    { desc: 'IPv6 プレフィックス (例: 2001:db8::/32)', ph: '2001:db8::/32', required: true },
  'descr':     { desc: '組織名などの説明。2 行目に「X-Keiro : alert@example.jp」と書くと経路奉行 (経路ハイジャック通知) の宛先になる', ph: 'Example Inc.\nX-Keiro : alert@example.jp', required: true, multiline: true },
  'origin':    { desc: 'この経路を広告する AS 番号', ph: 'AS64496', required: true },
  'member-of': { desc: '所属させる route-set 名 (任意)', ph: 'RS-EXAMPLE' },
  'notify':    { desc: 'このオブジェクトが変更されたときに通知を受けるメールアドレス (任意)', ph: 'noc@example.jp' },
  'mnt-by':    { desc: 'このオブジェクトを管理する maintainer。password はこの mntner のもの', ph: 'MAINT-AS64496', required: true },
  'mnt-lower': { desc: 'より細かいプレフィックスの登録を許可する maintainer (任意)', ph: 'MAINT-AS64496' },
  'admin-c':   { desc: '管理連絡担当者。JPNIC ハンドル (例: AA000JP) か氏名 (任意)', ph: 'AA000JP' },
  'tech-c':    { desc: '技術連絡担当者。JPNIC ハンドル (例: BB000JP) か氏名 (任意)', ph: 'BB000JP' },
  'remarks':   { desc: '自由記述の備考 (任意)。複数行可', ph: '', multiline: true },
  'country':   { desc: 'ISO 3166 の国コード (任意)', ph: 'JP' },
  'holes':     { desc: 'この経路の中で広告しないプレフィックス (任意)', ph: '192.0.2.128/25' },
  'geoidx':    { desc: '地理的な位置情報 (任意、ほぼ使われない)', ph: '' },
  'changed':   { desc: '送信時に「changed 用アドレス + 当日」で自動更新されるので編集不要', ph: '' },
  'source':    { desc: '常に JPIRR。編集不要', ph: 'JPIRR' },
  'mntner':    { desc: 'maintainer 名。新規登録は irr-admin@nic.ad.jp への申請が必要', ph: 'MAINT-AS64496', required: true },
  'upd-to':    { desc: '認証に失敗した更新があったときの通知先', ph: 'admin@example.jp' },
  'mnt-nfy':   { desc: 'この maintainer 配下が更新されたときの通知先。ガベージコレクタの警告もここに届く', ph: 'admin@example.jp' },
  'auth':      { desc: '認証方式。CRYPT-PW <ハッシュ> または PGPKEY-<ID>。whois の伏字 (HIDDENCRYPTPW) は送信時に入力パスワードから自動で作り直される', ph: 'CRYPT-PW lIUkAMPHFC2kE' },
  'aut-num':   { desc: 'AS 番号', ph: 'AS64496', required: true },
  'as-name':   { desc: 'AS の名前', ph: 'EXAMPLE-AS' },
  'as-set':    { desc: 'AS 集合の名前', ph: 'AS-EXAMPLE', required: true },
  'members':   { desc: '集合に含める AS 番号や as-set', ph: 'AS64496, AS-CUSTOMER' },
  'certif':    { desc: 'PGP 公開鍵ブロック', ph: '', multiline: true },
  'person':    { desc: '担当者の氏名 (ローマ字)。ハンドルの自動採番は無いので nic-hdl に割り当て済みの JPNIC ハンドルが必要', ph: 'Keiro Taro', required: true },
  'role':      { desc: '担当グループの名称', ph: 'IRR Operation Team', required: true },
  'nic-hdl':   { desc: 'JPNIC から割り当て済みのハンドル (例: TK36JP、グループは JP00000000)。JPIRR では新規発行できない', ph: 'TK36JP', required: true },
  'address':   { desc: '住所。行を分けて複数書ける', ph: 'Example Inc.\n2-3-4 Uchi-kanda\nChiyoda-ku, Tokyo', required: true, multiline: true },
  'phone':     { desc: '電話番号 (国際形式)', ph: '+81-3-1234-5678', required: true },
  'fax-no':    { desc: 'FAX 番号 (任意)', ph: '+81-3-9876-5432' },
  'e-mail':    { desc: '連絡先メールアドレス', ph: 'taro@example.jp' },
  'trouble':   { desc: '障害時の連絡先 (任意)', ph: 'noc@example.jp / +81-3-1234-5678' },
  'import':    { desc: '受信ポリシー (RPSL)。例: from AS64500 accept ANY', ph: 'from AS64500 accept ANY' },
  'export':    { desc: '広報ポリシー (RPSL)。例: to AS64500 announce AS64496', ph: 'to AS64500 announce AS64496' },
  'default':   { desc: 'デフォルトルートのポリシー (任意)', ph: 'to AS64500' },
};

// クラスごとの「属性を追加」候補。
const CLASS_SUGGESTIONS = {
  'route':   ['descr', 'origin', 'member-of', 'notify', 'mnt-by', 'mnt-lower', 'admin-c', 'tech-c', 'remarks', 'country', 'holes', 'geoidx'],
  'route6':  ['descr', 'origin', 'member-of', 'notify', 'mnt-by', 'mnt-lower', 'admin-c', 'tech-c', 'remarks', 'country', 'holes', 'geoidx'],
  'person':  ['address', 'phone', 'fax-no', 'e-mail', 'trouble', 'remarks', 'notify', 'mnt-by'],
  'role':    ['address', 'phone', 'fax-no', 'e-mail', 'trouble', 'remarks', 'notify', 'mnt-by', 'admin-c', 'tech-c'],
  'aut-num': ['as-name', 'descr', 'member-of', 'import', 'export', 'default', 'notify', 'admin-c', 'tech-c', 'mnt-by', 'remarks'],
  'as-set':  ['descr', 'members', 'notify', 'mnt-by', 'remarks', 'admin-c', 'tech-c'],
  'mntner':  ['descr', 'admin-c', 'tech-c', 'upd-to', 'notify', 'mnt-nfy', 'auth', 'remarks', 'mnt-by'],
};


// JPIRR のガベージコレクタは最終更新から 12 ヶ月で通知、14 ヶ月で削除する。
const GC_WARN_MONTHS = 11;
const GC_DELETE_MONTHS = 14;

const ACTION_LABEL = {
  none: '登録済み', create: '新規', update: '変更', delete: '削除', touch: '再登録',
};
// 状態バッジに使う Bootstrap の配色。
const ACTION_BADGE = {
  none: 'text-bg-light', create: 'text-bg-success', update: 'text-bg-warning',
  delete: 'text-bg-danger', touch: 'text-bg-info',
};
const ACTION_ROW = {
  none: '', create: 'table-success', update: 'table-warning',
  delete: 'table-danger', touch: 'table-info',
};

// ---- API ------------------------------------------------------------------

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  const text = await res.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = { error: text }; }
  }
  if (!res.ok) throw new Error((data && data.error) || `HTTP ${res.status}`);
  return data;
}

// ---- 補助 -----------------------------------------------------------------

const $ = (id) => document.getElementById(id);

// Bootstrap の alert は d-none で出し入れする。
function setAlert(id, msg) {
  const el = $(id);
  el.textContent = msg || '';
  el.classList.toggle('d-none', !msg);
}

const showError = (msg) => {
  setAlert('globalError', msg);
  if (msg) window.scrollTo({ top: 0, behavior: 'smooth' });
};
const showOk = (msg) => setAlert('globalOk', msg);

function attrGet(obj, name) {
  const a = obj.attrs.find((x) => x.name.toLowerCase() === name);
  return a ? a.value : '';
}

const sameAttrs = (a, b) => JSON.stringify(a.attrs) === JSON.stringify(b.attrs);
const objClass = (obj) => (obj.attrs.length ? obj.attrs[0].name.toLowerCase() : '');
const objKey = (obj) => (obj.attrs.length ? obj.attrs[0].value : '');

// changed: の日付から経過月数を求め、期限が近いものを一覧で目立たせる。
function lastChanged(obj) {
  const m = /(\d{8})\s*$/.exec(attrGet(obj, 'changed').trim());
  if (!m) return null;
  const y = +m[1].slice(0, 4), mo = +m[1].slice(4, 6), d = +m[1].slice(6, 8);
  const now = new Date();
  const months = (now.getFullYear() - y) * 12 + (now.getMonth() - (mo - 1))
    - (now.getDate() < d ? 1 : 0);
  return { text: `${m[1].slice(0, 4)}-${m[1].slice(4, 6)}-${m[1].slice(6, 8)}`, months };
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

// changed:/delete: に書くアドレス。専用設定が空ならメールの From を使う。
function changedAddress() {
  return $('cfgChanged').value.trim() || $('cfgFrom').value.trim();
}

// ---- 初期化 ---------------------------------------------------------------

async function loadState() {
  try {
    const s = await api('/api/state');
    state.cfg = s.config || {};
    state.authorized = !!s.authorized;
    state.account = s.account || '';
    $('cfgMntner').value = state.cfg.mntner || '';
    $('cfgFrom').value = state.cfg.from || '';
    $('cfgChanged').value = state.cfg.changedBy || '';
    $('cfgTo').value = state.cfg.to || '';
    $('cfgWhois').value = state.cfg.whoisServer || '';
    $('cfgNotify').value = state.cfg.notify || '';
    $('cfgOrigin').value = state.cfg.defaultOrigin || '';
    $('cfgPassword').value = state.cfg.password || '';
    $('cfgTransport').value = state.cfg.transport || 'smtp';
    $('cfgSmtpHost').value = state.cfg.smtpHost || '';
    $('cfgSmtpPort').value = state.cfg.smtpPort || '';
    $('cfgSmtpUser').value = state.cfg.smtpUser || '';
    $('cfgSmtpPass').value = '';
    $('smtpPassState').textContent = s.smtpPasswordSet ? '(保存済み。空欄のまま保存すると変更なし)' : '';
    updateTransportUI();
    $('cfgPath').textContent = s.configDir ? `設定の保存先: ${s.configDir}` : '';

    const badge = $('authBadge');
    const smtp = (state.cfg.transport || 'smtp') === 'smtp';
    if (state.authorized) {
      badge.className = 'badge rounded-pill text-bg-success';
      badge.textContent = (smtp ? 'SMTP: ' : '') + (state.account || '認証済み');
    } else {
      badge.className = 'badge rounded-pill text-bg-danger';
      badge.textContent = smtp ? 'SMTP 未設定' : 'Gmail 未認証';
    }
    // OAuth のログイン/ログアウトは Gmail API 方式のときだけ意味がある。
    $('authBtn').hidden = smtp || state.authorized;
    $('logoutBtn').hidden = smtp || !state.authorized;
    if (!smtp && s.gmailSetup) showError(s.gmailSetup);
    if (smtp && !state.authorized) bootstrap.Collapse.getOrCreateInstance($('settingsBody')).show();

    $('mntnerBadge').textContent = state.cfg.mntner || '未設定';
    // 初回起動時は設定が空なので、設定パネルを開いた状態で見せる。
    if (!state.cfg.mntner) {
      bootstrap.Collapse.getOrCreateInstance($('settingsBody')).show();
    }
  } catch (e) {
    showError(e.message);
  }
}

// 送信方式に応じて SMTP の入力欄と説明を出し分ける。
function updateTransportUI() {
  const smtp = $('cfgTransport').value === 'smtp';
  document.querySelectorAll('.smtp-only').forEach((el) => { el.hidden = !smtp; });
  $('transportHint').textContent = smtp
    ? 'Google アカウントのアプリパスワードで smtp.gmail.com に送ります。GCP の設定は不要です。'
    : 'GCP で作った OAuth クライアント (credentials.json) が必要です。手順は README を参照。';
}

// ---- オブジェクト取得 -----------------------------------------------------

async function fetchObjects() {
  const mntner = $('cfgMntner').value.trim();
  if (!mntner) {
    bootstrap.Collapse.getOrCreateInstance($('settingsBody')).show();
    showError('Maintainer 名を入力してください (例: MAINT-AS64496)');
    return;
  }
  const dirty = state.items.some((i) => currentAction(i) !== 'none');
  if (dirty && !confirm('未送信の変更があります。取得し直すと破棄されますが、よろしいですか？')) return;

  const btn = $('fetchBtn');
  btn.disabled = true;
  btn.textContent = '取得中…';
  showError(''); showOk('');
  try {
    const res = await api(`/api/objects?mntner=${encodeURIComponent(mntner)}`);
    state.items = (res.objects || []).map((o) => ({
      id: nextId++,
      orig: o,
      cur: structuredClone(o),
      action: 'none',
    }));
    const stale = state.items.filter((i) => {
      const ch = lastChanged(i.cur);
      return ch && ch.months >= GC_WARN_MONTHS;
    }).length;
    showOk(`${state.items.length} 件のオブジェクトを ${res.server} から取得しました。`
      + (stale ? ` うち ${stale} 件は最終更新から ${GC_WARN_MONTHS} ヶ月以上経過しています (${GC_DELETE_MONTHS} ヶ月で自動削除)。` : ''));
    rebuildClassFilter();
    render();
  } catch (e) {
    showError(e.message);
  } finally {
    btn.disabled = false;
    btn.textContent = 'JPIRR から取得';
  }
}

// 取得結果に含まれるクラスだけを種別セレクタに並べる。
function rebuildClassFilter() {
  const sel = $('classFilter');
  const classes = [...new Set(state.items.map((i) => objClass(i.cur)))].sort();
  const keep = state.classFilter;
  sel.textContent = '';
  sel.appendChild(new Option('route / route6', 'routes'));
  sel.appendChild(new Option('すべて', ''));
  classes.filter((c) => c !== 'route' && c !== 'route6')
    .forEach((c) => sel.appendChild(new Option(c, c)));
  sel.value = [...sel.options].some((o) => o.value === keep) ? keep : 'routes';
  state.classFilter = sel.value;
}

// ---- 描画 -----------------------------------------------------------------

function currentAction(item) {
  if (item.action === 'delete' || item.action === 'create') return item.action;
  if (item.orig && !sameAttrs(item.orig, item.cur)) return 'update';
  // 内容は変えずに changed: の日付だけ新しくする再登録。
  // JPIRR のガベージコレクタに消されないよう、定期的に送り直すための操作。
  if (item.action === 'touch') return 'touch';
  return 'none';
}

// 種別セレクタ・絞り込み文字列・期限フィルタをすべて適用した表示対象。
function visibleItems() {
  const q = state.filter.toLowerCase();
  return state.items.filter((item) => {
    const cls = objClass(item.cur);
    if (state.classFilter === 'routes') {
      if (cls !== 'route' && cls !== 'route6') return false;
    } else if (state.classFilter && cls !== state.classFilter) {
      return false;
    }
    if (state.staleOnly) {
      const ch = lastChanged(item.cur);
      if (!ch || ch.months < GC_WARN_MONTHS) return false;
    }
    if (!q) return true;
    return item.cur.attrs.some((a) => a.value.toLowerCase().includes(q));
  });
}

function render() {
  const tbody = $('objectRows');
  tbody.textContent = '';
  const visible = visibleItems();

  for (const item of visible) {
    const act = currentAction(item);
    const tr = document.createElement('tr');
    if (ACTION_ROW[act]) tr.className = ACTION_ROW[act];
    if (act === 'delete') tr.classList.add('row-delete');

    tr.appendChild(td(`<span class="badge text-bg-light font-monospace">${objClass(item.cur)}</span>`));
    tr.appendChild(td(escapeHTML(objKey(item.cur)), 'key'));
    tr.appendChild(td(escapeHTML(attrGet(item.cur, 'origin')), 'origin'));

    const descr = attrGet(item.cur, 'descr').split('\n')[0];
    const descrCell = td(escapeHTML(descr), 'descr');
    descrCell.title = attrGet(item.cur, 'descr');
    tr.appendChild(descrCell);

    const ch = lastChanged(item.cur);
    if (ch) {
      const stale = ch.months >= GC_WARN_MONTHS;
      tr.appendChild(td(
        `${escapeHTML(ch.text)}<br><span class="small ${stale ? 'text-danger fw-semibold' : 'text-body-secondary'}">`
        + `${ch.months} ヶ月前${stale ? `・残り ${Math.max(GC_DELETE_MONTHS - ch.months, 0)} ヶ月` : ''}</span>`,
        'changed'));
    } else {
      tr.appendChild(td('<span class="text-body-secondary">—</span>', 'changed'));
    }

    tr.appendChild(td(`<span class="badge ${ACTION_BADGE[act]}">${ACTION_LABEL[act]}</span>`));

    const actions = document.createElement('td');
    actions.className = 'actions';
    const group = document.createElement('div');
    group.className = 'btn-group btn-group-sm';

    if (item.action !== 'delete') {
      group.appendChild(button('編集', () => openEditor(item.id)));
    }
    if (item.action === 'create') {
      group.appendChild(button('取り消す', () => {
        state.items = state.items.filter((x) => x.id !== item.id);
        render();
      }, 'btn-outline-danger'));
    } else if (item.action === 'delete') {
      group.appendChild(button('削除をやめる', () => { item.action = 'none'; render(); }));
    } else {
      if (act === 'update') {
        group.appendChild(button('戻す', () => {
          item.cur = structuredClone(item.orig);
          item.action = 'none';
          render();
        }));
      } else if (act === 'touch') {
        group.appendChild(button('やめる', () => { item.action = 'none'; render(); }));
      } else {
        group.appendChild(button('再登録', () => { item.action = 'touch'; render(); },
          'btn-outline-warning', 'changed: の日付だけ更新して送り直します'));
      }
      group.appendChild(button('削除', () => { item.action = 'delete'; render(); }, 'btn-outline-danger'));
    }
    actions.appendChild(group);
    tr.appendChild(actions);
    tbody.appendChild(tr);
  }

  const hasItems = state.items.length > 0;
  $('emptyState').hidden = hasItems && visible.length > 0;
  if (hasItems && visible.length === 0) {
    $('emptyState').textContent = '表示条件に一致するオブジェクトがありません。種別や絞り込みを見直してください。';
  }
  $('rowCount').textContent = hasItems ? `${visible.length} / ${state.items.length} 件を表示` : '';

  renderChangeBar();
}

function td(html, cls) {
  const el = document.createElement('td');
  el.innerHTML = html;
  if (cls) el.className = cls;
  return el;
}

function button(label, onClick, cls = 'btn-outline-secondary', title = '') {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = `btn ${cls}`;
  b.textContent = label;
  if (title) b.title = title;
  b.addEventListener('click', onClick);
  return b;
}

function changeList() {
  return state.items
    .map((item) => ({ item, action: currentAction(item) }))
    .filter((x) => x.action !== 'none')
    // 再登録は JPIRR から見れば通常の更新と同じ。changed: が当日に書き換わる。
    .map((x) => ({ action: x.action === 'touch' ? 'update' : x.action, object: x.item.cur }));
}

function renderChangeBar() {
  const n = { create: 0, update: 0, delete: 0, touch: 0 };
  let total = 0;
  state.items.forEach((item) => {
    const act = currentAction(item);
    if (act !== 'none') { n[act]++; total++; }
  });

  $('changeCount').textContent = total ? `${total} 件の変更` : '変更なし';
  for (const k of ['create', 'update', 'delete', 'touch']) {
    const el = $('cnt' + k[0].toUpperCase() + k.slice(1));
    el.hidden = n[k] === 0;
    el.textContent = `${ACTION_LABEL[k]} ${n[k]}`;
  }
  $('reviewBtn').disabled = total === 0;
  $('resetBtn').disabled = total === 0;
}

// 表示中のオブジェクトをまとめて再登録の対象にする。
// 内容を変えずに changed: だけ更新するので、ガベージコレクタ対策の一括操作に使う。
function touchAllVisible() {
  const targets = visibleItems().filter((i) => currentAction(i) === 'none');
  if (!targets.length) {
    showError('再登録できるオブジェクトがありません (すでに変更済みのものは対象外です)。');
    return;
  }
  if (!confirm(`表示中の ${targets.length} 件を再登録します。内容は変えずに changed: の日付だけ更新されます。`)) return;
  targets.forEach((i) => { i.action = 'touch'; });
  showError('');
  render();
}

// ---- 編集モーダル ---------------------------------------------------------

// JPNIC の登録フォームに合わせた新規作成テンプレート。空欄は送信時に自動で落ちる。
function newObjectTemplate(cls) {
  const a = (name, value = '') => ({ name, value });
  const notify = a('notify', state.cfg.notify || '');
  const mntBy = a('mnt-by', state.cfg.mntner || '');
  const source = a('source', 'JPIRR');
  switch (cls) {
    case 'person':
      return { attrs: [a('person'), a('nic-hdl'), a('address'), a('phone'), a('fax-no'), a('e-mail'), notify, mntBy, source] };
    case 'role':
      return { attrs: [a('role'), a('nic-hdl'), a('address'), a('phone'), a('fax-no'), a('e-mail'), notify, mntBy, source] };
    case 'aut-num':
      return { attrs: [a('aut-num', state.cfg.defaultOrigin || ''), a('as-name'), a('descr'), notify, a('admin-c'), a('tech-c'), mntBy, source] };
    case 'as-set':
      return { attrs: [a('as-set'), a('descr'), a('members'), notify, mntBy, source] };
    default:
      return { attrs: [a(cls), a('descr'), a('origin', state.cfg.defaultOrigin || ''), a('admin-c'), a('tech-c'), notify, mntBy, source] };
  }
}

function addNew(cls) {
  const item = { id: nextId++, orig: null, cur: newObjectTemplate(cls), action: 'create' };
  state.items.unshift(item);
  // 今の種別フィルタで隠れてしまう種別なら「すべて」に切り替えて見えるようにする。
  const isRoute = cls === 'route' || cls === 'route6';
  if ((state.classFilter === 'routes' && !isRoute) || (state.classFilter && state.classFilter !== 'routes' && state.classFilter !== cls)) {
    state.classFilter = '';
    $('classFilter').value = '';
  }
  render();
  openEditor(item.id);
}

function openEditor(id) {
  const item = state.items.find((x) => x.id === id);
  if (!item) return;
  state.editing = id;
  state.draft = structuredClone(item.cur.attrs);
  $('editTitle').textContent =
    item.action === 'create'
      ? `${objClass(item.cur)} を新規作成`
      : `${objClass(item.cur)} ${objKey(item.cur)} を編集`;
  setAlert('editError', '');

  const picker = $('attrPicker');
  picker.textContent = '';
  picker.appendChild(new Option('属性を追加…', ''));
  const suggestions = CLASS_SUGGESTIONS[objClass(item.cur)] || Object.keys(ATTR_INFO);
  suggestions.forEach((a) => {
    const info = ATTR_INFO[a];
    picker.appendChild(new Option(info ? `${a} — ${info.desc.split('。')[0]}` : a, a));
  });

  renderDraft();
  editModal.show();
}

function renderDraft() {
  const wrap = $('attrRows');
  wrap.textContent = '';
  state.draft.forEach((attr, idx) => {
    const row = document.createElement('div');
    row.className = 'attr-row';

    const name = document.createElement('input');
    name.type = 'text';
    name.className = 'form-control form-control-sm';
    name.value = attr.name;
    // 先頭属性はクラス名そのものなので変更させない。
    name.readOnly = idx === 0;
    name.addEventListener('input', () => { attr.name = name.value.trim(); });
    row.appendChild(name);

    const info = ATTR_INFO[attr.name.toLowerCase()] || {};
    // descr や remarks は継続行を書けるよう複数行入力にする。
    const multiline = !!info.multiline;
    const value = document.createElement(multiline ? 'textarea' : 'input');
    value.className = 'form-control form-control-sm';
    if (!multiline) value.type = 'text';
    value.value = attr.value;
    value.placeholder = info.ph || '';
    // 属性名を打ち替えたら説明も追随させる。
    value.addEventListener('input', () => { attr.value = value.value; });
    row.appendChild(value);

    const help = document.createElement('div');
    help.className = 'attr-help form-text';
    const renderHelp = () => {
      const i = ATTR_INFO[attr.name.toLowerCase()];
      help.innerHTML = i
        ? `${i.required ? '<span class="badge text-bg-danger me-1">必須</span>' : ''}${escapeHTML(i.desc)}`
        : '<span class="text-body-secondary">JPIRR の属性名を入力してください</span>';
    };
    renderHelp();
    name.addEventListener('input', renderHelp);

    const del = document.createElement('button');
    del.type = 'button';
    del.className = 'btn btn-sm btn-outline-danger';
    del.textContent = '×';
    del.title = 'この属性を削除';
    del.disabled = idx === 0;
    del.addEventListener('click', () => {
      state.draft.splice(idx, 1);
      renderDraft();
    });
    row.appendChild(del);
    row.appendChild(help);

    wrap.appendChild(row);
  });
}

function saveEditor() {
  const item = state.items.find((x) => x.id === state.editing);
  if (!item) return;
  const attrs = state.draft
    .map((a) => ({ name: a.name.trim(), value: a.value.trim() }))
    .filter((a) => a.name !== '');
  if (!attrs.length || !attrs[0].value) {
    setAlert('editError', `${attrs.length ? attrs[0].name : 'キー'} の値を入力してください。`);
    return;
  }
  item.cur = { attrs };
  editModal.hide();
  state.editing = null;
  render();
}

// ---- 送信 -----------------------------------------------------------------

async function openSend() {
  setAlert('sendError', '');
  setAlert('sendWarn', '');
  $('sendPreview').textContent = '読み込み中…';
  $('sendTarget').textContent = `宛先: ${state.cfg.to || 'auto-dbm@nic.ad.jp'}`;
  // 設定に保存済みならそれを初期値にする。
  $('sendPassword').value = state.cfg.password || '';
  $('sendPasswordHint').textContent = state.cfg.password
    ? '設定に保存したパスワードが入っています'
    : '入力値はメール本文にのみ使われ、保存されません';
  sendModal.show();
  await refreshPreview();
}

async function refreshPreview() {
  try {
    const res = await api('/api/preview', {
      method: 'POST',
      body: JSON.stringify({
        // プレビューでは実パスワードを送らず、伏字のまま体裁を確認する。
        password: 'preview',
        from: changedAddress(),
        changes: changeList(),
      }),
    });
    $('sendPreview').textContent = res.preview;
    setAlert('sendWarn', (res.warnings || []).join('\n'));
    setAlert('sendError', '');
  } catch (e) {
    $('sendPreview').textContent = '';
    setAlert('sendError', e.message);
  }
}

async function doSend() {
  const password = $('sendPassword').value;
  if (!password) {
    setAlert('sendError', 'JPIRR のパスワードを入力してください。');
    return;
  }
  if (!state.authorized) {
    setAlert('sendError', 'Gmail が未認証です。画面右上からログインしてください。');
    return;
  }
  const btn = $('sendSubmit');
  btn.disabled = true;
  btn.textContent = '送信中…';
  try {
    const res = await api('/api/send', {
      method: 'POST',
      body: JSON.stringify({
        password,
        from: changedAddress(),
        changes: changeList(),
      }),
    });
    $('sendPassword').value = '';
    sendModal.hide();
    showError('');
    showOk(`${res.to} へ送信しました (件名: ${res.subject})。JPIRR からの結果通知メールで登録内容を必ず確認してください。`);
    // 送信済みの内容が現状になるよう、取り込み直す。
    state.items.forEach((i) => { i.action = 'none'; });
    await fetchObjects();
  } catch (e) {
    setAlert('sendError', e.message);
  } finally {
    btn.disabled = false;
    btn.textContent = 'この内容で送信';
  }
}

// ---- イベント -------------------------------------------------------------

function bind() {
  editModal = new bootstrap.Modal($('editModal'));
  sendModal = new bootstrap.Modal($('sendModal'));

  $('fetchBtn').addEventListener('click', fetchObjects);
  $('addRouteBtn').addEventListener('click', () => addNew('route'));
  $('addRoute6Btn').addEventListener('click', () => addNew('route6'));
  document.querySelectorAll('[data-new]').forEach((el) => {
    el.addEventListener('click', (e) => { e.preventDefault(); addNew(el.dataset.new); });
  });
  $('touchAllBtn').addEventListener('click', touchAllVisible);

  $('classFilter').addEventListener('change', (e) => {
    state.classFilter = e.target.value;
    render();
  });
  $('filterInput').addEventListener('input', (e) => {
    state.filter = e.target.value.trim();
    render();
  });
  $('staleOnly').addEventListener('change', (e) => {
    state.staleOnly = e.target.checked;
    render();
  });

  $('saveCfgBtn').addEventListener('click', async () => {
    try {
      const cfg = await api('/api/config', {
        method: 'POST',
        body: JSON.stringify({
          mntner: $('cfgMntner').value.trim(),
          from: $('cfgFrom').value.trim(),
          changedBy: $('cfgChanged').value.trim(),
          to: $('cfgTo').value.trim(),
          whoisServer: $('cfgWhois').value.trim(),
          notify: $('cfgNotify').value.trim(),
          defaultOrigin: $('cfgOrigin').value.trim(),
          password: $('cfgPassword').value,
          transport: $('cfgTransport').value,
          smtpHost: $('cfgSmtpHost').value.trim(),
          smtpPort: parseInt($('cfgSmtpPort').value, 10) || 0,
          smtpUser: $('cfgSmtpUser').value.trim(),
          smtpPassword: $('cfgSmtpPass').value.replace(/\s+/g, ''),
        }),
      });
      state.cfg = cfg;
      $('mntnerBadge').textContent = cfg.mntner || '未設定';
      showError('');
      showOk('設定を保存しました。');
      await loadState();
    } catch (e) {
      showError(e.message);
    }
  });

  $('authBtn').addEventListener('click', async () => {
    try {
      const res = await api('/api/auth/url');
      window.open(res.url, '_blank', 'noopener');
      // 別タブでの同意が終わる頃合いを見て状態を取り直す。
      const timer = setInterval(async () => {
        await loadState();
        if (state.authorized) clearInterval(timer);
      }, 2000);
      setTimeout(() => clearInterval(timer), 180000);
    } catch (e) {
      showError(e.message);
    }
  });

  $('cfgTransport').addEventListener('change', updateTransportUI);
  $('clearSmtpPass').addEventListener('click', async () => {
    await api('/api/smtp/clear', { method: 'POST' });
    $('cfgSmtpPass').value = '';
    await loadState();
  });

  $('logoutBtn').addEventListener('click', async () => {
    await api('/api/auth/logout', { method: 'POST' });
    await loadState();
  });

  $('editSave').addEventListener('click', saveEditor);
  $('addAttrBtn').addEventListener('click', () => {
    const name = $('attrPicker').value;
    if (!name) return;
    state.draft.push({ name, value: '' });
    renderDraft();
  });

  $('reviewBtn').addEventListener('click', openSend);
  $('refreshPreview').addEventListener('click', refreshPreview);
  $('sendSubmit').addEventListener('click', doSend);

  $('resetBtn').addEventListener('click', () => {
    if (!confirm('編集内容をすべて破棄して、取得直後の状態に戻します。')) return;
    state.items = state.items
      .filter((i) => i.action !== 'create')
      .map((i) => ({ ...i, cur: structuredClone(i.orig), action: 'none' }));
    showError('');
    render();
  });

  window.addEventListener('beforeunload', (e) => {
    if (changeList().length) e.preventDefault();
  });
}

// Bootstrap 5.3 のダークテーマを OS 設定に追随させる。
function applyTheme() {
  const dark = window.matchMedia('(prefers-color-scheme: dark)');
  const set = () => document.documentElement.setAttribute('data-bs-theme', dark.matches ? 'dark' : 'light');
  set();
  dark.addEventListener('change', set);
}

applyTheme();
bind();
loadState();
render();
