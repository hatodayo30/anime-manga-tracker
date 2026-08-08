// auth.js — 全ページ共通のログイン状態表示・ナビ制御
async function getCurrentUser() {
  try {
    return await api.getMe();
  } catch {
    return null;
  }
}

// サイドバー/ボトムナビの末尾にログイン状態を表示し、
// data-requires-auth 属性を持つページでは未ログイン時にログイン画面へ誘導する。
async function initAuthUI() {
  const user = await getCurrentUser();

  const authSlots = document.querySelectorAll('[data-auth-slot]');
  for (const slot of authSlots) {
    slot.replaceChildren(renderAuthSlot(user));
  }

  const requiresAuth = document.body.dataset.requiresAuth === 'true';
  if (requiresAuth && !user) {
    const next = encodeURIComponent(location.pathname);
    location.replace(`/login.html?next=${next}`);
  }

  return user;
}

function renderAuthSlot(user) {
  if (user) {
    const logoutBtn = el(
      'button',
      {
        className: 'btn btn-secondary',
        style: { width: '100%', fontSize: '12px' },
        onClick: async () => {
          await api.logout();
          location.href = '/login.html';
        },
      },
      'ログアウト'
    );
    return el('div', { style: { display: 'flex', flexDirection: 'column', gap: '6px' } }, [
      el('div', { className: 'text-muted', style: { fontSize: '11px', wordBreak: 'break-all' } }, user.email),
      logoutBtn,
    ]);
  }
  return el('a', { href: '/login.html', className: 'btn btn-primary', style: { width: '100%', fontSize: '12px' } }, 'ログイン');
}

// このスクリプトは body の末尾で読み込まれるため、DOM は既に構築済み。
// 他のページスクリプトはこの Promise を待ってから個人データの取得を始める
// （未ログイン時のリダイレクト前に不要なAPIリクエストが飛ぶのを防ぐため）。
window.authReadyPromise = initAuthUI();
