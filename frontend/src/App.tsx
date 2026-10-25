import { Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { AuthFormPage } from './pages/AuthFormPage'
import { HomePage } from './pages/HomePage'
import { LibraryPage } from './pages/LibraryPage'
import { NewArrivalsPage } from './pages/NewArrivalsPage'
import { RecommendPage } from './pages/RecommendPage'
import { SearchPage } from './pages/SearchPage'
import { SeasonPage } from './pages/SeasonPage'

// ログイン画面とサインアップ画面以外はすべてログイン必須。アプリを開いたら必ずログイン画面に
// 飛ばす（2026-10-25）。サーバーの /api/home/* などは未ログインでも叩けるままで、
// ここで塞いでいるのは画面だけ（BACKLOG 4-a の 4）。
function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthFormPage mode="login" />} />
      <Route path="/signup" element={<AuthFormPage mode="signup" />} />
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
    </Routes>
  )
}

export default App
