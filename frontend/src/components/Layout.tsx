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
    to: '/new',
    label: '新着',
    bottomLabel: '新着',
    icon: (
      <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
        <path d="M128 32v32M128 192v32M32 128h32M192 128h32M60 60l23 23M173 173l23 23M60 196l23-23M173 83l23-23" strokeLinecap="round" />
        <circle cx="128" cy="128" r="36" />
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

const ACCOUNT_ICON = (
  <svg width="17" height="17" viewBox="0 0 256 256" fill="none" stroke="currentColor" strokeWidth="16">
    <circle cx="128" cy="96" r="44" />
    <path d="M40 216c12-40 48-64 88-64s76 24 88 64" strokeLinecap="round" />
  </svg>
)

function AuthSlot() {
  const { user } = useAuth()
  if (user) {
    return (
      <NavLink to="/account" className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
        {ACCOUNT_ICON}
        {/* メールアドレスだけだと押せる場所に見えないので、行き先を名前で書き、アドレスは下に小さく添える */}
        <span style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          アカウント設定
          <span className="text-muted" style={{ fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {user.email}
          </span>
        </span>
      </NavLink>
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
        <div style={{ marginTop: 'var(--space-3)' }}>
          <AuthSlot />
        </div>
      </nav>

      <main className="main has-bottom-nav">
        {/* スマホ幅ではサイドバーが隠れるので、アカウント画面（ログアウトもここ）への入口を上に置く。
            下のナビは 6 項目で埋まっているので足さない */}
        <div className="mobile-topbar">
          <NavLink to="/account" className="btn btn-icon btn-secondary" aria-label="アカウント設定">
            {ACCOUNT_ICON}
          </NavLink>
        </div>
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
