// library.js — マイライブラリ画面
const libState = {
  kind: 'anime',
  status: 'active',
  activeGenres: [],
  allRecords: [], // 現在の kind の全ステータス分
  user: null,
};

function initLibraryPage(user) {
  libState.user = user;
  libState.kind = getKind();
  initKindToggle((kind) => {
    libState.kind = kind;
    libState.activeGenres = [];
    syncStatusUI();
    loadAndRender();
  });

  for (const input of document.querySelectorAll('#lib-status-tabs input[name="libstatus"]')) {
    input.addEventListener('change', () => {
      libState.status = input.value;
      syncStatusUI();
      render();
    });
  }

  syncStatusUI();
  loadAndRender();
}

function syncStatusUI() {
  const labels = STATUS_LABELS[libState.kind];
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
    libState.allRecords = await api.listRecords({ type: libState.kind });
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }
  render();
}

function render() {
  renderStatCards();
  renderGenreDist();
  renderGenreChips();
  renderResults();
}

function renderStatCards() {
  const container = document.getElementById('lib-stat-cards');
  const labels = STATUS_LABELS[libState.kind];
  const counts = { done: 0, active: 0, want: 0 };
  for (const r of libState.allRecords) {
    if (counts[r.status] !== undefined) counts[r.status]++;
  }
  container.replaceChildren(
    ...['done', 'active', 'want'].map((key) =>
      el('div', { className: 'card elev-sm stat-card' }, [
        el('div', { className: 'card-meta' }, labels[key]),
        el('div', { style: { fontFamily: 'var(--font-heading)', fontSize: '24px', fontWeight: '600' } }, `${counts[key]}本`),
      ])
    )
  );
}

function renderGenreDist() {
  const container = document.getElementById('lib-genre-dist');
  const recorded = libState.allRecords.filter((r) => r.status);
  const counts = new Map();
  for (const r of recorded) {
    for (const g of r.genres) counts.set(g, (counts.get(g) || 0) + 1);
  }
  const total = Array.from(counts.values()).reduce((a, b) => a + b, 0);
  const top = Array.from(counts.entries()).sort((a, b) => b[1] - a[1]).slice(0, 3);

  if (top.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '12px', margin: '0' } }, 'まだ記録がありません。'));
    return;
  }

  container.replaceChildren(
    ...top.map(([genre, count]) => {
      const pct = Math.round((count / total) * 100);
      return el('div', {}, [
        el('div', { style: { display: 'flex', justifyContent: 'space-between', fontSize: '11px', marginBottom: '3px' } }, [
          el('span', {}, translateGenre(genre)),
          el('span', { className: 'text-muted' }, `${pct}%`),
        ]),
        el('div', { className: 'genre-dist-bar' }, [el('div', { className: 'genre-dist-bar-fill', style: { width: `${pct}%` } })]),
      ]);
    })
  );
}

function renderGenreChips() {
  const container = document.getElementById('lib-genre-chips');
  const pool = libState.allRecords.filter((r) => r.status === libState.status);
  const allGenres = Array.from(new Set(pool.flatMap((r) => r.genres)));

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

  const unit = unitFor();
  container.replaceChildren(
    el(
      'div',
      { className: 'poster-grid' },
      pool.map((r) => {
        const pct = r.status === 'active' && r.total ? Math.min(100, Math.round((r.progress / r.total) * 100)) : null;
        const caption = r.status === 'active'
          ? (r.total ? `${r.progress} / ${r.total}${unit}` : `${r.progress}${unit}まで`)
          : (r.genres || []).slice(0, 2).map(translateGenre).join('・');
        return posterCardEl(r, { pct, caption, onClick: () => openLibraryItemModal(r) });
      })
    )
  );
}

function openLibraryItemModal(record) {
  openWorkModal({
    item: record,
    mediaType: libState.kind,
    record,
    user: libState.user,
    onChange: loadAndRender,
  });
}

window.authReadyPromise.then((user) => {
  if (!user) return; // 未ログイン: auth.js がログイン画面へリダイレクト中
  initLibraryPage(user);
});
