import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'

const NAV_ITEMS = [
  {
    to: '/',
    label: 'ホーム',
    bottomLabel: 'ホーム',
    icon: (
      <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
        <path d="M40 120 128 48l88 72v88a8 8 0 0 1-8 8H48a8 8 0 0 1-8-8Z" strokeLinejoin="round" />
        <path d="M96 216v-64h64v64" />
      </svg>
    ),
  },
  {
    to: '/search',
    label: '検索・追加',
    bottomLabel: '検索',
    icon: (
      <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
        <circle cx="112" cy="112" r="76" />
        <line x1="168" y1="168" x2="220" y2="220" strokeLinecap="round" />
      </svg>
    ),
  },
  {
    to: '/season',
    label: 'シーズン',
    bottomLabel: 'シーズン',
    icon: (
      <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
        <rect x="40" y="48" width="176" height="152" rx="8" />
        <line x1="40" y1="92" x2="216" y2="92" />
        <line x1="84" y1="32" x2="84" y2="64" />
        <line x1="172" y1="32" x2="172" y2="64" />
      </svg>
    ),
  },
  {
    to: '/recommend',
    label: 'おすすめ',
    bottomLabel: 'おすすめ',
    icon: (
      <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
        <path d="M128 40l24 56 60 5-46 40 14 59-52-32-52 32 14-59-46-40 60-5Z" strokeLinejoin="round" />
      </svg>
    ),
  },
  {
    to: '/library',
    label: 'マイライブラリ',
    bottomLabel: 'ライブラリ',
    icon: (
      <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
        <rect x="44" y="44" width="168" height="168" rx="8" />
        <line x1="44" y1="92" x2="212" y2="92" />
        <line x1="100" y1="44" x2="100" y2="212" />
      </svg>
    ),
  },
]

function AuthSlot() {
  const { user, logout } = useAuth()
  if (user) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div className="text-muted" style={{ fontSize: 11, wordBreak: 'break-all' }}>
          {user.email}
        </div>
        <button className="btn btn-secondary" style={{ width: '100%', fontSize: 12 }} onClick={() => logout()}>
          ログアウト
        </button>
      </div>
    )
  }
  return (
    <NavLink to="/login" className="btn btn-primary" style={{ width: '100%', fontSize: 12 }}>
      ログイン
    </NavLink>
  )
}

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="app-shell">
      <nav className="sidebar">
        <div className="sidebar-brand">
          <div className="brand-badge">
            <svg width="21" height="21" viewBox="0 0 64 64" style={{ flex: 'none' }}>
              <path d="M32 18c-6-4-14-5-22-4v34c8-1 16 0 22 4V18Z" fill="var(--color-bg)" />
              <path d="M32 18c6-4 14-5 22-4v34c-8-1-16 0-22 4V18Z" fill="var(--color-bg)" opacity="0.5" />
              <path d="M27 27l13 7-13 7V27Z" fill="var(--color-accent)" />
            </svg>
            <span>みろく</span>
          </div>
        </div>
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}
          >
            {item.icon}
            {item.label}
          </NavLink>
        ))}
        <div className="sidebar-footer">
          アニメも漫画も、
          <br />
          ひとつの記録に。
        </div>
        <div style={{ padding: '0 10px', marginTop: 'var(--space-3)' }}>
          <AuthSlot />
        </div>
      </nav>

      <main className="main has-bottom-nav">
        <div className="screen">{children}</div>
      </main>

      <nav className="bottom-nav">
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) => `bottom-nav-link${isActive ? ' active' : ''}`}
          >
            {item.icon}
            {item.bottomLabel}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
