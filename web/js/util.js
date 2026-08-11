// util.js — 表示用の小さなヘルパー群
const THUMB_PALETTE = ['#b5abfc', '#9690c9', '#7972a9', '#5c5783', '#d2cefd', '#b5afe8', '#423e5d'];

function colorForTitle(title) {
  let hash = 0;
  for (let i = 0; i < title.length; i++) {
    hash = (hash * 31 + title.charCodeAt(i)) >>> 0;
  }
  return THUMB_PALETTE[hash % THUMB_PALETTE.length];
}

function initialForTitle(title) {
  return title.trim().charAt(0) || '?';
}

// サムネイル用の要素を作る。カバー画像があればそれを、無ければ頭文字+色のプレースホルダーを表示する。
function thumbEl(item, { width, height, fontSize }) {
  const el = document.createElement('div');
  el.className = 'thumb';
  el.style.width = width;
  el.style.height = height;
  el.style.fontSize = fontSize + 'px';
  if (item.coverImageUrl) {
    el.style.backgroundImage = `url("${item.coverImageUrl}")`;
  } else {
    const title = displayTitle(item);
    el.style.background = colorForTitle(title);
    el.textContent = initialForTitle(title);
  }
  return el;
}

// タイトル表示言語（JA/EN）の設定。AniList検索結果は title(ja) / titleEn を両方持つ。
const LANG_STORAGE_KEY = 'lang';

function getLang() {
  return localStorage.getItem(LANG_STORAGE_KEY) === 'en' ? 'en' : 'ja';
}

function setLang(lang) {
  localStorage.setItem(LANG_STORAGE_KEY, lang);
}

function displayTitle(item) {
  if (getLang() === 'en' && item.titleEn) return item.titleEn;
  return item.title;
}

// サイドバーの id="lang-toggle" があるページで JA/EN 切り替えボタンを有効化する。
function initLangToggle() {
  const container = document.getElementById('lang-toggle');
  if (!container) return;

  const sync = () => {
    for (const btn of container.querySelectorAll('[data-lang]')) {
      btn.classList.toggle('checked', btn.dataset.lang === getLang());
    }
  };

  container.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-lang]');
    if (!btn || btn.dataset.lang === getLang()) return;
    setLang(btn.dataset.lang);
    location.reload();
  });

  sync();
}

initLangToggle();

function formatCountdown(nextAiringAt) {
  if (!nextAiringAt) return '';
  const diff = new Date(nextAiringAt).getTime() - Date.now();
  if (diff <= 0) return '放送中';
  const hours = Math.ceil(diff / 3600000);
  if (hours < 24) return `あと${hours}時間`;
  return `あと${Math.ceil(hours / 24)}日`;
}

// AniList のジャンル名（英語）→ 表示用の日本語ラベル。未知のジャンルはそのまま表示する。
const GENRE_JA = {
  Action: 'アクション',
  Adventure: '冒険',
  Comedy: 'コメディ',
  Drama: 'ドラマ',
  Ecchi: 'エッチ',
  Fantasy: 'ファンタジー',
  Horror: 'ホラー',
  'Mahou Shoujo': '魔法少女',
  Mecha: 'ロボット',
  Music: '音楽',
  Mystery: 'ミステリー',
  Psychological: 'サイコロジカル',
  Romance: '恋愛',
  'Sci-Fi': 'SF',
  'Slice of Life': '日常',
  Sports: 'スポーツ',
  Supernatural: '超自然',
  Thriller: 'スリラー',
};

function translateGenre(genre) {
  return GENRE_JA[genre] || genre;
}

const WEEKDAY_JA = ['日', '月', '火', '水', '木', '金', '土'];

// nextAiringAt（unix秒 or ISO文字列）から「木 25:00」のような表示用ラベルを作る。
// 24時以降（深夜アニメの慣例表記）は前日の曜日として扱う。
function formatWeekday(nextAiringAt) {
  if (!nextAiringAt) return '';
  const ms = typeof nextAiringAt === 'number' ? nextAiringAt * 1000 : new Date(nextAiringAt).getTime();
  const d = new Date(ms);
  let hour = d.getHours();
  let dayIndex = d.getDay();
  if (hour < 4) {
    hour += 24;
    dayIndex = (dayIndex + 6) % 7;
  }
  const minute = String(d.getMinutes()).padStart(2, '0');
  return `${WEEKDAY_JA[dayIndex]} ${hour}:${minute}`;
}

function formatScore(score) {
  if (score == null) return null;
  return (score / 10).toFixed(1);
}

function unitFor(mediaType) {
  return mediaType === 'anime' ? '話' : '巻';
}

function el(tag, props = {}, children = []) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props)) {
    if (key === 'className') node.className = value;
    else if (key === 'style') Object.assign(node.style, value);
    else if (key.startsWith('on')) node.addEventListener(key.slice(2).toLowerCase(), value);
    else if (typeof value === 'boolean') node[key] = value; // disabled, checked, autofocus など
    else node.setAttribute(key, value);
  }
  for (const child of [].concat(children)) {
    if (child == null) continue;
    node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return node;
}
