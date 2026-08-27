// util.js — 表示用の小さなヘルパー群

// ステータス（見た/見てる/見たい 等）の表示ラベル。検索・ライブラリ・モーダル・ホームで共通利用する。
const STATUS_LABELS = {
  anime: { done: '見た', active: '見てる', want: '見たい' },
  manga: { done: '読んだ', active: '読んでる', want: '読みたい' },
};

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
const WEEKDAY_EN_SHORT = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

// nextAiringAt（unix秒 or ISO文字列）から「木 25:00」のような表示用ラベルを作る。
// 放送時刻はJST（日本の深夜アニメ表記）前提のため、閲覧者のブラウザのタイムゾーンに関わらず
// 常にAsia/Tokyoとして曜日・時刻を計算する。
// 24時以降（深夜アニメの慣例表記）は前日の曜日として扱う。
function formatWeekday(nextAiringAt) {
  if (!nextAiringAt) return '';
  const ms = typeof nextAiringAt === 'number' ? nextAiringAt * 1000 : new Date(nextAiringAt).getTime();

  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Asia/Tokyo',
    weekday: 'short',
    hour: 'numeric',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(ms);
  const get = (type) => parts.find((p) => p.type === type)?.value;

  let hour = Number(get('hour'));
  let dayIndex = WEEKDAY_EN_SHORT.indexOf(get('weekday'));
  if (hour < 4) {
    hour += 24;
    dayIndex = (dayIndex + 6) % 7;
  }
  return `${WEEKDAY_JA[dayIndex]} ${hour}:${get('minute')}`;
}

function formatScore(score) {
  if (score == null) return null;
  return (score / 10).toFixed(1);
}

// 進捗の追跡単位。漫画もAniListのchaptersを使うため、アニメの話数と同じ「話」で統一する
// （「巻」だと巻数と誤認しやすく、実際の値は話数のため不一致になっていた）。
function unitFor() {
  return '話';
}

// ひらがな/カタカナ→ローマ字（ヘボン式簡易版）。AniListの検索対象は漢字(native)/ローマ字(romaji)/
// 英語(english)のみでひらがな・カタカナのタイトルは持たないため、かな入力でも検索できるよう
// ローマ字に変換してから検索するために使う（完全な変換規則である必要はない）。
const HIRAGANA_DIGRAPHS = {
  きゃ: 'kya', きゅ: 'kyu', きょ: 'kyo',
  しゃ: 'sha', しゅ: 'shu', しょ: 'sho',
  ちゃ: 'cha', ちゅ: 'chu', ちょ: 'cho',
  にゃ: 'nya', にゅ: 'nyu', にょ: 'nyo',
  ひゃ: 'hya', ひゅ: 'hyu', ひょ: 'hyo',
  みゃ: 'mya', みゅ: 'myu', みょ: 'myo',
  りゃ: 'rya', りゅ: 'ryu', りょ: 'ryo',
  ぎゃ: 'gya', ぎゅ: 'gyu', ぎょ: 'gyo',
  じゃ: 'ja', じゅ: 'ju', じょ: 'jo',
  びゃ: 'bya', びゅ: 'byu', びょ: 'byo',
  ぴゃ: 'pya', ぴゅ: 'pyu', ぴょ: 'pyo',
  // カタカナの外来語表記（例: フォックス, ウィッチ, ジェット）をひらがな化した後の拗音。
  てぃ: 'ti', でぃ: 'di', とぅ: 'tu', どぅ: 'du',
  ふぁ: 'fa', ふぃ: 'fi', ふぇ: 'fe', ふぉ: 'fo',
  うぃ: 'wi', うぇ: 'we', うぉ: 'wo',
  ちぇ: 'che', しぇ: 'she', じぇ: 'je',
  つぁ: 'tsa', つぃ: 'tsi', つぇ: 'tse', つぉ: 'tso',
  くぁ: 'kwa', ぐぁ: 'gwa',
  ゔぁ: 'va', ゔぃ: 'vi', ゔぇ: 've', ゔぉ: 'vo', ゔゅ: 'vyu',
};
const HIRAGANA_MONOGRAPHS = {
  あ: 'a', い: 'i', う: 'u', え: 'e', お: 'o',
  か: 'ka', き: 'ki', く: 'ku', け: 'ke', こ: 'ko',
  さ: 'sa', し: 'shi', す: 'su', せ: 'se', そ: 'so',
  た: 'ta', ち: 'chi', つ: 'tsu', て: 'te', と: 'to',
  な: 'na', に: 'ni', ぬ: 'nu', ね: 'ne', の: 'no',
  は: 'ha', ひ: 'hi', ふ: 'fu', へ: 'he', ほ: 'ho',
  ま: 'ma', み: 'mi', む: 'mu', め: 'me', も: 'mo',
  や: 'ya', ゆ: 'yu', よ: 'yo',
  ら: 'ra', り: 'ri', る: 'ru', れ: 're', ろ: 'ro',
  わ: 'wa', を: 'wo', ん: 'n',
  が: 'ga', ぎ: 'gi', ぐ: 'gu', げ: 'ge', ご: 'go',
  ざ: 'za', じ: 'ji', ず: 'zu', ぜ: 'ze', ぞ: 'zo',
  だ: 'da', ぢ: 'ji', づ: 'zu', で: 'de', ど: 'do',
  ば: 'ba', び: 'bi', ぶ: 'bu', べ: 'be', ぼ: 'bo',
  ぱ: 'pa', ぴ: 'pi', ぷ: 'pu', ぺ: 'pe', ぽ: 'po',
  ぁ: 'a', ぃ: 'i', ぅ: 'u', ぇ: 'e', ぉ: 'o',
  ゔ: 'vu', ゖ: 'ke',
};

