// Package auth は認証・セッションまわりのユースケースを担う。
//
// NOTE: records の移行（usecase/record）とは異なり、ここでは repository の interface 抽出を
// まだ行っておらず、infrastructure/postgres の具体型に直接依存している（依存の向きが逆）。
// これは意図的な暫定措置で、auth 自体をクリーンアーキテクチャへ移行する段階で
// record と同様に Repository interface をこのパッケージ側に定義する。
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/infrastructure/postgres"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailTaken         = postgres.ErrEmailTaken
)

const SessionTTL = 30 * 24 * time.Hour

type Service struct {
	users    *postgres.UserRepository
	sessions *postgres.SessionRepository
}

func NewService(users *postgres.UserRepository, sessions *postgres.SessionRepository) *Service {
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
func (s *Service) Login(ctx context.Context, email, password string) (*domain.User, string, time.Time, error) {
	email = normalizeEmail(email)

	userWithHash, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
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
		return nil, postgres.ErrNotFound
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
