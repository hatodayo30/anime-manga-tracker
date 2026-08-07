import type { LibraryItem } from '../types';

export function makeSample(): LibraryItem[] {
  const now = Date.now();
  const h = (n: number) => now + n * 3600000;
  return [
    { id: 1, type: 'anime', title: '蒼穹のフロンティア', genres: ['SF', 'アクション'], status: 'active', color: '#b5abfc', initial: '蒼', progress: 8, total: 24, nextAir: h(50) },
    { id: 2, type: 'anime', title: '月夜のレシピ', genres: ['日常'], status: 'active', color: '#9690c9', initial: '月', progress: 5, total: 12, nextAir: h(16) },
    { id: 3, type: 'anime', title: '鋼の記憶', genres: ['アクション', 'ドラマ'], status: 'done', color: '#7972a9', initial: '鋼', progress: 24, total: 24, nextAir: null },
    { id: 4, type: 'anime', title: 'サイレント・オーケストラ', genres: ['音楽'], status: 'want', color: '#5c5783', initial: 'サ', progress: 0, total: 13, nextAir: null },
    { id: 5, type: 'manga', title: '深海のパラドックス', genres: ['SF', 'ミステリー'], status: 'active', color: '#d2cefd', initial: '深', progress: 12, total: null, nextAir: null },
    { id: 6, type: 'manga', title: '花時計の少女', genres: ['恋愛'], status: 'active', color: '#b5afe8', initial: '花', progress: 4, total: 10, nextAir: null },
    { id: 7, type: 'manga', title: '鉄槌の勇者', genres: ['ファンタジー', 'アクション'], status: 'done', color: '#9690c9', initial: '鉄', progress: 18, total: 18, nextAir: null },
    { id: 8, type: 'manga', title: '夜想曲の旅人', genres: ['ミステリー'], status: 'want', color: '#423e5d', initial: '夜', progress: 0, total: null, nextAir: null },
  ];
}

export const STATUS_LABELS = {
  anime: { done: '見た', active: '見てる', want: '見たい' },
  manga: { done: '読んだ', active: '読んでる', want: '読みたい' },
} as const;
