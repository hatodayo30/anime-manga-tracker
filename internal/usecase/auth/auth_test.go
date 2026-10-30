package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

type fakeUserRepository struct {
	createFunc       func(ctx context.Context, email, passwordHash string) (*domain.User, error)
	findByEmailFunc  func(ctx context.Context, email string) (*auth.UserWithHash, error)
	passwordHashFunc func(ctx context.Context, userID int64) (string, error)
	called           bool

	// Google ログイン（google_test.go）で使う。nil のまま呼ばれたら ErrNotFound を返す
	findByIdentityFunc      func(ctx context.Context, provider, subject string) (*domain.User, error)
	linkIdentityByEmailFunc func(ctx context.Context, email, provider, subject string) (*domain.User, error)
	createWithIdentityFunc  func(ctx context.Context, email, provider, subject string) (*domain.User, error)
	// 呼ばれた順のメソッド名
	identityCalls []string

	// UpdatePassword に渡された引数。呼ばれなければ updateCalled は false のまま
	updateCalled         bool
	updatedUserID        int64
	updatedHash          string
	updatedKeepTokenHash string
}

func (f *fakeUserRepository) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	f.called = true
	return f.createFunc(ctx, email, passwordHash)
}

func (f *fakeUserRepository) FindByEmail(ctx context.Context, email string) (*auth.UserWithHash, error) {
	f.called = true
	return f.findByEmailFunc(ctx, email)
}

func (f *fakeUserRepository) PasswordHash(ctx context.Context, userID int64) (string, error) {
	f.called = true
	return f.passwordHashFunc(ctx, userID)
}

func (f *fakeUserRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash, keepTokenHash string) error {
	f.called = true
	f.updateCalled = true
	f.updatedUserID = userID
	f.updatedHash = passwordHash
	f.updatedKeepTokenHash = keepTokenHash
	return nil
}

func (f *fakeUserRepository) FindByIdentity(ctx context.Context, provider, subject string) (*domain.User, error) {
	f.identityCalls = append(f.identityCalls, "FindByIdentity")
	if f.findByIdentityFunc == nil {
		return nil, auth.ErrNotFound
	}
	return f.findByIdentityFunc(ctx, provider, subject)
}

func (f *fakeUserRepository) LinkIdentityByEmail(ctx context.Context, email, provider, subject string) (*domain.User, error) {
	f.identityCalls = append(f.identityCalls, "LinkIdentityByEmail")
	if f.linkIdentityByEmailFunc == nil {
		return nil, auth.ErrNotFound
	}
	return f.linkIdentityByEmailFunc(ctx, email, provider, subject)
}

func (f *fakeUserRepository) CreateWithIdentity(ctx context.Context, email, provider, subject string) (*domain.User, error) {
	f.identityCalls = append(f.identityCalls, "CreateWithIdentity")
	if f.createWithIdentityFunc == nil {
		return nil, auth.ErrNotFound
	}
	return f.createWithIdentityFunc(ctx, email, provider, subject)
}

