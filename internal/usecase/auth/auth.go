// Package auth は認証・セッションまわりのユースケースを担う。
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

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
}

func NewService(users UserRepository, sessions SessionRepository) *Service {
	return &Service{users: users, sessions: sessions}
}

// SignUp はユーザーを作成し、ログイン済みとして扱うセッションを発行する。
func (s *Service) SignUp(ctx context.Context, email, password string) (*domain.User, string, time.Time, error) {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, "", time.Time{}, fmt.Errorf("invalid email")
	}
	if len(password) < 8 {
		return nil, "", time.Time{}, fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > maxPasswordBytes {
		return nil, "", time.Time{}, fmt.Errorf("password must be at most %d bytes (a Japanese character counts as 3)", maxPasswordBytes)
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

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, token)
}

// CurrentUser はセッショントークンからログイン中ユーザーを引く。
func (s *Service) CurrentUser(ctx context.Context, token string) (*domain.User, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	return s.sessions.FindUser(ctx, token)
}

func (s *Service) createSession(ctx context.Context, userID int64) (string, time.Time, error) {
	token, err := generateToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	expiresAt := time.Now().Add(SessionTTL)
	if err := s.sessions.Create(ctx, token, userID, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
