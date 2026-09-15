// home.js — ホーム画面（棚型レイアウト）
// 公開データ（ログイン不要）: 今週の放送予定 / 今季放送中アニメ
// ログイン後: つづきを見る/読む・読んでる漫画・あなたへのおすすめ
const homeState = { kind: 'anime', user: null, season: [], library: [], ranking: [], activeMagazine: null, rankingCache: null };

async function renderHome(user) {
  homeState.user = user;
  homeState.kind = getKind();
  initKindToggle((kind) => {
    homeState.kind = kind;
    loadAndRender();
  });
  await loadAndRender();
}

async function loadAndRender() {
  const { kind, user } = homeState;

  document.getElementById('continue-title').textContent = kind === 'anime' ? 'つづきを見る' : 'つづきを読む';
  document.getElementById('schedule-shelf').style.display = kind === 'anime' ? '' : 'none';
  document.getElementById('season-shelf').style.display = kind === 'anime' ? '' : 'none';
  document.getElementById('ranking-shelf').style.display = kind === 'manga' ? '' : 'none';
  document.getElementById('magazine-shelf').style.display = kind === 'manga' ? '' : 'none';

  try {
    homeState.season = kind === 'anime' ? await api.seasonAnime() : [];
  } catch (err) {
    homeState.season = [];
    document.getElementById('season-list').replaceChildren(el('p', { className: 'text-muted' }, `AniListからの取得に失敗しました: ${err.message}`));
  }

  homeState.activeMagazine = null;
  try {
    if (kind === 'manga') {
      if (!homeState.rankingCache) {
        homeState.rankingCache = await api.trendingManga();
      }
      homeState.ranking = homeState.rankingCache;
    } else {
      homeState.ranking = [];
    }
  } catch (err) {
    homeState.ranking = [];
    document.getElementById('ranking-list').replaceChildren(el('p', { className: 'text-muted' }, `ランキングの取得に失敗しました: ${err.message}`));
  }

  homeState.library = [];
  if (user) {
    try {
      homeState.library = await api.listRecords({ type: kind });
    } catch {
      homeState.library = [];
    }
  }

  render();
}

function render() {
  renderContinueShelf();
  renderScheduleShelf();
  renderSeasonShelf();
  renderRankingShelf();
  renderMagazineShelf();
  renderRecommendPreview();
}

function libraryByAniListId() {
  return new Map(homeState.library.map((r) => [r.anilistId, r]));
}

function openLibraryItemModal(record) {
  openWorkModal({
    item: record,
    mediaType: homeState.kind,
    record,
    user: homeState.user,
    onStatusChange: loadAndRender,
    onChange: loadAndRender,
  });
}

function openDiscoveryItemModal(item) {
  const record = libraryByAniListId().get(item.anilistId) || null;
  openWorkModal({
    item,
    mediaType: homeState.kind,
    record,
    user: homeState.user,
    onChange: loadAndRender,
  });
}

function renderContinueShelf() {
  const container = document.getElementById('continue-list');
  const { user, kind, library } = homeState;

  if (!user) {
    container.replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, [
        'ログインすると、記録中の作品からつづきを再開できます。 ',
        el('a', { href: '/login.html' }, 'ログイン'),
      ])
    );
    return;
  }

  const unit = unitFor();
  const active = library.filter((r) => r.status === 'active');
  if (active.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, `記録中の${kind === 'anime' ? 'アニメ' : '漫画'}はまだありません。検索から追加できます。`));
    return;
  }

  container.replaceChildren(
    ...active.map((r) => {
      const pct = r.total ? Math.min(100, Math.round((r.progress / r.total) * 100)) : 0;
      const caption = r.total ? `${r.progress} / ${r.total}${unit}` : `${r.progress}${unit}まで`;
      return posterCardEl(r, { pct, caption, onClick: () => openLibraryItemModal(r) });
    })
  );
}

