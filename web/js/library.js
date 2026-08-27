// library.js — マイライブラリ画面
const libState = {
  type: 'anime',
  status: 'active',
  activeGenres: [],
  allRecords: [], // 現在の type の全ステータス分
  aniListInfo: new Map(), // anilistId -> AniList最新情報（総話数・巻数・放送状況・次話）
  pendingIds: new Set(), // 更新中のレコードID。連打で古いprogressのまま二重更新するのを防ぐ。
  editingId: null, // 話数を直接入力中のレコードID（大きな話数への一気ジャンプ用）
};

function initLibraryPage() {
  for (const input of document.querySelectorAll('#lib-type-tabs input[name="libtype"]')) {
    input.addEventListener('change', () => {
      libState.type = input.value;
      libState.activeGenres = [];
      syncTypeUI();
      syncStatusUI(); // タブのラベル（見た/見てる/見たい ⇔ 読んだ/読んでる/読みたい）も種別に合わせて更新する
      loadAndRender();
    });
  }

  for (const input of document.querySelectorAll('#lib-status-tabs input[name="libstatus"]')) {
    input.addEventListener('change', () => {
      libState.status = input.value;
      syncStatusUI();
      render();
    });
  }

  syncTypeUI();
  syncStatusUI();
  loadAndRender();
}

function syncTypeUI() {
  for (const opt of document.querySelectorAll('#lib-type-tabs .seg-opt')) {
    const input = opt.querySelector('input');
    opt.classList.toggle('checked', input.value === libState.type);
  }
}

function syncStatusUI() {
  const labels = STATUS_LABELS[libState.type];
  const opts = document.querySelectorAll('#lib-status-tabs .seg-opt');
  const order = ['done', 'active', 'want'];
  opts.forEach((opt, i) => {
    const input = opt.querySelector('input');
    const key = order[i];
    input.value = key;
    opt.querySelector('.seg-label').textContent = labels[key];
    opt.classList.toggle('checked', key === libState.status);
  });
}

async function loadAndRender() {
  const container = document.getElementById('lib-results');
  try {
    libState.allRecords = await api.listRecords({ type: libState.type });
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }

  libState.aniListInfo = new Map();
  if (libState.allRecords.length > 0) {
    try {
      const ids = libState.allRecords.map((r) => r.anilistId);
      const fresh = await api.mediaByIds({ type: libState.type, ids });
      libState.aniListInfo = new Map(fresh.map((f) => [f.anilistId, f]));
    } catch {
      // AniList側の取得に失敗しても、保存済みの記録だけで表示を続ける。
    }
  }

  render();
}

function render() {
  renderGenreChips();
  renderResults();
}

function renderGenreChips() {
  const container = document.getElementById('lib-genre-chips');
  const allGenres = Array.from(new Set(libState.allRecords.flatMap((r) => r.genres)));

  if (allGenres.length === 0) {
    container.replaceChildren();
    return;
  }

  const chips = allGenres.map((g) =>
    el(
      'span',
      {
        className: `tag ${libState.activeGenres.includes(g) ? 'tag-accent' : 'tag-outline'}`,
        onClick: () => {
          libState.activeGenres = libState.activeGenres.includes(g)
            ? libState.activeGenres.filter((x) => x !== g)
            : [...libState.activeGenres, g];
          render();
        },
      },
      translateGenre(g)
    )
  );
  container.replaceChildren(el('div', { style: { display: 'flex', gap: '6px', flexWrap: 'wrap' } }, chips));
}

function renderResults() {
  const container = document.getElementById('lib-results');
  let pool = libState.allRecords.filter((r) => r.status === libState.status);
  if (libState.activeGenres.length > 0) {
    pool = pool.filter((r) => r.genres.some((g) => libState.activeGenres.includes(g)));
  }

  if (pool.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品がありません。'));
    return;
  }

  const grid = el('div', { className: 'grid-cards' });
  for (const item of pool) {
    grid.appendChild(renderLibraryCard(item));
  }
  container.replaceChildren(grid);
}

function renderLibraryCard(item) {
  const thumb = thumbEl(item, { width: '56px', height: '74px', fontSize: 20 });
  thumb.style.borderRadius = 'var(--radius-sm)';

  const genreTags = el(
    'div',
    { style: { display: 'flex', gap: '4px', flexWrap: 'wrap', marginTop: '4px' } },
    item.genres.map((g) => el('span', { className: 'tag tag-neutral' }, translateGenre(g)))
  );

  const header = el('div', { style: { display: 'flex', gap: 'var(--space-3)' } }, [
    thumb,
    el('div', { style: { minWidth: '0', flex: '1' } }, [el('div', { className: 'card-title', style: { fontSize: '15px' } }, item.title), genreTags]),
  ]);

  const children = [header];

  if (item.status === 'active') {
    children.push(renderProgressBlock(item));
  }

  return el('div', { className: 'card elev-sm' }, children);
}

// アニメが「放送中」のとき、AniListのnextAiringEpisode（次に放送される話数）から
// 現在放送済みの話数（= 次話 - 1）を導く。
function currentAiringEpisode(freshInfo) {
  if (!freshInfo || freshInfo.airingStatus !== 'RELEASING' || freshInfo.nextEpisode == null) return null;
  return freshInfo.nextEpisode - 1;
}

