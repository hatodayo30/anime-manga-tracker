package auth

import (
	"context"
	"errors"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// ErrGoogleDisabled は Google ログインが設定されていない（GOOGLE_CLIENT_ID などが無い）ことを表す。
var ErrGoogleDisabled = errors.New("google login is not configured")

// ErrGoogleEmailUnverified は初回の Google ログインで、Google 側のメールアドレスが確認済みでないことを表す。
var ErrGoogleEmailUnverified = errors.New("google email is not verified")

// GoogleProfile は Google の userinfo から取る、ログインに要る項目だけ。
type GoogleProfile struct {
	// Subject は Google アカウントの安定した識別子（sub）。以後の照合はこれだけで行う
	Subject       string
	Email         string
	EmailVerified bool
}

// GoogleGateway は Google の認可コードフローのうち、Google と通信する部分。実装は internal/google が提供する。
type GoogleGateway interface {
	// AuthCodeURL は利用者を送る Google の同意画面の URL を返す。state はそのまま付けて返ってくる
	AuthCodeURL(state string) string
	// Profile は認可コードをアクセストークンに引き換え、userinfo を引いて返す。
	// id_token は検証せずに捨てる。TLS で Google から直接受け取ったアクセストークンで userinfo を叩けば、
	// 返ってきた値は Google が答えたものだと言えるため（BACKLOG 8-b）
	Profile(ctx context.Context, code string) (*GoogleProfile, error)
}

// GoogleEnabled は Google ログインが使えるかどうか。フロントはこれでボタンを出し分ける。
func (s *Service) GoogleEnabled() bool {
	return s.google != nil
}

// GoogleAuthURL は state を付けた Google の同意画面の URL を返す。
// state の生成と照合（短命 Cookie）は HTTP の関心事なのでハンドラが持つ。
func (s *Service) GoogleAuthURL(state string) (string, error) {
	if s.google == nil {
		return "", ErrGoogleDisabled
	}
	return s.google.AuthCodeURL(state), nil
}

// LoginWithGoogle は Google から戻ってきた認可コードでログインし、パスワードのログインと同じセッションを発行する。
// 登録とログインを区別しない（初めてなら作る）。
//
// ユーザーの決め方:
//  1. sub で引く。見つかればそのユーザー。メールアドレスは見ない（Google 側で変わりうるので、
//     照合に使うのは初回の紐付けだけ）
//  2. 見つからなければ初回。email_verified が false なら断る
//  3. 同じメールアドレスのユーザーがいれば、そこに Google を紐付ける（パスワードで登録済みの人が Google でも入れる）
//  4. いなければ、パスワードを持たないユーザーを作る
//
// 2 で断るのは、確認されていないメールアドレスは誰でも名乗れるから。紐付け（3）に使えば、他人のメールアドレスを
// 名乗る Google アカウントでその人のアカウントに入れてしまう。新規作成（4）でも断るのは、他人のメールアドレスで
// 先に作っておくと、本人がメールアドレスで登録しようとしたときに 409 で弾かれるうえ、本人が後から
// Google で入ったときに 3 で乗っ取った側のアカウントに紐付いてしまうため。
func (s *Service) LoginWithGoogle(ctx context.Context, code string) (*domain.User, string, time.Time, error) {
	if s.google == nil {
		return nil, "", time.Time{}, ErrGoogleDisabled
	}
	profile, err := s.google.Profile(ctx, code)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	user, err := s.findOrCreateGoogleUser(ctx, profile)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.createSession(ctx, user.ID)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return user, token, expiresAt, nil
}

func (s *Service) findOrCreateGoogleUser(ctx context.Context, profile *GoogleProfile) (*domain.User, error) {
	user, err := s.users.FindByIdentity(ctx, ProviderGoogle, profile.Subject)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	if !profile.EmailVerified {
		return nil, ErrGoogleEmailUnverified
	}
	email := normalizeEmail(profile.Email)

	user, err = s.users.LinkIdentityByEmail(ctx, email, ProviderGoogle, profile.Subject)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return s.users.CreateWithIdentity(ctx, email, ProviderGoogle, profile.Subject)
}
