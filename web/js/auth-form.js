// auth-form.js — login.html / signup.html 共通のフォーム送信処理
function safeNextPath() {
  const next = new URLSearchParams(location.search).get('next');
  // "/" 始まりかつ "//" では始まらないパスのみ許可（外部サイトへのオープンリダイレクト対策）
  if (next && next.startsWith('/') && !next.startsWith('//')) return next;
  return null;
}

(function () {
  const mode = document.currentScript.dataset.mode; // 'login' | 'signup'
  const form = document.getElementById(mode === 'login' ? 'login-form' : 'signup-form');
  const errorBox = document.getElementById('login-error');

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.style.display = 'none';

    const email = form.email.value.trim();
    const password = form.password.value;
    const submitBtn = form.querySelector('button[type="submit"]');
    submitBtn.disabled = true;

    try {
      if (mode === 'login') {
        await api.login({ email, password });
      } else {
        await api.signup({ email, password });
      }
      location.href = safeNextPath() || '/index.html';
    } catch (err) {
      errorBox.textContent = err.message;
      errorBox.style.display = 'block';
      submitBtn.disabled = false;
    }
  });
})();
