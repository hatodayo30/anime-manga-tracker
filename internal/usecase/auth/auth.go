// Package auth は認証・セッションまわりのユースケースを担う。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

// ErrWrongCurrentPassword はパスワード変更で、確認のために入力された現在のパスワードが違うことを表す。
// ErrInvalidCredentials と分けるのは、ログイン済みの利用者に 401 を返すとフロントが
// セッション切れと区別できず、ログイン画面へ送ってしまうため（ハンドラは 403 にする）。
var ErrWrongCurrentPassword = errors.New("current password is incorrect")

// ErrNoPassword はパスワード変更で、利用者がパスワードを持っていない（Google だけで登録した）ことを表す。
// ハンドラは 400 にする。ErrNotFound のままだと 401 になり、フロントがログイン画面へ送ってしまう。
var ErrNoPassword = errors.New("this account has no password (signed up with Google)")

// InputError は利用者の入力が条件を満たさないことを表す。ハンドラはこれを 400 にし、
// それ以外（DB の失敗など）は 500 にする。メッセージはそのまま画面に出る。
type InputError struct{ msg string }

func (e *InputError) Error() string { return e.msg }

const SessionTTL = 30 * 24 * time.Hour

// maxPasswordBytes は bcrypt が受け付けるパスワードの上限（バイト数）。これを超えると
// GenerateFromPassword が ErrPasswordTooLong を返し、利用者には "hash password: bcrypt: ..." という
// 内部の言葉のまま 400 が返っていた（BACKLOG 4-d）。入口で検証して何が悪いかを返す。
//
// 73 バイト目以降を黙って切り捨てる（古い bcrypt 実装の挙動）ことはしない。末尾だけ違う
// パスワードが同じものとして通ってしまい、利用者の認識とずれるため。
// 文字数ではなくバイト数で測るのは bcrypt の制約がバイトだからで、日本語なら 1 文字 3 バイトになる。
const maxPasswordBytes = 72

// dummyHash は Login でメールアドレスが見つからなかったときに、比較だけ回すためのハッシュ。
// 見つからなければ即座に返す実装だと、bcrypt（数十 ms）の有無で応答時間に差が出て、
// 登録済みのメールアドレスを外から判別できる（BACKLOG 4-c）。
//
// 定数の文字列で持たずに起動後に作るのは、cost を本物のハッシュ（bcrypt.DefaultCost）と
// 必ず揃えるため。cost が違えば所要時間も違い、対策にならない。
// 初回の作成だけ 1 回ぶん遅くなるが、プロセスの寿命で 1 度きりなので判別の材料にはならない。
// 元のパスワードは何でもよい。比較の結果は捨てて常に ErrInvalidCredentials を返す。
var dummyHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)
	if err != nil {
		// 72 バイト以下の固定値なので、失敗するのは乱数源が壊れたときだけ。
		// そのときはセッショントークンも作れないので、起動し直すしかない
		panic(fmt.Sprintf("auth: generate dummy hash: %v", err))
	}
	return hash
})

type Service struct {
	users    UserRepository
	sessions SessionRepository
	// google は Google ログイン（BACKLOG 8-b）。nil なら無効で、GoogleEnabled が false を返す
	google GoogleGateway
}

// Option は NewService の任意の設定。
type Option func(*Service)

// WithGoogle は Google ログインを有効にする。GOOGLE_CLIENT_ID などが揃っている環境でだけ渡す。
func WithGoogle(g GoogleGateway) Option {
	return func(s *Service) { s.google = g }
}