type fakeSessionRepository struct {
	createFunc   func(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error
	findUserFunc func(ctx context.Context, tokenHash string) (*domain.User, error)
	deleteFunc   func(ctx context.Context, tokenHash string) error
	deleteCalled bool
	// Delete に渡された引数
	deletedTokenHash string
}

func (f *fakeSessionRepository) Create(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
	if f.createFunc == nil {
		return nil
	}
	return f.createFunc(ctx, tokenHash, userID, expiresAt)
}

func (f *fakeSessionRepository) FindUser(ctx context.Context, tokenHash string) (*domain.User, error) {
	return f.findUserFunc(ctx, tokenHash)
}

func (f *fakeSessionRepository) Delete(ctx context.Context, tokenHash string) error {
	f.deleteCalled = true
	f.deletedTokenHash = tokenHash
	if f.deleteFunc == nil {
		return nil
	}
	return f.deleteFunc(ctx, tokenHash)
}

// aTokenHash は "a-token" の SHA-256 を 16 進にしたもの（`printf a-token | shasum -a 256` の値）。
// 実装の hashToken を呼び直して比べるのではなく固定値で持つのは、計算方法が変わったときに
// テストも一緒に変わってしまわないようにするため。DB に残っている行（0006 のマイグレーションで
// 同じ計算で置き換えたもの）と食い違えば、既存の利用者が全員ログアウトされる。
const aTokenHash = "1f6076e3a47ba1ded08025ffe06e57af217c14f9407f33fba50f99b1c7019387"

// sha256Hex は SignUp のように乱数のトークンが返る場面で、保存されたハッシュと照らすために使う。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestService_SignUp_Validation(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
	}{
		{"invalid email", "not-an-email", "password123"},
		{"empty email", "", "password123"},
		{"short password", "user@example.com", "short"},
		// bcrypt の上限（72 バイト）を超えるパスワードは、bcrypt の内部エラーではなく
		// 検証エラーとして返り、リポジトリにも届かない（BACKLOG 4-d）
		{"password over 72 bytes", "user@example.com", strings.Repeat("a", 73)},
		// 上限はバイト数で測る。25 文字でも日本語なら 75 バイトになる
		{"multibyte password over 72 bytes", "user@example.com", strings.Repeat("あ", 25)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserRepository{}
			sessions := &fakeSessionRepository{}
			s := auth.NewService(users, sessions)

			_, _, _, err := s.SignUp(context.Background(), tt.email, tt.password)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if users.called {
				t.Error("repository should not be called when validation fails")
			}
		})
	}
}

