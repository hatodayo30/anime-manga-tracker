import { useEffect, useState } from 'react'
import { api, type Me } from '../lib/api'

export function HomePage() {
  const [status, setStatus] = useState('checking API connection...')

  useEffect(() => {
    api
      .getMe()
      .then((me: Me) => setStatus(`API connected as ${me.email}`))
      .catch(() => setStatus('API connected (not logged in)'))
  }, [])

  return (
    <main>
      <h1>anime-manga-tracker</h1>
      <p>{status}</p>
    </main>
  )
}
