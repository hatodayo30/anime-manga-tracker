export type ItemType = 'anime' | 'manga';
export type Status = 'done' | 'active' | 'want';

export interface LibraryItem {
  id: number;
  type: ItemType;
  title: string;
  genres: string[];
  status: Status;
  color: string;
  initial: string;
  progress: number;
  total: number | null;
  nextAir: number | null;
}

export type Screen = 'home' | 'search' | 'library';
