// 初めて来た人にだけ出す、アプリの紹介。画面はすべてログイン必須で、初めての人は必ずログイン画面に
// 着くので、出すのはログイン画面だけにした（サインアップ画面に来た人はもう登録すると決めている）。
//
// 「見た」の印は閉じたときではなく表示した時点で付ける。閉じずに再読み込み・タブを閉じた人にも、
// 2 回目以降は出さないため。サーバーには残さないので、端末やブラウザが変われば 1 回は出る。
// localStorage が使えない環境（プライベートブラウズなど）では毎回出るが、スキップできるので許容する。
const STORAGE_KEY = 'miroku.onboardingSeen'

export function hasSeenOnboarding(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

export function markOnboardingSeen(): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, '1')
  } catch {
    // 保存できなくても紹介は閉じられるので無視する
  }
}
