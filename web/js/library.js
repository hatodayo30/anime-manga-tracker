// library.js — マイライブラリ画面
const LIB_STATUS_LABELS = {
  anime: { done: '見た', active: '見てる', want: '見たい' },
  manga: { done: '読んだ', active: '読んでる', want: '読みたい' },
};

const libState = {
  type: 'anime',
  status: 'active',
  activeGenres: [],
  editingId: null,
  allRecords: [], // 現在の type の全ステータス分
};

function initLibraryPage() {
  for (const input of document.querySelectorAll('#lib-type-tabs input[name="libtype"]')) {
    input.addEventListener('change', () => {
      libState.type = input.value;
      libState.activeGenres = [];
      syncTypeUI();
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
  const labels = LIB_STATUS_LABELS[libState.type];
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
    children.push(libState.editingId === item.id ? renderProgressEditor(item) : renderProgressDisplay(item));
  }

  return el('div', { className: 'card elev-sm' }, children);
}

function renderProgressDisplay(item) {
  return el(
    'div',
    {
      style: { marginTop: '4px', fontSize: '12px', color: 'color-mix(in srgb, var(--color-text) 70%, transparent)', cursor: 'pointer' },
      onClick: () => {
        libState.editingId = item.id;
        renderResults();
      },
    },
    progressLabel(item)
  );
}

function renderProgressEditor(item) {
  const unit = unitFor(item.mediaType);
  const suffix = item.total ? `${unit} / ${item.total}${unit}` : `${unit}まで`;

  const input = el('input', {
    className: 'input input-progress',
    type: 'number',
    min: '0',
    value: item.progress,
  });

  const commit = async () => {
    const n = Math.max(0, parseInt(input.value, 10) || 0);
    libState.editingId = null;
    if (n !== item.progress) {
      try {
        await api.updateRecord(item.id, { progress: n });
        item.progress = n;
      } catch (err) {
        alert(`更新に失敗しました: ${err.message}`);
      }
    }
    renderResults();
  };

  input.addEventListener('blur', commit);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') input.blur();
  });

  const wrapper = el('div', { style: { display: 'flex', alignItems: 'baseline', gap: '3px', marginTop: '4px', fontSize: '12px' } }, [
    input,
    el('span', {}, suffix),
  ]);
  queueMicrotask(() => input.focus());
  return wrapper;
}

window.authReadyPromise.then((user) => {
  if (!user) return; // 未ログイン: auth.js がログイン画面へリダイレクト中
  initLibraryPage();
});
