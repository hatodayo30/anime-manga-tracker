import { Route, Routes } from 'react-router-dom'
import { AuthFormPage } from './pages/AuthFormPage'
import { HomePage } from './pages/HomePage'

function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/login" element={<AuthFormPage mode="login" />} />
      <Route path="/signup" element={<AuthFormPage mode="signup" />} />
    </Routes>
  )
}

export default App
