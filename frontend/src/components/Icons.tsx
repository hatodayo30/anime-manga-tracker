type IconProps = { size?: number };

export function LogoIcon({ size = 22 }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth={14}>
      <circle cx="128" cy="128" r="96" />
      <path d="M88 100 L168 100 L108 156 L168 156" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function HomeIcon({ size = 18 }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth={16}>
      <path d="M40 120 128 48l88 72v88a8 8 0 0 1-8 8H48a8 8 0 0 1-8-8Z" strokeLinejoin="round" />
      <path d="M96 216v-64h64v64" />
    </svg>
  );
}

export function SearchIcon({ size = 18 }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth={16}>
      <circle cx="112" cy="112" r="76" />
      <line x1="168" y1="168" x2="220" y2="220" strokeLinecap="round" />
    </svg>
  );
}

export function LibraryIcon({ size = 18 }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth={16}>
      <rect x="44" y="44" width="168" height="168" rx="8" />
      <line x1="44" y1="92" x2="212" y2="92" />
      <line x1="100" y1="44" x2="100" y2="212" />
    </svg>
  );
}

export function ClockIcon({ size = 12 }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth={18}>
      <circle cx="128" cy="128" r="96" />
      <polyline points="128,72 128,128 172,152" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