func NewService(users UserRepository, sessions SessionRepository, opts ...Option) *Service {
	s := &Service{users: users, sessions: sessions}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SignUp はユーザーを作成し、ログイン済みとして扱うセッションを発行する。
func (s *Service) SignUp(ctx context.Context, email, password string) (*domain.User, string, time.Time, error) {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, "", time.Time{}, fmt.Errorf("invalid email")
	}
	if err := validatePassword(password); err != nil {
		return nil, "", time.Time{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, email, string(hash))
	if err != nil {
		return nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.createSession(ctx, user.ID)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return user, token, expiresAt, nil
}

// Login はメール/パスワードを検証し、成功すればセッションを発行する。
//
// メールアドレスが無いときも、パスワードが違うときも、同じエラーを同じくらいの時間で返す
// （dummyHash を参照）。72 バイトを超えるパスワードは SignUp と違って入口で弾かない。
// SignUp が受け付けないのでそういうアカウントは存在せず、比較すればどのみち
// ErrInvalidCredentials になる（CompareHashAndPassword は長さでエラーにしない）。
func (s *Service) Login(ctx context.Context, email, password string) (*domain.User, string, time.Time, error) {
	email = normalizeEmail(email)

	userWithHash, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
			return nil, "", time.Time{}, ErrInvalidCredentials
		}
		return nil, "", time.Time{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(userWithHash.PasswordHash), []byte(password)); err != nil {
		return nil, "", time.Time{}, ErrInvalidCredentials
	}

	token, expiresAt, err := s.createSession(ctx, userWithHash.ID)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return &userWithHash.User, token, expiresAt, nil
}

// ChangePassword は現在のパスワードを確かめてから新しいパスワードに差し替え、
// keepToken（いま使っているセッション）以外のセッションをすべて消す。
//
// 他の端末のセッションを消すのは、パスワードを変える一番の理由が「漏れたかもしれない」だから。
// 残すと、盗んだパスワードでログインした相手がそのまま使い続けられる。ボタンとしての
// 「全端末からログアウト」はやらないと決めた（BACKLOG 4-d）が、変更に伴って消すのはその判断と別。
// いまの端末まで消すと、変更した本人がすぐにログインし直すことになるので残す。
//
// 現在のパスワードを求めるのは、開いたままの端末やセッションを盗んだ相手が、本人を締め出せないようにするため。
func (s *Service) ChangePassword(ctx context.Context, userID int64, keepToken, currentPassword, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	if newPassword == currentPassword {
		return &InputError{"new password must be different from the current one"}
	}

	hash, err := s.users.PasswordHash(ctx, userID)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(currentPassword)); err != nil {
		return ErrWrongCurrentPassword
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.users.UpdatePassword(ctx, userID, string(newHash), hashToken(keepToken))
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, hashToken(token))
}

// CurrentUser はセッショントークンからログイン中ユーザーを引く。
func (s *Service) CurrentUser(ctx context.Context, token string) (*domain.User, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	return s.sessions.FindUser(ctx, hashToken(token))
}

// createSession はセッションを作り、Cookie に入れる平文のトークンを返す。DB に残すのはハッシュだけ。
func (s *Service) createSession(ctx context.Context, userID int64) (string, time.Time, error) {
	token, err := generateToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	expiresAt := time.Now().Add(SessionTTL)
	if err := s.sessions.Create(ctx, hashToken(token), userID, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// hashToken はセッショントークンを DB に保存・照合するときの形（SHA-256 の 16 進）にする（BACKLOG 4-d）。
// 平文のまま持つと、DB の中身が漏れた時点でそのまま Cookie を作ってなりすませる。
//
// bcrypt ではなく SHA-256 にするのは、トークンが crypto/rand の 32 バイトで総当たりが成り立たず、
// 遅いハッシュで守る必要がないため。リクエストのたびに引くので速さも要り、ソルトを付けると
// トークンから行を引けなくなる。
//
// リポジトリではなく usecase でハッシュするのは、「DB に平文を渡さない」ことを
// fake のテストで保証するため。0006 のマイグレーションも同じ計算で既存の行を置き換えている。
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// validatePassword は SignUp と ChangePassword で共通のパスワードの条件を確かめる。
func validatePassword(password string) error {
	if len(password) < 8 {
		return &InputError{"password must be at least 8 characters"}
	}
	if len(password) > maxPasswordBytes {
		return &InputError{fmt.Sprintf("password must be at most %d bytes (a Japanese character counts as 3)", maxPasswordBytes)}
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
