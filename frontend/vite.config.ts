import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

// Go サーバーと共有している .env はリポジトリ直下にある。Vite の既定（frontend/）では読まれず、
// プロキシ先が 8080 に落ちて README どおりの `npm run dev` で /api が繋がらなかった。
const repoRoot = fileURLToPath(new URL('..', import.meta.url))

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // 第 3 引数の '' で VITE_ 接頭辞の無い変数も読む。読んだ値は vite.config の中だけで使い、
  // クライアントのバンドルには入らない（define していない）ので、.env の秘密が漏れることはない。
  // シェルの環境変数は .env より優先されるので、BACKEND_PORT=... npm run dev での上書きはそのまま効く。
  const env = loadEnv(mode, repoRoot, '')
  const backendPort = env.BACKEND_PORT || env.PORT || '8080'

  return {
    plugins: [react()],
    server: {
      port: 5174,
      // ポートが埋まっていたら黙って 5175 以降に逃げずに起動を失敗させる。
      // .env の GOOGLE_REDIRECT_URL と Google Cloud Console の登録が 5174 固定なので、逃げた先では
      // Google から戻った先が別のプロセスになり、エラーも出ずにログインだけが完了しなくなる。
      strictPort: true,
      proxy: {
        '/api': {
          target: `http://localhost:${backendPort}`,
          changeOrigin: true,
        },
      },
    },
  }
})
