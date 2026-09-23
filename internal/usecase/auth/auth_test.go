package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

type fakeUserRepository struct {
	createFunc      func(ctx context.Context, email, passwordHash string) (*domain.User, error)
	findByEmailFunc func(ctx context.Context, email string) (*auth.UserWithHash, error)
	called          bool
}

func (f *fakeUserRepository) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	f.called = true
	return f.createFunc(ctx, email, passwordHash)
}

func (f *fakeUserRepository) FindByEmail(ctx context.Context, email string) (*auth.UserWithHash, error) {
	f.called = true
	return f.findByEmailFunc(ctx, email)
}

type fakeSessionRepository struct {
	createFunc   func(ctx context.Context, token string, userID int64, expiresAt time.Time) error
	findUserFunc func(ctx context.Context, token string) (*domain.User, error)
	deleteFunc   func(ctx context.Context, token string) error
	deleteCalled bool
}

func (f *fakeSessionRepository) Create(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	if f.createFunc == nil {
		return nil
	}
	return f.createFunc(ctx, token, userID, expiresAt)
}

func (f *fakeSessionRepository) FindUser(ctx context.Context, token string) (*domain.User, error) {
	return f.findUserFunc(ctx, token)
}

func (f *fakeSessionRepository) Delete(ctx context.Context, token string) error {
	f.deleteCalled = true
	if f.deleteFunc == nil {
		return nil
	}
	return f.deleteFunc(ctx, token)
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
	var createdToken string
	sessions := &fakeSessionRepository{
		createFunc: func(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
			createdToken = token
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
	if token == "" || token != createdToken {
		t.Error("expected a non-empty token to be created")
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
		createFunc: func(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
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

	if err := s.Logout(context.Background(), "some-token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sessions.deleteCalled {
		t.Error("expected sessions.Delete to be called")
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
		findUserFunc: func(ctx context.Context, token string) (*domain.User, error) {
			if token != "a-token" {
				t.Errorf("unexpected token: %s", token)
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
