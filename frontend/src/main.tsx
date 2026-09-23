import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { WorkModalProvider } from './components/WorkModalContext.tsx'
import { AuthProvider } from './context/AuthContext.tsx'
import './index.css'
import './styles.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <WorkModalProvider>
          <App />
        </WorkModalProvider>
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
)
