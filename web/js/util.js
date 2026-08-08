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
    el.style.background = colorForTitle(item.title);
    el.textContent = initialForTitle(item.title);
  }
  return el;
}

function formatCountdown(nextAiringAt) {
  if (!nextAiringAt) return '';
  const diff = new Date(nextAiringAt).getTime() - Date.now();
  if (diff <= 0) return '放送中';
  const hours = Math.ceil(diff / 3600000);
  if (hours < 24) return `あと${hours}時間`;
  return `あと${Math.ceil(hours / 24)}日`;
}

function unitFor(mediaType) {
  return mediaType === 'anime' ? '話' : '巻';
}

function progressLabel(item) {
  const unit = unitFor(item.mediaType);
  return item.total ? `${item.progress}${unit} / ${item.total}${unit}` : `${item.progress}${unit}まで`;
}

function el(tag, props = {}, children = []) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props)) {
    if (key === 'className') node.className = value;
    else if (key === 'style') Object.assign(node.style, value);
    else if (key.startsWith('on')) node.addEventListener(key.slice(2).toLowerCase(), value);
    else node.setAttribute(key, value);
  }
  for (const child of [].concat(children)) {
    if (child == null) continue;
    node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return node;
}
