import { Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { AccountPage } from './pages/AccountPage'
import { AuthFormPage } from './pages/AuthFormPage'
import { HomePage } from './pages/HomePage'
import { LibraryPage } from './pages/LibraryPage'
import { NewArrivalsPage } from './pages/NewArrivalsPage'
import { PrivacyPage } from './pages/PrivacyPage'
import { RecommendPage } from './pages/RecommendPage'
import { SearchPage } from './pages/SearchPage'
import { SeasonPage } from './pages/SeasonPage'

// ログイン画面・サインアップ画面・プライバシーポリシー以外はすべてログイン必須。アプリを開いたら必ずログイン画面に
// 飛ばす（2026-10-25）。サーバーの /api/home/* などは未ログインでも叩けるままで、
// ここで塞いでいるのは画面だけ（BACKLOG 4-a の 4）。
// プライバシーポリシーは登録前に読めないと意味が無く、Google の同意画面からも未ログインで開かれるので公開にする。
function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthFormPage mode="login" />} />
      <Route path="/signup" element={<AuthFormPage mode="signup" />} />
      <Route path="/privacy" element={<PrivacyPage />} />
      <Route
        path="/"
        element={
          <Layout>
            <ProtectedRoute>
              <HomePage />
            </ProtectedRoute>
          </Layout>
        }
      />
      <Route
        path="/search"
        element={
          <Layout>
            <ProtectedRoute>
              <SearchPage />
            </ProtectedRoute>
          </Layout>
        }
      />
      <Route
        path="/library"
        element={
          <Layout>
            <ProtectedRoute>
              <LibraryPage />
            </ProtectedRoute>
          </Layout>
        }
      />
      <Route
        path="/recommend"
        element={
          <Layout>
            <ProtectedRoute>
              <RecommendPage />
            </ProtectedRoute>
          </Layout>
        }
      />
      <Route
        path="/new"
        element={
          <Layout>
            <ProtectedRoute>
              <NewArrivalsPage />
            </ProtectedRoute>
          </Layout>
        }
      />
      <Route
        path="/season"
        element={
          <Layout>
            <ProtectedRoute>
              <SeasonPage />
            </ProtectedRoute>
          </Layout>
        }
      />
      <Route
        path="/account"
        element={
          <Layout>
            <ProtectedRoute>
              <AccountPage />
            </ProtectedRoute>
          </Layout>
        }
      />
    </Routes>
  )
}

export default App