function containsHiragana(text) {
  return /[ぁ-ゖ]/.test(text);
}

function containsKatakana(text) {
  return /[ァ-ヺー]/.test(text);
}

function containsKana(text) {
  return containsHiragana(text) || containsKatakana(text);
}

function containsKanji(text) {
  return /[一-鿿々〆〤]/.test(text);
}

// カタカナ→ひらがな。長音記号「ー」はひらがなに対応する文字がないのでそのまま残し、
// 後段のローマ字変換で「直前の母音を繰り返す」処理に使う。
function katakanaToHiragana(text) {
  return text.replace(/[ァ-ヶ]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) - 0x60));
}

// ひらがな文字列をローマ字の「単位」の配列にトークナイズする。
// 促音「っ」と長音「ー」は直後/直前の文脈を見て初めて展開できるため、
// この段階では専用マーカーのまま残す。
function tokenizeHiragana(text) {
  const units = [];
  for (let i = 0; i < text.length; i++) {
    const two = text.slice(i, i + 2);
    if (HIRAGANA_DIGRAPHS[two]) {
      units.push(HIRAGANA_DIGRAPHS[two]);
      i++;
      continue;
    }
    const one = text[i];
    if (one === 'っ') {
      units.push('SOKUON'); // 促音: 次の単位の子音を重ねる
      continue;
    }
    if (one === 'ー') {
      units.push('CHOON'); // 長音: 直前の母音を繰り返す
      continue;
    }
    if (HIRAGANA_MONOGRAPHS[one] !== undefined) {
      units.push(HIRAGANA_MONOGRAPHS[one]);
      continue;
    }
    units.push(one); // 未対応の文字（漢字・英数字など）はそのまま通す。
  }
  return units;
}

function hiraganaToRomaji(text) {
  const units = tokenizeHiragana(text);

  let result = '';
  for (let i = 0; i < units.length; i++) {
    if (units[i] === 'SOKUON') {
      const next = units[i + 1];
      if (next && /^[a-z]/.test(next)) {
        result += next[0]; // 促音: 次の子音を重ねる（例: った -> tta）
      }
      continue;
    }
    if (units[i] === 'CHOON') {
      const lastVowel = result.match(/[aiueo](?=[^aiueo]*$)/); // これまでの結果の末尾側にある最後の母音
      if (lastVowel) result += lastVowel[0]; // 長音: 直前の母音を繰り返す（例: フリーレン -> furiiren）
      continue;
    }
    result += units[i];
  }
  return result;
}

// ひらがな・カタカナが混在していてもまとめてローマ字に変換する。
function kanaToRomaji(text) {
  return hiraganaToRomaji(katakanaToHiragana(text));
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