function renderProgressBlock(item) {
  const unit = unitFor(item.mediaType);
  const freshInfo = libState.aniListInfo.get(item.anilistId);
  const total = freshInfo?.total ?? item.total ?? null;
  const airingEp = item.mediaType === 'anime' ? currentAiringEpisode(freshInfo) : null;

  const rows = [];

  if (airingEp != null) {
    let text = `${item.progress}${unit}視聴 / 現在${airingEp}${unit}放送中`;
    if (total) text += ` / 全${total}${unit}`;
    rows.push(el('div', { className: 'text-muted', style: { fontSize: '12px' } }, text));
  } else if (item.mediaType === 'manga' && freshInfo?.volumes) {
    // 話数（chapters）が進捗の追跡単位だが、参考情報として既刊巻数も表示する。
    rows.push(el('div', { className: 'text-muted', style: { fontSize: '12px' } }, `既刊${freshInfo.volumes}巻`));
  }

  // 放送中は、まだ放送されていない話数までは視聴済みにできないよう、
  // スライダー/直接入力の上限を現在放送済み話数（airingEp）に制限する。放送終了後は総話数(total)が上限。
  const progressCap = airingEp != null ? airingEp : total;
  const pending = libState.pendingIds.has(item.id);
  const isEditing = libState.editingId === item.id;
  const totalLabel = total ? `${total}${unit}` : `？${unit}`;

  // 中央の数字表示：通常はクリックで直接入力に切り替わる表示、編集中は入力欄そのもの。
  let countEl;
  if (isEditing) {
    // 話数が数百に及ぶ漫画では＋/－やスライダーの微調整だけでは非効率なため、
    // クリックで直接入力に切り替えて一気にジャンプできるようにする。
    const input = el('input', {
      type: 'number',
      min: '0',
      className: 'input input-progress-edit',
      value: item.progress,
    });
    const commit = () => {
      const n = parseInt(input.value, 10);
      libState.editingId = null;
      if (!Number.isNaN(n)) {
        commitProgress(item, n, total, progressCap);
      } else {
        renderResults();
      }
    };
    input.addEventListener('blur', commit);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') input.blur();
      if (e.key === 'Escape') {
        libState.editingId = null;
        renderResults();
      }
    });
    queueMicrotask(() => {
      input.focus();
      input.select();
    });
    countEl = el('div', { style: { display: 'flex', alignItems: 'center', gap: '3px' } }, [input, el('span', { style: { fontSize: '12px' } }, `/ ${totalLabel}`)]);
  } else {
    countEl = el(
      'span',
      {
        className: 'progress-count',
        style: { cursor: 'pointer' },
        onClick: () => {
          libState.editingId = item.id;
          renderResults();
        },
      },
      `${item.progress} / ${totalLabel}`
    );
  }

  const minusBtn = el(
    'button',
    {
      className: 'btn btn-secondary btn-step',
      disabled: pending || isEditing || item.progress <= 0,
      onClick: () => commitProgress(item, item.progress - 1, total, progressCap),
    },
    '－'
  );
  const plusBtn = el(
    'button',
    {
      className: 'btn btn-secondary btn-step',
      disabled: pending || isEditing || (progressCap != null && item.progress >= progressCap),
      onClick: () => commitProgress(item, item.progress + 1, total, progressCap),
    },
    '＋'
  );

  rows.push(el('div', { className: 'progress-row' }, [minusBtn, countEl, plusBtn]));

  if (!isEditing && progressCap != null && progressCap > 0) {
    // スライダーはドラッグ中（input）はローカル表示だけ更新し、離した時（change）にAPIへ反映する。
    // ドラッグのたびにカード全体を再描画すると操作感が壊れるため、countEl のテキストを直接書き換える。
    const slider = el('input', {
      type: 'range',
      min: '0',
      max: String(progressCap),
      value: String(Math.min(item.progress, progressCap)),
      className: 'progress-slider',
      disabled: pending,
      oninput: (e) => {
        countEl.textContent = `${e.target.value} / ${totalLabel}`;
      },
      onchange: (e) => {
        commitProgress(item, parseInt(e.target.value, 10), total, progressCap);
      },
    });
    rows.push(el('div', { className: 'progress-slider-row' }, [slider]));
  }

  if (total) {
    const pct = Math.min(100, Math.round((item.progress / total) * 100));
    rows.push(el('div', { className: 'progress-pct' }, `${pct}%`));
  }

  return el('div', { className: 'progress-block' }, rows);
}

// progressCap: 放送中なら現在放送済み話数、それ以外は総話数（未定ならnull）。
// スライダーのmax属性でも同じ上限にしているが、ここでも同じ上限で clamp しておくことで、
// 更新処理自体が上限を知らないまま呼ばれても未放送分まで視聴済みにしてしまわないようにする。
async function commitProgress(item, rawN, total, progressCap) {
  if (libState.pendingIds.has(item.id)) return;

  let n = Math.max(0, rawN);
  if (progressCap != null) n = Math.min(n, progressCap);
  if (n === item.progress) return;

  const body = { progress: n };
  if (total != null && n >= total) {
    body.status = 'done';
  }

  libState.pendingIds.add(item.id);
  renderResults(); // ボタンを即座に disabled にして連打による二重更新を防ぐ

  try {
    await api.updateRecord(item.id, body);
  } catch (err) {
    alert(`更新に失敗しました: ${err.message}`);
    libState.pendingIds.delete(item.id);
    renderResults();
    return;
  }

  // AniListの最新情報（総話数・放送状況）は progress の変更では変わらないので、
  // ライブラリ全体とAniList情報をまるごと再取得せず、ローカルの状態だけ更新して再描画する。
  item.progress = n;
  if (body.status) item.status = body.status;
  libState.pendingIds.delete(item.id);
  renderResults();
}

window.authReadyPromise.then((user) => {
  if (!user) return; // 未ログイン: auth.js がログイン画面へリダイレクト中
  initLibraryPage();
});
