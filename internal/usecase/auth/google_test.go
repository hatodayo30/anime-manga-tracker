package auth_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

type fakeGoogle struct {
	profile *auth.GoogleProfile
	err     error
	// Profile に渡された認可コードと PKCE の verifier
	gotCode     string
	gotVerifier string
}

func (f *fakeGoogle) AuthCodeURL(state, verifier string) string {
	return "https://accounts.example/auth?state=" + state
}

func (f *fakeGoogle) Profile(ctx context.Context, code, verifier string) (*auth.GoogleProfile, error) {
	f.gotCode = code
	f.gotVerifier = verifier
	return f.profile, f.err
}

// sessionRecorder は Create に渡された userID を記録する SessionRepository。
func sessionRecorder(createdFor *int64) *fakeSessionRepository {
	return &fakeSessionRepository{
		createFunc: func(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
			*createdFor = userID
			return nil
		},
	}
}

// 設定が無ければ Google ログインは無効で、Google にもリポジトリにも触らない。
// フロントは GoogleEnabled でボタンを出さないので、ここに来るのは URL を直接叩いたときだけ。
func TestService_Google_Disabled(t *testing.T) {
	users := &fakeUserRepository{}
	s := auth.NewService(users, &fakeSessionRepository{})

	if s.GoogleEnabled() {
		t.Error("GoogleEnabled = true without WithGoogle")
	}
	if _, err := s.GoogleAuthURL("state", "verifier"); !errors.Is(err, auth.ErrGoogleDisabled) {
		t.Errorf("GoogleAuthURL: expected ErrGoogleDisabled, got %v", err)
	}
	if _, _, _, err := s.LoginWithGoogle(context.Background(), "code", "the-verifier"); !errors.Is(err, auth.ErrGoogleDisabled) {
		t.Errorf("LoginWithGoogle: expected ErrGoogleDisabled, got %v", err)
	}
	if len(users.identityCalls) != 0 {
		t.Errorf("repository should not be called, got %v", users.identityCalls)
	}
}

