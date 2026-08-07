import { useEffect, useState } from 'react';
import type { LibraryItem, Screen, Status } from './types';
import { makeSample } from './data/sampleData';
import { Sidebar, BottomNav } from './components/Nav';
import { Home } from './screens/Home';
import { Search } from './screens/Search';
import { Library } from './screens/Library';

function App() {
  const [screen, setScreen] = useState<Screen>('home');
  const [isMobile, setIsMobile] = useState(false);
  const [library, setLibrary] = useState<LibraryItem[]>(() => makeSample());

  useEffect(() => {
    const update = () => setIsMobile(window.innerWidth < 800);
    update();
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, []);

  const setStatus = (id: number, status: Status) => {
    setLibrary((prev) => prev.map((it) => (it.id === id ? { ...it, status } : it)));
  };

  const setProgress = (id: number, val: string) => {
    const n = Math.max(0, parseInt(val, 10) || 0);
    setLibrary((prev) => prev.map((it) => (it.id === id ? { ...it, progress: n } : it)));
  };

  const isSidebar = !isMobile;
  const navProps = {
    screen,
    goHome: () => setScreen('home'),
    goSearch: () => setScreen('search'),
    goLibrary: () => setScreen('library'),
  };

  return (
    <div style={{ minHeight: '100vh', background: 'var(--color-bg)', color: 'var(--color-text)', display: 'flex' }}>
      {isSidebar && <Sidebar {...navProps} />}

      <main style={{ flex: 1, minWidth: 0, padding: isSidebar ? '40px 48px 40px 40px' : '24px 20px 88px' }}>
        {screen === 'home' && <Home library={library} />}
        {screen === 'search' && <Search library={library} setStatus={setStatus} />}
        {screen === 'library' && <Library library={library} setProgress={setProgress} />}
      </main>

      {!isSidebar && <BottomNav {...navProps} />}
    </div>
  );
}

export default App;
