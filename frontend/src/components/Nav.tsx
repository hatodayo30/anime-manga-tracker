import type { Screen } from '../types';
import { HomeIcon, LibraryIcon, LogoIcon, SearchIcon } from './Icons';

interface NavProps {
  screen: Screen;
  goHome: () => void;
  goSearch: () => void;
  goLibrary: () => void;
}

const navColor = (screen: Screen, key: Screen) => (screen === key ? 'var(--color-accent)' : 'var(--color-text)');
const navBg = (screen: Screen, key: Screen) =>
  screen === key ? 'color-mix(in srgb, var(--color-accent) 12%, transparent)' : 'transparent';

export function Sidebar({ screen, goHome, goSearch, goLibrary }: NavProps) {
  return (
    <nav
      style={{
        width: 220,
        flex: 'none',
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
        padding: 'var(--space-6) var(--space-4)',
        borderRight: '1px solid var(--color-divider)',
        position: 'sticky',
        top: 0,
        alignSelf: 'flex-start',
        height: '100vh',
        background: 'var(--color-surface)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '0 var(--space-2)', marginBottom: 'var(--space-6)' }}>
        <span style={{ color: 'var(--color-accent)', flex: 'none' }}>
          <LogoIcon />
        </span>
        <h4 style={{ margin: 0 }}>RecTrack</h4>
      </div>
      <NavLink label="ホーム" onClick={goHome} color={navColor(screen, 'home')} bg={navBg(screen, 'home')} icon={<HomeIcon />} />
      <NavLink label="検索・追加" onClick={goSearch} color={navColor(screen, 'search')} bg={navBg(screen, 'search')} icon={<SearchIcon />} />
      <NavLink label="マイライブラリ" onClick={goLibrary} color={navColor(screen, 'library')} bg={navBg(screen, 'library')} icon={<LibraryIcon />} />
      <div
        style={{
          marginTop: 'auto',
          fontSize: 11,
          lineHeight: 1.6,
          color: 'color-mix(in srgb, var(--color-text) 45%, transparent)',
          padding: '0 var(--space-2)',
        }}
      >
        アニメ・漫画の視聴/読了記録を一元管理
      </div>
    </nav>
  );
}

function NavLink({
  label,
  onClick,
  color,
  bg,
  icon,
}: {
  label: string;
  onClick: () => void;
  color: string;
  bg: string;
  icon: React.ReactNode;
}) {
  return (
    <a
      href="javascript:void(0)"
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '10px var(--space-3)',
        borderRadius: 'var(--radius-md)',
        textDecoration: 'none',
        color,
        background: bg,
        fontSize: 14,
      }}
    >
      {icon}
      {label}
    </a>
  );
}

export function BottomNav({ screen, goHome, goSearch, goLibrary }: NavProps) {
  return (
    <nav
      style={{
        position: 'fixed',
        left: 0,
        right: 0,
        bottom: 0,
        display: 'flex',
        justifyContent: 'space-around',
        padding: '8px 4px calc(8px + env(safe-area-inset-bottom))',
        background: 'var(--color-surface)',
        borderTop: '1px solid var(--color-divider)',
      }}
    >
      <BottomLink label="ホーム" onClick={goHome} color={navColor(screen, 'home')} icon={<HomeIcon size={20} />} />
      <BottomLink label="検索" onClick={goSearch} color={navColor(screen, 'search')} icon={<SearchIcon size={20} />} />
      <BottomLink label="ライブラリ" onClick={goLibrary} color={navColor(screen, 'library')} icon={<LibraryIcon size={20} />} />
    </nav>
  );
}

function BottomLink({
  label,
  onClick,
  color,
  icon,
}: {
  label: string;
  onClick: () => void;
  color: string;
  icon: React.ReactNode;
}) {
  return (
    <a
      href="javascript:void(0)"
      onClick={onClick}
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: 2,
        textDecoration: 'none',
        color,
        fontSize: 10,
        flex: 1,
      }}
    >
      {icon}
      {label}
    </a>
  );
}
