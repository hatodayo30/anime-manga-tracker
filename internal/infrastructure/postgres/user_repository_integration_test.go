package postgres_test

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/hatodayo30/anime-manga-tracker/internal/infrastructure/postgres"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
	"github.com/hatodayo30/anime-manga-tracker/migrations"
)

// 実 DB に対する結合テスト。usecase のテストはフェイクなので、SQL・制約名・マイグレーションのデータ移行は
// ここでしか確かめられない（BACKLOG 8-b）。
//
// TEST_DATABASE_URL が無ければスキップする。CI は go test を回していないので手元専用で、
// 使い捨てのデータベースを作ってから流す:
//
//	psql "postgres://anime_manga:anime_manga@localhost:5432/postgres" -c "CREATE DATABASE amt_google_auth_test"
//	TEST_DATABASE_URL=postgres://anime_manga:anime_manga@localhost:5432/amt_google_auth_test?sslmode=disable \
//	  go test ./internal/infrastructure/postgres -run Integration
//
// テストごとに public スキーマを作り直すので、データベース名が _test で終わらなければ失敗させる。
// 共有の開発 DB（anime_manga_tracker）を指したまま流すと、全ワークツリーのデータが消える。

// identityMigration は user_identities を作るマイグレーションの番号。これより前まで流した状態に
// 旧形式のユーザーを入れてから流すと、データ移行を確かめられる。
const identityMigration = "0007"

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if !strings.HasSuffix(strings.TrimPrefix(u.Path, "/"), "_test") {
		t.Fatalf("TEST_DATABASE_URL のデータベース名は _test で終わること（スキーマを作り直すため）: %q", u.Path)
	}

	ctx := context.Background()
	pool, err := postgres.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	return pool
}

// migrateBefore は version より前のマイグレーションだけを流す。
func migrateBefore(t *testing.T, pool *pgxpool.Pool, version string) {
	t.Helper()
	names, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	partial := fstest.MapFS{}
	for _, name := range names {
		if name < version {
			b, err := fs.ReadFile(migrations.FS, name)
			if err != nil {
				t.Fatal(err)
			}
			partial[name] = &fstest.MapFile{Data: b}
		}
	}
	if err := postgres.Migrate(context.Background(), pool, partial); err != nil {
		t.Fatalf("migrate before %s: %v", version, err)
	}
}

func migrateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if err := postgres.Migrate(context.Background(), pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func hash(t *testing.T, password string) string {
	t.Helper()
	// テストを速くするため最小の cost にする。比較は cost をハッシュから読むので結果は変わらない
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

// 0007 は既存ユーザーの password_hash を provider='password'・subject=users.id の行に移し、
// users からカラムを消す。移した後もそのパスワードでログインの照合ができる。
func TestIntegration_Migration0007_MovesPasswordHash(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateBefore(t, pool, identityMigration)

	var id int64
	h := hash(t, "old-password")
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('old@example.com', $1) RETURNING id`, h).Scan(&id); err != nil {
		t.Fatalf("insert old-style user: %v", err)
	}

	migrateAll(t, pool)

	var provider, subject, gotHash string
	if err := pool.QueryRow(ctx, `SELECT provider, subject, password_hash FROM user_identities WHERE user_id = $1`, id).
		Scan(&provider, &subject, &gotHash); err != nil {
		t.Fatalf("select identity: %v", err)
	}
	if provider != auth.ProviderPassword || subject != strconv.FormatInt(id, 10) || gotHash != h {
		t.Errorf("identity = (%q, %q, hash match %v), want (password, %d, true)", provider, subject, gotHash == h, id)
	}

	var hasColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'password_hash')
	`).Scan(&hasColumn); err != nil {
		t.Fatal(err)
	}
	if hasColumn {
		t.Error("users.password_hash should be dropped")
	}

	users := postgres.NewUserRepository(pool)
	u, err := users.FindByEmail(ctx, "old@example.com")
	if err != nil {
		t.Fatalf("FindByEmail after migration: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("old-password")) != nil {
		t.Error("migrated hash does not match the old password")
	}
}

// down はパスワードを users に戻し、Google だけのユーザーは消さずに空のハッシュにする（記録を失わないため）。
func TestIntegration_Migration0007_Down(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)
	users := postgres.NewUserRepository(pool)

	h := hash(t, "password-1")
	pw, err := users.Create(ctx, "pw@example.com", h)
	if err != nil {
		t.Fatal(err)
	}
	g, err := users.CreateWithIdentity(ctx, "g@example.com", auth.ProviderGoogle, "sub-g")
	if err != nil {
		t.Fatal(err)
	}

	down, err := fs.ReadFile(migrations.FS, identityMigration+"_user_identities.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}

	for _, tt := range []struct {
		id   int64
		want string
	}{{pw.ID, h}, {g.ID, ""}} {
		var got string
		if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, tt.id).Scan(&got); err != nil {
			t.Fatalf("user %d after down: %v", tt.id, err)
		}
		if got != tt.want {
			t.Errorf("user %d password_hash = %q, want %q", tt.id, got, tt.want)
		}
	}
}

// パスワードの登録・ログイン・変更が user_identities で動く。
func TestIntegration_PasswordUser(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)
	users := postgres.NewUserRepository(pool)

	u, err := users.Create(ctx, "user@example.com", hash(t, "password-1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := users.Create(ctx, "user@example.com", hash(t, "password-2")); !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("Create duplicate: expected ErrEmailTaken, got %v", err)
	}

	got, err := users.FindByEmail(ctx, "user@example.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindByEmail = %+v, %v", got, err)
	}

	if err := users.UpdatePassword(ctx, u.ID, hash(t, "password-3"), "keep-token"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	stored, err := users.PasswordHash(ctx, u.ID)
	if err != nil {
		t.Fatalf("PasswordHash: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored), []byte("password-3")) != nil {
		t.Error("PasswordHash does not match the updated password")
	}

	if _, err := users.PasswordHash(ctx, u.ID+1000); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("PasswordHash of missing user: expected ErrNotFound, got %v", err)
	}
}

// Google だけで登録したユーザーは、パスワードのログインでは見つからず（未登録と同じ経路になる）、
// パスワード変更では ErrNoPassword になる。sub で引ける。
func TestIntegration_GoogleOnlyUser(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)
	users := postgres.NewUserRepository(pool)

	u, err := users.CreateWithIdentity(ctx, "g@example.com", auth.ProviderGoogle, "sub-1")
	if err != nil {
		t.Fatalf("CreateWithIdentity: %v", err)
	}
	if got, err := users.FindByIdentity(ctx, auth.ProviderGoogle, "sub-1"); err != nil || got.ID != u.ID {
		t.Errorf("FindByIdentity = %+v, %v", got, err)
	}
	if _, err := users.FindByIdentity(ctx, auth.ProviderGoogle, "sub-unknown"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("FindByIdentity unknown: expected ErrNotFound, got %v", err)
	}
	if _, err := users.FindByEmail(ctx, "g@example.com"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("FindByEmail: expected ErrNotFound, got %v", err)
	}
	if _, err := users.PasswordHash(ctx, u.ID); !errors.Is(err, auth.ErrNoPassword) {
		t.Errorf("PasswordHash: expected ErrNoPassword, got %v", err)
	}
	if err := users.UpdatePassword(ctx, u.ID, hash(t, "password-1"), "keep-token"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("UpdatePassword: expected ErrNotFound, got %v", err)
	}
}