// 2 回目以降は sub だけで引き、メールアドレスは見ない。Google 側でメールアドレスが変わっても、
// email_verified が false になっても同じアカウントに入れる（BACKLOG 8-b「以後の照合は sub」）。
func TestService_LoginWithGoogle_ExistingIdentityMatchedBySubOnly(t *testing.T) {
	google := &fakeGoogle{profile: &auth.GoogleProfile{Subject: "sub-1", Email: "changed@example.com", EmailVerified: false}}
	users := &fakeUserRepository{
		findByIdentityFunc: func(ctx context.Context, provider, subject string) (*domain.User, error) {
			if provider != auth.ProviderGoogle || subject != "sub-1" {
				t.Errorf("FindByIdentity(%q, %q), want (google, sub-1)", provider, subject)
			}
			return &domain.User{ID: 7, Email: "old@example.com"}, nil
		},
	}
	var sessionFor int64
	s := auth.NewService(users, sessionRecorder(&sessionFor), auth.WithGoogle(google))

	user, token, _, err := s.LoginWithGoogle(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 引き換えには start で作った PKCE の verifier をそのまま渡す。違えば Google が断る
	if google.gotCode != "the-code" || google.gotVerifier != "the-verifier" {
		t.Errorf("passed to Google: code %q, verifier %q; want the-code, the-verifier", google.gotCode, google.gotVerifier)
	}
	if user.ID != 7 || sessionFor != 7 || token == "" {
		t.Errorf("user = %d, session for %d, token %q; want user 7 with a session", user.ID, sessionFor, token)
	}
	if !slices.Equal(users.identityCalls, []string{"FindByIdentity"}) {
		t.Errorf("calls = %v, want only FindByIdentity", users.identityCalls)
	}
}

// 初回で email_verified が false なら、紐付けも新規作成もしない。紐付ければ他人のメールアドレスを
// 名乗る Google アカウントで乗っ取れ、新規作成すれば他人のメールアドレスを先取りできる。
// セッションも作らない。
func TestService_LoginWithGoogle_UnverifiedEmailRejectedOnFirstLogin(t *testing.T) {
	google := &fakeGoogle{profile: &auth.GoogleProfile{Subject: "sub-1", Email: "victim@example.com", EmailVerified: false}}
	users := &fakeUserRepository{}
	var sessionFor int64
	s := auth.NewService(users, sessionRecorder(&sessionFor), auth.WithGoogle(google))

	_, _, _, err := s.LoginWithGoogle(context.Background(), "code", "the-verifier")
	if !errors.Is(err, auth.ErrGoogleEmailUnverified) {
		t.Fatalf("expected ErrGoogleEmailUnverified, got %v", err)
	}
	if !slices.Equal(users.identityCalls, []string{"FindByIdentity"}) {
		t.Errorf("calls = %v, want only FindByIdentity", users.identityCalls)
	}
	if sessionFor != 0 {
		t.Error("session should not be created")
	}
}

// 初回で email_verified が true なら、同じメールアドレスのユーザーに Google を紐付ける。
// メールアドレスはパスワードの登録と同じく正規化（小文字・前後の空白除去）してから照合する。
func TestService_LoginWithGoogle_LinksToExistingUserByVerifiedEmail(t *testing.T) {
	google := &fakeGoogle{profile: &auth.GoogleProfile{Subject: "sub-1", Email: " User@Example.com", EmailVerified: true}}
	users := &fakeUserRepository{
		linkIdentityByEmailFunc: func(ctx context.Context, email, provider, subject string) (*domain.User, error) {
			if email != "user@example.com" || provider != auth.ProviderGoogle || subject != "sub-1" {
				t.Errorf("LinkIdentityByEmail(%q, %q, %q), want (user@example.com, google, sub-1)", email, provider, subject)
			}
			return &domain.User{ID: 3, Email: email}, nil
		},
	}
	var sessionFor int64
	s := auth.NewService(users, sessionRecorder(&sessionFor), auth.WithGoogle(google))

	user, _, _, err := s.LoginWithGoogle(context.Background(), "code", "the-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != 3 || sessionFor != 3 {
		t.Errorf("user = %d, session for %d; want 3", user.ID, sessionFor)
	}
	if !slices.Equal(users.identityCalls, []string{"FindByIdentity", "LinkIdentityByEmail"}) {
		t.Errorf("calls = %v, want FindByIdentity then LinkIdentityByEmail", users.identityCalls)
	}
}

// 同じメールアドレスのユーザーもいなければ、パスワードを持たないユーザーを作る（登録とログインが同じ経路）。
func TestService_LoginWithGoogle_CreatesUserWhenNoneMatches(t *testing.T) {
	google := &fakeGoogle{profile: &auth.GoogleProfile{Subject: "sub-1", Email: "new@example.com", EmailVerified: true}}
	users := &fakeUserRepository{
		createWithIdentityFunc: func(ctx context.Context, email, provider, subject string) (*domain.User, error) {
			if email != "new@example.com" || provider != auth.ProviderGoogle || subject != "sub-1" {
				t.Errorf("CreateWithIdentity(%q, %q, %q), want (new@example.com, google, sub-1)", email, provider, subject)
			}
			return &domain.User{ID: 9, Email: email}, nil
		},
	}
	var sessionFor int64
	s := auth.NewService(users, sessionRecorder(&sessionFor), auth.WithGoogle(google))

	user, _, _, err := s.LoginWithGoogle(context.Background(), "code", "the-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != 9 || sessionFor != 9 {
		t.Errorf("user = %d, session for %d; want 9", user.ID, sessionFor)
	}
	if !slices.Equal(users.identityCalls, []string{"FindByIdentity", "LinkIdentityByEmail", "CreateWithIdentity"}) {
		t.Errorf("calls = %v", users.identityCalls)
	}
}

// 同じメールアドレスのユーザーに別の Google アカウントが紐付いていれば、紐付けも新規作成もせずに返す。
// 黙って新規作成に進むと、メールアドレスの重複で失敗するか、別人として 2 つ目のアカウントができる。
func TestService_LoginWithGoogle_UserAlreadyHasAnotherGoogleAccount(t *testing.T) {
	google := &fakeGoogle{profile: &auth.GoogleProfile{Subject: "sub-2", Email: "user@example.com", EmailVerified: true}}
	users := &fakeUserRepository{
		linkIdentityByEmailFunc: func(ctx context.Context, email, provider, subject string) (*domain.User, error) {
			return nil, auth.ErrIdentityTaken
		},
	}
	var sessionFor int64
	s := auth.NewService(users, sessionRecorder(&sessionFor), auth.WithGoogle(google))

	_, _, _, err := s.LoginWithGoogle(context.Background(), "code", "the-verifier")
	if !errors.Is(err, auth.ErrIdentityTaken) {
		t.Fatalf("expected ErrIdentityTaken, got %v", err)
	}
	if slices.Contains(users.identityCalls, "CreateWithIdentity") {
		t.Error("CreateWithIdentity should not be called")
	}
	if sessionFor != 0 {
		t.Error("session should not be created")
	}
}

// Google との通信が失敗したら、リポジトリに触らずにそのエラーを返す。
func TestService_LoginWithGoogle_GatewayError(t *testing.T) {
	gatewayErr := errors.New("token exchange failed")
	users := &fakeUserRepository{}
	s := auth.NewService(users, &fakeSessionRepository{}, auth.WithGoogle(&fakeGoogle{err: gatewayErr}))

	_, _, _, err := s.LoginWithGoogle(context.Background(), "code", "the-verifier")
	if !errors.Is(err, gatewayErr) {
		t.Fatalf("expected gateway error, got %v", err)
	}
	if len(users.identityCalls) != 0 {
		t.Errorf("repository should not be called, got %v", users.identityCalls)
	}
}

// Google だけで登録したユーザーのパスワード変更は ErrNoPassword をそのまま返す（ハンドラは 400 にする）。
// ErrNotFound に混ぜると 401 になり、フロントがログイン画面へ送ってしまう。
func TestService_ChangePassword_NoPassword(t *testing.T) {
	users := &fakeUserRepository{
		passwordHashFunc: func(ctx context.Context, userID int64) (string, error) {
			return "", auth.ErrNoPassword
		},
	}
	s := auth.NewService(users, &fakeSessionRepository{})

	err := s.ChangePassword(context.Background(), 1, "a-token", "anything-1", "new-password")
	if !errors.Is(err, auth.ErrNoPassword) {
		t.Fatalf("expected ErrNoPassword, got %v", err)
	}
	if users.updateCalled {
		t.Error("UpdatePassword should not be called")
	}
}
