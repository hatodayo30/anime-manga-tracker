import { Link } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'

export function HomePage() {
  const { user, loading, logout } = useAuth()

  return (
    <main style={{ padding: 'var(--space-6)' }}>
      <h1>anime-manga-tracker</h1>
      {loading ? (
        <p className="text-muted">読み込み中...</p>
      ) : user ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, alignItems: 'flex-start' }}>
          <p>ログイン中: {user.email}</p>
          <button className="btn btn-secondary" onClick={() => logout()}>
            ログアウト
          </button>
        </div>
      ) : (
        <p>
          <Link className="btn btn-primary" to="/login">
            ログイン
          </Link>
        </p>
      )}
    </main>
  )
}
