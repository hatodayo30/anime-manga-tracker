// home.js — ホーム画面（パターンD：情報密度×ダッシュボード）
// 公開エリア（ログイン不要）: 今季放送中アニメ / 全体人気ランキング / 積み作品＋今季人気TOP5（今季人気分のみ）
// 非公開エリア（ログイン後）: 統計サマリー・積み作品（「見たい」ステータス）・各カードの登録状況バッジ
async function renderHome(user) {
  let season = [];
  let trending = [];
  try {
    [season, trending] = await Promise.all([api.seasonAnime(), api.trending()]);
  } catch (err) {
    document.getElementById('season-anime-list').replaceChildren(
      el('p', { className: 'text-muted' }, `AniListからの取得に失敗しました: ${err.message}`)
    );
    return;
  }

  let libraryRecords = [];
  if (user) {
    try {
      libraryRecords = await api.listRecords({ type: 'anime' });
    } catch {
      libraryRecords = [];
    }
  }
  const libraryByAniListId = new Map(libraryRecords.map((r) => [r.anilistId, r]));

  renderStatBadges(user, libraryRecords);
  renderSeasonRow(season, libraryByAniListId, user);
  renderRanking(trending, libraryByAniListId, user);
  renderSideList(libraryRecords, season, user);
}

function renderStatBadges(user, libraryRecords) {
  const container = document.getElementById('stat-badges');
  if (!user) {
    container.replaceChildren();
    return;
  }

  const doneCount = libraryRecords.filter((r) => r.status === 'done').length;
  const activeRecords = libraryRecords.filter((r) => r.status === 'active');
  const activeCount = activeRecords.length;

  const withTotal = activeRecords.filter((r) => r.total);
  const seasonRate = withTotal.length
    ? Math.round(
        (withTotal.reduce((sum, r) => sum + r.progress / r.total, 0) / withTotal.length) * 100
      )
    : 0;

  const badges = [
    { label: '見た本数', value: `${doneCount}本` },
    { label: '見てる本数', value: `${activeCount}本` },
    { label: '今季消化率', value: `${seasonRate}%` },
  ];
  container.replaceChildren(
    ...badges.map((b) => el('div', { className: 'tag tag-accent stat-badge' }, `${b.label}：${b.value}`))
  );
}

function statusBadgeLabel(mediaType, status) {
  const labels = { anime: { done: '見た', active: '見てる', want: '見たい' } };
  return `${labels[mediaType][status]} ✓`;
}

function renderSeasonRow(season, libraryByAniListId, user) {
  const container = document.getElementById('season-anime-list');
  if (season.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品がありません。'));
    return;
  }

  container.replaceChildren(
    ...season.map((item) => {
      const record = libraryByAniListId.get(item.anilistId);
      const thumb = thumbEl(item, { width: '100%', height: '112px', fontSize: 28 });

      const children = [thumb];
      if (record) {
        children.push(el('span', { className: 'tag tag-accent card-badge' }, statusBadgeLabel('anime', record.status)));
      }
      children.push(
        el('div', { className: 'hrow-card-body' }, [
          el('div', { className: 'card-title', style: { fontSize: '14px' } }, displayTitle(item)),
          el(
            'div',
            { style: { display: 'flex', gap: '6px', marginTop: '6px' } },
            [
              el('span', { className: 'tag tag-neutral' }, formatWeekday(item.nextAiringAt) || '放送日未定'),
              item.nextEpisode ? el('span', { className: 'tag tag-neutral' }, `${item.nextEpisode}話`) : null,
            ]
          ),
        ])
      );

      return el('div', { className: 'card elev-sm hrow-card', onClick: () => openItemModal(item, record, user, () => renderHome(user)) }, children);
    })
  );
}

function renderRanking(trending, libraryByAniListId, user) {
  const container = document.getElementById('trending-list');
  if (trending.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品がありません。'));
    return;
  }

  container.replaceChildren(
    ...trending.map((item, i) => {
      const record = libraryByAniListId.get(item.anilistId);
      const thumb = thumbEl(item, { width: '36px', height: '36px', fontSize: 16 });
      thumb.style.borderRadius = 'var(--radius-sm)';

      const children = [
        el('div', { style: { fontFamily: 'var(--font-heading)', fontWeight: '700', fontSize: '18px', color: 'var(--color-accent)', width: '20px', flex: 'none' } }, String(i + 1)),
        thumb,
        el('div', { style: { flex: '1', minWidth: '0' } }, [
          el('div', { className: 'card-title', style: { fontSize: '14px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, displayTitle(item)),
        ]),
      ];
      if (record) {
        children.push(el('span', { className: 'tag tag-accent', style: { flex: 'none' } }, statusBadgeLabel('anime', record.status)));
      }
      const score = formatScore(item.score);
      if (score) {
        children.push(el('div', { className: 'text-muted', style: { fontSize: '12px', flex: 'none' } }, `★${score}`));
      }

      return el('div', { className: 'card elev-sm ranking-row', onClick: () => openItemModal(item, record, user, () => renderHome(user)) }, children);
    })
  );
}

function renderSideList(libraryRecords, season, user) {
  const container = document.getElementById('side-list');
  const tsumi = libraryRecords.filter((r) => r.status === 'want').slice(0, 5);
  const usedIds = new Set(tsumi.map((r) => r.anilistId));
  const seasonTop5 = season.filter((s) => !usedIds.has(s.anilistId)).slice(0, 5);

  if (tsumi.length === 0 && seasonTop5.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '表示できる作品がありません。'));
    return;
  }

  const rows = [];
  for (const item of tsumi) {
    const thumb = thumbEl(item, { width: '32px', height: '32px', fontSize: 13 });
    thumb.style.borderRadius = 'var(--radius-sm)';
    rows.push(
      el('div', { className: 'card elev-sm side-row' }, [
        thumb,
        el('div', { className: 'card-title', style: { fontSize: '13px' } }, item.title),
        el('span', { className: 'tag tag-neutral', style: { flex: 'none' } }, '積み'),
      ])
    );
  }
  for (const item of seasonTop5) {
    const thumb = thumbEl(item, { width: '32px', height: '32px', fontSize: 13 });
    thumb.style.borderRadius = 'var(--radius-sm)';
    rows.push(
      el('div', { className: 'card elev-sm side-row', onClick: () => openItemModal(item, null, user, () => renderHome(user)) }, [
        thumb,
        el('div', { className: 'card-title', style: { fontSize: '13px' } }, displayTitle(item)),
        el('span', { className: 'tag tag-neutral', style: { flex: 'none' } }, '今季'),
      ])
    );
  }
  container.replaceChildren(...rows);
}

function openItemModal(item, record, user, onStatusChange) {
  openWorkModal({
    item: { ...item, title: displayTitle(item) },
    mediaType: 'anime',
    status: record ? record.status : null,
    user,
    onStatusChange,
  });
}

window.authReadyPromise.then(renderHome);
