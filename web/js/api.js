// api.js — バックエンドAPIへの薄いラッパー
const API_BASE = '/api';

async function request(path, options = {}) {
  const res = await fetch(API_BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `request failed: ${res.status}`);
  }
  if (res.status === 204) return null;
  return res.json();
}

function buildQuery(params) {
  const usp = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value) usp.set(key, value);
  }
  const qs = usp.toString();
  return qs ? `?${qs}` : '';
}

const api = {
  listRecords({ type = '', status = '' } = {}) {
    return request('/records' + buildQuery({ type, status }));
  },
  createRecord(body) {
    return request('/records', { method: 'POST', body: JSON.stringify(body) });
  },
  updateRecord(id, body) {
    return request(`/records/${id}`, { method: 'PATCH', body: JSON.stringify(body) });
  },
  searchAniList({ type, q = '' }) {
    return request('/search' + buildQuery({ type, q }));
  },
  mediaByIds({ type, ids }) {
    if (ids.length === 0) return Promise.resolve([]);
    return request('/anilist/media' + buildQuery({ type, ids: ids.join(',') }));
  },
  seasonAnime() {
    return request('/home/season-anime');
  },
  trending() {
    return request('/home/trending');
  },
  getMe() {
    return request('/auth/me');
  },
  signup({ email, password }) {
    return request('/auth/signup', { method: 'POST', body: JSON.stringify({ email, password }) });
  },
  login({ email, password }) {
    return request('/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) });
  },
  logout() {
    return request('/auth/logout', { method: 'POST' });
  },
};