function renderScheduleShelf() {
  const container = document.getElementById('schedule-list');
  if (homeState.kind !== 'anime') return;

  const withSchedule = homeState.season.filter((a) => a.nextAiringAt);
  if (withSchedule.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '放送予定の情報がありません。'));
    return;
  }

  const todayName = new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Tokyo', weekday: 'short' }).format(new Date());
  const todayIndex = WEEKDAY_EN_SHORT.indexOf(todayName);

  const byDay = new Map();
  for (const a of withSchedule) {
    const info = jstAiringInfo(a.nextAiringAt);
    if (!byDay.has(info.dayIndex)) byDay.set(info.dayIndex, []);
    byDay.get(info.dayIndex).push({ item: a, hour: info.hour, minute: info.minute });
  }

  const record = libraryByAniListId();
  const days = Array.from(byDay.keys()).sort((a, b) => a - b);

  container.replaceChildren(
    ...days.map((dayIndex) => {
      const items = byDay.get(dayIndex).sort((a, b) => a.hour - b.hour);
      const isToday = dayIndex === todayIndex;
      return el(
        'div',
        { className: 'card elev-sm', style: { flex: 'none', width: '150px', padding: 'var(--space-3)', gap: '10px', borderColor: isToday ? 'var(--color-accent)' : undefined } },
        [
          el('div', { style: { display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' } }, [
            el('span', { style: { fontFamily: 'var(--font-heading)', fontSize: '14px', fontWeight: '600', color: isToday ? 'var(--color-accent)' : 'var(--color-text)' } }, isToday ? `${WEEKDAY_JA[dayIndex]} 今日` : WEEKDAY_JA[dayIndex]),
            el('span', { className: 'text-muted', style: { fontSize: '11px' } }, `${items.length}件`),
          ]),
          ...items.map(({ item, hour, minute }) =>
            el('div', { style: { display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }, onClick: () => openDiscoveryItemModal(item) }, [
              thumbEl(item, { width: '24px', height: '32px', className: 'thumb', fontSize: 12 }),
              el('div', { style: { minWidth: '0' } }, [
                el('div', { style: { fontSize: '11px', lineHeight: '1.3', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, item.title),
                el('div', { className: 'text-muted', style: { fontSize: '10px', marginTop: '2px' } }, `${hour}:${minute}${record.has(item.anilistId) ? ' ・ 記録済み' : ''}`),
              ]),
            ])
          ),
        ]
      );
    })
  );
}

function renderSeasonShelf() {
  const container = document.getElementById('season-list');
  if (homeState.kind !== 'anime') return;
  if (homeState.season.length === 0) return;

  const record = libraryByAniListId();
  container.replaceChildren(
    ...homeState.season.map((item) => {
      const r = record.get(item.anilistId);
      return posterCardEl(item, {
        badgeLabel: r ? `${STATUS_LABELS.anime[r.status]} ✓` : null,
        caption: formatWeekday(item.nextAiringAt) || '放送日未定',
        onClick: () => openDiscoveryItemModal(item),
      });
    })
  );
}

function rankingPosterEl(item, rank) {
  const thumb = thumbEl(item, { fontSize: 26, className: 'poster-thumb' });
  const score = formatScore(item.score);
  const captionParts = [score ? `★${score}` : null, item.malRank ? `MAL #${item.malRank}` : null].filter(Boolean);
  const children = [
    el('span', { className: 'poster-rank-num' }, String(rank)),
    thumb,
    el('div', { className: 'poster-title' }, item.title),
  ];
  if (captionParts.length > 0) {
    children.push(el('div', { className: 'text-muted', style: { fontSize: '11px', marginTop: '4px' } }, captionParts.join(' ・ ')));
  }
  if (item.members) {
    children.push(el('div', { className: 'text-muted', style: { fontSize: '11px', marginTop: '2px' } }, `${Math.round(item.members / 1000)}千人が記録`));
  }
  return el('div', { className: 'poster-card poster-card-ranked', onClick: () => openDiscoveryItemModal(item) }, children);
}

function renderRankingShelf() {
  const container = document.getElementById('ranking-list');
  if (homeState.kind !== 'manga') return;

  if (homeState.ranking.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, 'ランキング情報を取得できませんでした。'));
    return;
  }

  container.replaceChildren(...homeState.ranking.map((item, i) => rankingPosterEl(item, i + 1)));
}

function renderMagazineShelf() {
  const chipsContainer = document.getElementById('magazine-chips');
  const listContainer = document.getElementById('magazine-list');
  const descEl = document.getElementById('magazine-desc');
  if (homeState.kind !== 'manga') return;

  const counts = new Map();
  for (const item of homeState.ranking) {
    for (const mag of item.magazines || []) counts.set(mag, (counts.get(mag) || 0) + 1);
  }
  const magazines = Array.from(counts.keys()).sort((a, b) => counts.get(b) - counts.get(a));

  if (magazines.length === 0) {
    descEl.textContent = '';
    chipsContainer.replaceChildren();
    listContainer.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '掲載誌の情報を取得できませんでした。'));
    return;
  }

  if (!homeState.activeMagazine || !counts.has(homeState.activeMagazine)) {
    homeState.activeMagazine = magazines[0];
  }
  const active = homeState.activeMagazine;

  descEl.textContent = `${active} に載っている作品`;
  chipsContainer.replaceChildren(
    ...magazines.map((mag) =>
      el(
        'button',
        {
          type: 'button',
          className: `tag ${mag === active ? 'tag-accent' : 'tag-outline'}`,
          onClick: () => {
            homeState.activeMagazine = mag;
            renderMagazineShelf();
          },
        },
        `${mag} (${counts.get(mag)})`
      )
    )
  );

  const works = homeState.ranking.filter((item) => (item.magazines || []).includes(active));
  listContainer.replaceChildren(
    ...works.map((item) => {
      const score = formatScore(item.score);
      return posterCardEl(item, {
        caption: score ? `★${score}` : null,
        onClick: () => openDiscoveryItemModal(item),
      });
    })
  );
}

async function renderRecommendPreview() {
  const container = document.getElementById('rec-preview-list');
  const { user, kind, library } = homeState;

  if (!user) {
    container.replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, [
        'ログインすると、あなたの好みに合わせたおすすめが表示されます。 ',
        el('a', { href: '/login.html' }, 'ログイン'),
      ])
    );
    return;
  }

  const counts = new Map();
  for (const r of library) {
    if (r.status !== 'done' && r.status !== 'active') continue;
    for (const g of r.genres) counts.set(g, (counts.get(g) || 0) + 1);
  }
  const genres = Array.from(counts.entries()).sort((a, b) => b[1] - a[1]).slice(0, 3).map(([g]) => g);

  if (genres.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '記録が増えると、あなたの好みに合わせたおすすめが表示されます。'));
    return;
  }

  let results = [];
  try {
    results = await api.recommendations({ type: kind, genres });
  } catch {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, 'おすすめの取得に失敗しました。'));
    return;
  }

  const registeredIds = new Set(library.map((r) => r.anilistId));
  const preview = results.filter((item) => !registeredIds.has(item.anilistId)).slice(0, 6);
  if (preview.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, 'おすすめできる未登録の作品が見つかりませんでした。'));
    return;
  }

  container.replaceChildren(
    ...preview.map((item) => {
      const score = formatScore(item.score);
      return posterCardEl(item, {
        caption: score ? `★${score}` : null,
        onClick: () =>
          openWorkModal({
            item,
            mediaType: kind,
            record: null,
            user,
            onChange: loadAndRender,
          }),
      });
    })
  );
}

window.authReadyPromise.then(renderHome);