// ちょうど 72 バイトのパスワードは受け付ける（上限の境界）。
func TestService_SignUp_AcceptsPasswordOf72Bytes(t *testing.T) {
	users := &fakeUserRepository{
		createFunc: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			return &domain.User{ID: 1, Email: email}, nil
		},
	}
	s := auth.NewService(users, &fakeSessionRepository{})

	if _, _, _, err := s.SignUp(context.Background(), "user@example.com", strings.Repeat("a", 72)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_SignUp_EmailTaken(t *testing.T) {
	users := &fakeUserRepository{
		createFunc: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			return nil, auth.ErrEmailTaken
		},
	}
	sessions := &fakeSessionRepository{}
	s := auth.NewService(users, sessions)

	_, _, _, err := s.SignUp(context.Background(), "user@example.com", "password123")
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestService_SignUp_Success(t *testing.T) {
	want := &domain.User{ID: 1, Email: "user@example.com"}
	users := &fakeUserRepository{
		createFunc: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			if email != "user@example.com" {
				t.Errorf("unexpected email: %s", email)
			}
			if passwordHash == "password123" {
				t.Error("password should be hashed before reaching the repository")
			}
			return want, nil
		},
	}
	var createdTokenHash string
	sessions := &fakeSessionRepository{
		createFunc: func(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
			createdTokenHash = tokenHash
			if userID != want.ID {
				t.Errorf("unexpected userID: %d", userID)
			}
			return nil
		},
	}
	s := auth.NewService(users, sessions)

	user, token, expiresAt, err := s.SignUp(context.Background(), " User@Example.com ", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != want {
		t.Errorf("got %v, want %v", user, want)
	}
	if token == "" {
		t.Error("expected a non-empty token to be created")
	}
	// リポジトリに渡るのは Cookie に入れる平文ではなく、そのハッシュ（BACKLOG 4-d）
	if createdTokenHash == token {
		t.Error("the plain token should not reach the repository")
	}
	if createdTokenHash != sha256Hex(token) {
		t.Errorf("stored %q, want the SHA-256 of the returned token", createdTokenHash)
	}
	if expiresAt.Before(time.Now()) {
		t.Error("expected expiresAt to be in the future")
	}
}

func TestService_Login_UserNotFound(t *testing.T) {
	users := &fakeUserRepository{
		findByEmailFunc: func(ctx context.Context, email string) (*auth.UserWithHash, error) {
			return nil, auth.ErrNotFound
		},
	}
	sessions := &fakeSessionRepository{}
	s := auth.NewService(users, sessions)

	_, _, _, err := s.Login(context.Background(), "nobody@example.com", "password123")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

// メールアドレスが見つからなくても bcrypt の比較を 1 回回し、パスワード違いと同程度の時間がかかる。
// 即座に返すと応答時間の差で登録済みのメールアドレスを判別できてしまう（BACKLOG 4-c）。
//
// 時間で測るのは不安定になりやすいので、「本物のハッシュとの比較の半分以上かかる」という
// 緩い条件にしている。bcrypt を回していなければ数 µs で返るので、それでも桁違いに区別できる。
func TestService_Login_UserNotFound_TakesAsLongAsWrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	found := auth.NewService(&fakeUserRepository{
		findByEmailFunc: func(ctx context.Context, email string) (*auth.UserWithHash, error) {
			return &auth.UserWithHash{User: domain.User{ID: 1, Email: email}, PasswordHash: string(hash)}, nil
		},
	}, &fakeSessionRepository{})
	notFound := auth.NewService(&fakeUserRepository{
		findByEmailFunc: func(ctx context.Context, email string) (*auth.UserWithHash, error) {
			return nil, auth.ErrNotFound
		},
	}, &fakeSessionRepository{})

	// ダミーハッシュは初回に作られるので、その分を測定から外す
	_, _, _, _ = notFound.Login(context.Background(), "nobody@example.com", "wrong-password")

	measure := func(s *auth.Service, email string) time.Duration {
		start := time.Now()
		_, _, _, err := s.Login(context.Background(), email, "wrong-password")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
		return time.Since(start)
	}
	wrongPassword := measure(found, "user@example.com")
	unknownEmail := measure(notFound, "nobody@example.com")

	if unknownEmail < wrongPassword/2 {
		t.Errorf("login for an unknown email took %s, much faster than a wrong password (%s)", unknownEmail, wrongPassword)
	}
}

func TestService_Login_WrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	users := &fakeUserRepository{
		findByEmailFunc: func(ctx context.Context, email string) (*auth.UserWithHash, error) {
			return &auth.UserWithHash{User: domain.User{ID: 1, Email: email}, PasswordHash: string(hash)}, nil
		},
	}
	sessions := &fakeSessionRepository{}
	s := auth.NewService(users, sessions)

	_, _, _, err = s.Login(context.Background(), "user@example.com", "wrong-password")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Login_Success(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	want := domain.User{ID: 1, Email: "user@example.com"}
	users := &fakeUserRepository{
		findByEmailFunc: func(ctx context.Context, email string) (*auth.UserWithHash, error) {
			return &auth.UserWithHash{User: want, PasswordHash: string(hash)}, nil
		},
	}
	sessions := &fakeSessionRepository{
		createFunc: func(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
			return nil
		},
	}
	s := auth.NewService(users, sessions)

	user, token, _, err := s.Login(context.Background(), "user@example.com", "correct-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != want.ID || user.Email != want.Email {
		t.Errorf("got %v, want %v", user, want)
	}
	if token == "" {
		t.Error("expected a non-empty token")
	}
}

func TestService_Logout(t *testing.T) {
	sessions := &fakeSessionRepository{}
	s := auth.NewService(&fakeUserRepository{}, sessions)

	if err := s.Logout(context.Background(), "a-token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sessions.deleteCalled {
		t.Error("expected sessions.Delete to be called")
	}
	// Cookie の平文ではなくハッシュで消す。平文で消そうとすると、どの行にも当たらずログアウトできない
	if sessions.deletedTokenHash != aTokenHash {
		t.Errorf("Delete got %q, want %q", sessions.deletedTokenHash, aTokenHash)
	}
}

func TestService_Logout_EmptyToken(t *testing.T) {
	sessions := &fakeSessionRepository{}
	s := auth.NewService(&fakeUserRepository{}, sessions)

	if err := s.Logout(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions.deleteCalled {
		t.Error("sessions.Delete should not be called for an empty token")
	}
}

func TestService_CurrentUser_EmptyToken(t *testing.T) {
	s := auth.NewService(&fakeUserRepository{}, &fakeSessionRepository{})

	_, err := s.CurrentUser(context.Background(), "")
	if !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_CurrentUser_DelegatesToRepository(t *testing.T) {
	want := &domain.User{ID: 1, Email: "user@example.com"}
	sessions := &fakeSessionRepository{
		// Cookie の平文ではなくハッシュで引く
		findUserFunc: func(ctx context.Context, tokenHash string) (*domain.User, error) {
			if tokenHash != aTokenHash {
				t.Errorf("FindUser got %q, want %q", tokenHash, aTokenHash)
			}
			return want, nil
		},
	}
	s := auth.NewService(&fakeUserRepository{}, sessions)

	got, err := s.CurrentUser(context.Background(), "a-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// usersWithPassword は userID 1 のパスワードが password であるリポジトリを返す。
func usersWithPassword(t *testing.T, password string) *fakeUserRepository {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	return &fakeUserRepository{
		passwordHashFunc: func(ctx context.Context, userID int64) (string, error) {
			if userID != 1 {
				return "", auth.ErrNotFound
			}
			return string(hash), nil
		},
	}
}

// 新しいパスワードが条件を満たさなければ、bcrypt もリポジトリも回さずに入力エラー（400）を返す。
// 条件は SignUp と同じ。現在と同じパスワードへの変更も、変えたつもりで何も変わらないので弾く。
func TestService_ChangePassword_Validation(t *testing.T) {
	tests := []struct {
		name        string
		newPassword string
	}{
		{"short password", "short"},
		{"password over 72 bytes", strings.Repeat("a", 73)},
		{"same as current", "current-password"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := usersWithPassword(t, "current-password")
			s := auth.NewService(users, &fakeSessionRepository{})

			err := s.ChangePassword(context.Background(), 1, "a-token", "current-password", tt.newPassword)
			var inputErr *auth.InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("expected InputError, got %v", err)
			}
			if users.called {
				t.Error("repository should not be called when validation fails")
			}
		})
	}
}

// 現在のパスワードが違えば差し替えない。開いたままの端末やセッションを盗んだ相手が、
// 本人を締め出せないようにするため。ログインの失敗（401）とは別のエラーにする。
func TestService_ChangePassword_WrongCurrentPassword(t *testing.T) {
	users := usersWithPassword(t, "current-password")
	s := auth.NewService(users, &fakeSessionRepository{})

	err := s.ChangePassword(context.Background(), 1, "a-token", "wrong-password", "new-password")
	if !errors.Is(err, auth.ErrWrongCurrentPassword) {
		t.Fatalf("expected ErrWrongCurrentPassword, got %v", err)
	}
	if users.updateCalled {
		t.Error("UpdatePassword should not be called when the current password is wrong")
	}
}

// 成功したら、新しいパスワードのハッシュを、いまのセッションのトークンのハッシュと一緒に渡す。
// リポジトリはそれ以外のセッションを消すので、他の端末はログアウトされ、いまの端末は残る。
// 平文で渡すとどの行とも一致せず、いまの端末まで消えてしまう。
func TestService_ChangePassword_Success(t *testing.T) {
	users := usersWithPassword(t, "current-password")
	s := auth.NewService(users, &fakeSessionRepository{})

	if err := s.ChangePassword(context.Background(), 1, "a-token", "current-password", "new-password"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !users.updateCalled {
		t.Fatal("expected UpdatePassword to be called")
	}
	if users.updatedUserID != 1 {
		t.Errorf("userID = %d, want 1", users.updatedUserID)
	}
	if users.updatedKeepTokenHash != aTokenHash {
		t.Errorf("keepTokenHash = %q, want %q", users.updatedKeepTokenHash, aTokenHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(users.updatedHash), []byte("new-password")); err != nil {
		t.Errorf("stored hash does not match the new password: %v", err)
	}
}

// ユーザーが見つからなければ ErrNotFound をそのまま返す（ハンドラは 401 にする）。
func TestService_ChangePassword_UserNotFound(t *testing.T) {
	users := usersWithPassword(t, "current-password")
	s := auth.NewService(users, &fakeSessionRepository{})

	err := s.ChangePassword(context.Background(), 2, "a-token", "current-password", "new-password")
	if !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
