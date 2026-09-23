export type MediaKind = 'anime' | 'manga'
export type Status = 'done' | 'active' | 'want'

export const STATUS_LABELS: Record<MediaKind, Record<Status, string>> = {
  anime: { done: '見た', active: '見てる', want: '見たい' },
  manga: { done: '読んだ', active: '読んでる', want: '読みたい' },
}

const THUMB_PALETTE = ['#b5abfc', '#9690c9', '#7972a9', '#5c5783', '#d2cefd', '#b5afe8', '#423e5d']

export function colorForTitle(title: string): string {
  let hash = 0
  for (let i = 0; i < title.length; i++) {
    hash = (hash * 31 + title.charCodeAt(i)) >>> 0
  }
  return THUMB_PALETTE[hash % THUMB_PALETTE.length]
}

export function initialForTitle(title: string): string {
  return title.trim().charAt(0) || '?'
}

const GENRE_JA: Record<string, string> = {
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
}

export function translateGenre(genre: string): string {
  return GENRE_JA[genre] || genre
}

const RELATION_JA: Record<string, string> = {
  SEQUEL: '続編',
  PREQUEL: '前日譚',
  SIDE_STORY: '外伝',
  SPIN_OFF: 'スピンオフ',
  PARENT: '原作',
  ALTERNATIVE: '別バージョン',
  ADAPTATION: '原作/アニメ化',
  SUMMARY: '総集編',
  FULL_STORY: '完全版',
  COMPILATION: 'コンピレーション',
  CONTAINS: '収録作品',
  CHARACTER: '関連キャラクター',
  OTHER: '関連作品',
  SOURCE: '原作',
}

export function translateRelation(relationType: string): string {
  return RELATION_JA[relationType] || relationType
}

export const WEEKDAY_JA = ['日', '月', '火', '水', '木', '金', '土']
export const WEEKDAY_EN_SHORT = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

export interface JstAiringInfo {
  dayIndex: number
  hour: number
  minute: string
}

// 放送時刻はJST（日本の深夜アニメ表記）前提のため、閲覧者のタイムゾーンに関わらず
// 常にAsia/Tokyoとして曜日・時刻を計算する。24時以降は前日の曜日として扱う。
export function jstAiringInfo(nextAiringAt: number | string | null | undefined): JstAiringInfo | null {
  if (!nextAiringAt) return null
  const ms = typeof nextAiringAt === 'number' ? nextAiringAt * 1000 : new Date(nextAiringAt).getTime()

  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Asia/Tokyo',
    weekday: 'short',
    hour: 'numeric',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(ms)
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? ''

  let hour = Number(get('hour'))
  let dayIndex = WEEKDAY_EN_SHORT.indexOf(get('weekday'))
  if (hour < 4) {
    hour += 24
    dayIndex = (dayIndex + 6) % 7
  }
  return { dayIndex, hour, minute: get('minute') }
}

export function formatWeekday(nextAiringAt: number | string | null | undefined): string {
  const info = jstAiringInfo(nextAiringAt)
  if (!info) return ''
  return `${WEEKDAY_JA[info.dayIndex]} ${info.hour}:${info.minute}`
}

export function formatScore(score: number | null | undefined): string | null {
  if (score == null) return null
  return (score / 10).toFixed(1)
}

// 進捗の追跡単位。漫画もAniListのchaptersを使うため、アニメの話数と同じ「話」で統一する。
export function unitFor(): string {
  return '話'
}

const HIRAGANA_DIGRAPHS: Record<string, string> = {
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
  てぃ: 'ti', でぃ: 'di', とぅ: 'tu', どぅ: 'du',
  ふぁ: 'fa', ふぃ: 'fi', ふぇ: 'fe', ふぉ: 'fo',
  うぃ: 'wi', うぇ: 'we', うぉ: 'wo',
  ちぇ: 'che', しぇ: 'she', じぇ: 'je',
  つぁ: 'tsa', つぃ: 'tsi', つぇ: 'tse', つぉ: 'tso',
  くぁ: 'kwa', ぐぁ: 'gwa',
  ゔぁ: 'va', ゔぃ: 'vi', ゔぇ: 've', ゔぉ: 'vo', ゔゅ: 'vyu',
}
const HIRAGANA_MONOGRAPHS: Record<string, string> = {
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
}

export function containsHiragana(text: string): boolean {
  return /[ぁ-ゖ]/.test(text)
}

export function containsKatakana(text: string): boolean {
  return /[ァ-ヺー]/.test(text)
}

export function containsKana(text: string): boolean {
  return containsHiragana(text) || containsKatakana(text)
}

export function containsKanji(text: string): boolean {
  return /[一-鿿々〆〤]/.test(text)
}

// カタカナ→ひらがな。長音記号「ー」はそのまま残し、後段で直前の母音を繰り返す処理に使う。
export function katakanaToHiragana(text: string): string {
  return text.replace(/[ァ-ヶ]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) - 0x60))
}

// 促音「っ」と長音「ー」は直後/直前の文脈依存のため、この段階では専用マーカーのまま残す。
function tokenizeHiragana(text: string): string[] {
  const units: string[] = []
  for (let i = 0; i < text.length; i++) {
    const two = text.slice(i, i + 2)
    if (HIRAGANA_DIGRAPHS[two]) {
      units.push(HIRAGANA_DIGRAPHS[two])
      i++
      continue
    }
    const one = text[i]
    if (one === 'っ') {
      units.push('SOKUON')
      continue
    }
    if (one === 'ー') {
      units.push('CHOON')
      continue
    }
    if (HIRAGANA_MONOGRAPHS[one] !== undefined) {
      units.push(HIRAGANA_MONOGRAPHS[one])
      continue
    }
    units.push(one)
  }
  return units
}

export function hiraganaToRomaji(text: string): string {
  const units = tokenizeHiragana(text)

  let result = ''
  for (let i = 0; i < units.length; i++) {
    if (units[i] === 'SOKUON') {
      const next = units[i + 1]
      if (next && /^[a-z]/.test(next)) {
        result += next[0]
      }
      continue
    }
    if (units[i] === 'CHOON') {
      const lastVowel = result.match(/[aiueo](?=[^aiueo]*$)/)
      if (lastVowel) result += lastVowel[0]
      continue
    }
    result += units[i]
  }
  return result
}

export function kanaToRomaji(text: string): string {
  return hiraganaToRomaji(katakanaToHiragana(text))
}