// CreateWithIdentity は 1 つのトランザクションで書くので、識別子の行で失敗したら users の行も残らない
// （どの手段でもログインできないユーザーが残らない）。メールアドレスの重複は ErrEmailTaken。
func TestIntegration_CreateWithIdentity_Atomic(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)
	users := postgres.NewUserRepository(pool)

	if _, err := users.CreateWithIdentity(ctx, "a@example.com", auth.ProviderGoogle, "sub-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := users.CreateWithIdentity(ctx, "a@example.com", auth.ProviderGoogle, "sub-2"); !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("duplicate email: expected ErrEmailTaken, got %v", err)
	}

	// 同じ sub で別のメールアドレス（初回ログインが並行したとき）
	if _, err := users.CreateWithIdentity(ctx, "b@example.com", auth.ProviderGoogle, "sub-1"); err == nil {
		t.Fatal("duplicate sub: expected error")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = 'b@example.com'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("user row should be rolled back when the identity insert fails")
	}
}

// LinkIdentityByEmail はパスワードのユーザーに Google を足す（パスワードでも入れるまま）。
// 別の Google アカウントが紐付き済みなら ErrIdentityTaken。これは制約名（UNIQUE (user_id, provider) の既定の名前）で
// 見分けているので、実 DB でしか確かめられない。
func TestIntegration_LinkIdentityByEmail(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)
	users := postgres.NewUserRepository(pool)

	pw, err := users.Create(ctx, "user@example.com", hash(t, "password-1"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.CreateWithIdentity(ctx, "other@example.com", auth.ProviderGoogle, "sub-other")
	if err != nil {
		t.Fatal(err)
	}

	linked, err := users.LinkIdentityByEmail(ctx, "user@example.com", auth.ProviderGoogle, "sub-1")
	if err != nil || linked.ID != pw.ID {
		t.Fatalf("LinkIdentityByEmail = %+v, %v", linked, err)
	}
	if got, err := users.FindByIdentity(ctx, auth.ProviderGoogle, "sub-1"); err != nil || got.ID != pw.ID {
		t.Errorf("FindByIdentity after link = %+v, %v", got, err)
	}
	if _, err := users.FindByEmail(ctx, "user@example.com"); err != nil {
		t.Errorf("password login should still work after link: %v", err)
	}
	// 紐付けた後は両方の手段が名前順で返る（/me がアカウント画面に渡す）
	if got, err := users.Providers(ctx, pw.ID); err != nil || !slices.Equal(got, []string{"google", "password"}) {
		t.Errorf("Providers after link = %v, %v; want [google password]", got, err)
	}
	if got, err := users.Providers(ctx, other.ID); err != nil || !slices.Equal(got, []string{"google"}) {
		t.Errorf("Providers of google-only user = %v, %v; want [google]", got, err)
	}
	if got, err := users.Providers(ctx, pw.ID+1000); err != nil || len(got) != 0 {
		t.Errorf("Providers of missing user = %v, %v; want empty", got, err)
	}

	if _, err := users.LinkIdentityByEmail(ctx, "user@example.com", auth.ProviderGoogle, "sub-2"); !errors.Is(err, auth.ErrIdentityTaken) {
		t.Errorf("second Google account: expected ErrIdentityTaken, got %v", err)
	}
	if _, err := users.LinkIdentityByEmail(ctx, "nobody@example.com", auth.ProviderGoogle, "sub-3"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("unknown email: expected ErrNotFound, got %v", err)
	}
	// 別のユーザーが使っている sub。ErrIdentityTaken（このユーザーに別の Google がある）とは区別する
	if _, err := users.LinkIdentityByEmail(ctx, "user@example.com", auth.ProviderGoogle, "sub-other"); err == nil || errors.Is(err, auth.ErrIdentityTaken) {
		t.Errorf("sub used by user %d: expected a non-ErrIdentityTaken error, got %v", other.ID, err)
	}
}

// password_hash はパスワードの行だけが持つ（CHECK 制約）。Google の行にハッシュが入ると、
// パスワードのログインの経路に乗ってしまう。
func TestIntegration_IdentityCheckConstraint(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)
	users := postgres.NewUserRepository(pool)

	u, err := users.CreateWithIdentity(ctx, "g@example.com", auth.ProviderGoogle, "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_identities SET password_hash = 'x' WHERE user_id = $1`, u.ID); err == nil {
		t.Error("google identity with password_hash should violate the CHECK constraint")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, 'password', 'p')`, u.ID); err == nil {
		t.Error("password identity without password_hash should violate the CHECK constraint")
	}
}

// usecase と実リポジトリを繋いだ通し: メールで登録した人が、確認済みの同じメールアドレスの Google で入ると
// 同じアカウントに紐付き、2 回目は sub で入れ、パスワードのログインもそのまま使える。
func TestIntegration_Service_PasswordThenGoogle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)

	google := &stubGoogle{profile: &auth.GoogleProfile{Subject: "sub-1", Email: "User@Example.com", EmailVerified: true}}
	s := auth.NewService(postgres.NewUserRepository(pool), postgres.NewSessionRepository(pool), auth.WithGoogle(google))

	signed, _, _, err := s.SignUp(ctx, "user@example.com", "password-1")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	first, _, _, err := s.LoginWithGoogle(ctx, "code", "verifier")
	if err != nil || first.ID != signed.ID {
		t.Fatalf("first Google login = %+v, %v; want user %d", first, err, signed.ID)
	}

	// Google 側でメールアドレスが変わり、未確認になっても sub で同じアカウントに入る
	google.profile = &auth.GoogleProfile{Subject: "sub-1", Email: "changed@example.com", EmailVerified: false}
	second, _, _, err := s.LoginWithGoogle(ctx, "code", "verifier")
	if err != nil || second.ID != signed.ID {
		t.Fatalf("second Google login = %+v, %v; want user %d", second, err, signed.ID)
	}

	if _, _, _, err := s.Login(ctx, "user@example.com", "password-1"); err != nil {
		t.Errorf("password login after linking: %v", err)
	}
}

// Google だけで登録したメールアドレスでは、パスワードのログインは失敗し（401）、メールでの登録は 409 になる。
func TestIntegration_Service_GoogleOnlyCannotUsePassword(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	migrateAll(t, pool)

	google := &stubGoogle{profile: &auth.GoogleProfile{Subject: "sub-1", Email: "g@example.com", EmailVerified: true}}
	s := auth.NewService(postgres.NewUserRepository(pool), postgres.NewSessionRepository(pool), auth.WithGoogle(google))

	if _, _, _, err := s.LoginWithGoogle(ctx, "code", "verifier"); err != nil {
		t.Fatalf("LoginWithGoogle: %v", err)
	}
	if _, _, _, err := s.Login(ctx, "g@example.com", "any-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("password login: expected ErrInvalidCredentials, got %v", err)
	}
	if _, _, _, err := s.SignUp(ctx, "g@example.com", "password-1"); !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("signup: expected ErrEmailTaken, got %v", err)
	}
}

type stubGoogle struct{ profile *auth.GoogleProfile }

func (g *stubGoogle) AuthCodeURL(state, verifier string) string { return "" }

func (g *stubGoogle) Profile(ctx context.Context, code, verifier string) (*auth.GoogleProfile, error) {
	return g.profile, nil
}
