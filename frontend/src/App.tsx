import { Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { AuthFormPage } from './pages/AuthFormPage'
import { HomePage } from './pages/HomePage'
import { LibraryPage } from './pages/LibraryPage'
import { RecommendPage } from './pages/RecommendPage'
import { SearchPage } from './pages/SearchPage'
import { SeasonPage } from './pages/SeasonPage'

function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthFormPage mode="login" />} />
      <Route path="/signup" element={<AuthFormPage mode="signup" />} />
      <Route
        path="/"
        element={
          <Layout>
            <HomePage />
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
            <RecommendPage />
          </Layout>
        }
      />
      <Route
        path="/season"
        element={
          <Layout>
            <SeasonPage />
          </Layout>
        }
      />
    </Routes>
  )
}

export default App
