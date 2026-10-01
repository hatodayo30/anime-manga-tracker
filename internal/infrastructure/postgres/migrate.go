package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// migrationsTable は適用済みバージョンを記録するテーブル。
	// golang-migrate の schema_migrations とは形が違う（あちらは単一行 + dirty フラグ）ので、
	// 将来あちらに乗り換えたときに衝突しないよう名前を分けておく。
	migrationsTable = "applied_migrations"

	// migrationLockID は pg_advisory_lock に渡す任意の識別子。
	// ECS は desiredCount: 1 固定なので本来レースは起きないが、デプロイ戦略の設定ミスや
	// 手元からの直接実行と重なったときに壊れないよう、取得コストの低い保険として掛けておく。
	migrationLockID int64 = 8923751104

	// baselineVersion は docker-entrypoint-initdb.d 方式で適用されていた最後のバージョン。
	// このランナー導入前に作られたローカルDBボリュームには 0001〜0004 が適用済みだが
	// applied_migrations が無い。そのまま流すと 0001 の CREATE TABLE が衝突するため、
	// 既存スキーマを検出したら初回だけこのバージョンまでを適用済みとして記録する。
	baselineVersion int64 = 4

	upSuffix = ".up.sql"
)

// migration は 1 本の up マイグレーション。
type migration struct {
	version int64
	name    string
	sql     string
}

func (m migration) filename() string {
	return fmt.Sprintf("%04d_%s%s", m.version, m.name, upSuffix)
}

// Migrate は fsys 内の *.up.sql のうち未適用のものをバージョン昇順に適用する。
// 各マイグレーションはトランザクションで囲むので、途中で失敗してもそのファイルは
// 全体が巻き戻り、適用済みとしては記録されない。
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	migrations, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	// アドバイザリロックはセッション単位なので、解放まで同じ接続を使い続ける必要がある。
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("migrate: acquire lock: %w", err)
	}
	defer func() {
		// ctx が既にキャンセルされていても解放は試みる。放置すると接続が
		// プールに返ったあともロックを握り続ける。
		if _, err := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockID); err != nil {
			log.Printf("migrate: release lock: %v", err)
		}
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+migrationsTable+` (
    version     BIGINT PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("migrate: create %s: %w", migrationsTable, err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		baselined, err := baselineExistingSchema(ctx, conn, migrations)
		if err != nil {
			return err
		}
		for _, v := range baselined {
			applied[v] = true
		}
	}

	count := 0
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", m.filename(), err)
		}
		log.Printf("migrate: applied %s", m.filename())
		count++
	}

	if count == 0 {
		log.Printf("migrate: already up to date (%d migrations)", len(migrations))
	}
	return nil
}

// loadMigrations は *.up.sql を読み込み、バージョン昇順で返す。
// ファイル名は golang-migrate の規約どおり NNNN_name.up.sql を前提とする。
func loadMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*"+upSuffix)
	if err != nil {
		return nil, fmt.Errorf("migrate: glob migrations: %w", err)
	}
	if len(names) == 0 {
		// embed の対象から漏れている（Dockerfile の COPY 忘れなど）。
		// 黙って「適用済み」として起動すると、空のDBでアプリが動き出してしまう。
		return nil, fmt.Errorf("migrate: no *%s found", upSuffix)
	}

	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		base := strings.TrimSuffix(name, upSuffix)
		sep := strings.Index(base, "_")
		if sep <= 0 {
			return nil, fmt.Errorf("migrate: %q does not match NNNN_name%s", name, upSuffix)
		}
		version, err := strconv.ParseInt(base[:sep], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migrate: %q has a non-numeric version: %w", name, err)
		}

		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    base[sep+1:],
			sql:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// 同じ番号が 2 本あると適用順が曖昧なまま片方だけ記録されるので、起動時に落とす。
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version == migrations[i-1].version {
			return nil, fmt.Errorf("migrate: duplicate version %d (%s, %s)",
				migrations[i].version, migrations[i-1].filename(), migrations[i].filename())
		}
	}

	return migrations, nil
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[int64]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM "+migrationsTable)
	if err != nil {
		return nil, fmt.Errorf("migrate: select applied versions: %w", err)
	}
	defer rows.Close()

	applied := map[int64]bool{}
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("migrate: scan applied version: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate applied versions: %w", err)
	}
	return applied, nil
}

// baselineExistingSchema は applied_migrations が空のDBについて、
// records テーブルが既にあれば baselineVersion までを適用済みとして記録する。
// 新規DB（records が無い）では何もしないので、全マイグレーションが普通に流れる。
func baselineExistingSchema(ctx context.Context, conn *pgxpool.Conn, migrations []migration) ([]int64, error) {
	var hasSchema bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('public.records') IS NOT NULL").Scan(&hasSchema); err != nil {
		return nil, fmt.Errorf("migrate: probe existing schema: %w", err)
	}
	if !hasSchema {
		return nil, nil
	}

	var baselined []int64
	for _, m := range migrations {
		if m.version > baselineVersion {
			break
		}
		if _, err := conn.Exec(ctx,
			"INSERT INTO "+migrationsTable+" (version, name) VALUES ($1, $2)", m.version, m.name,
		); err != nil {
			return nil, fmt.Errorf("migrate: baseline %s: %w", m.filename(), err)
		}
		baselined = append(baselined, m.version)
	}

	log.Printf("migrate: existing schema detected; marked %d migrations up to %04d as applied",
		len(baselined), baselineVersion)
	return baselined, nil
}

func apply(ctx context.Context, conn *pgxpool.Conn, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // Commit 済みなら no-op

	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO "+migrationsTable+" (version, name) VALUES ($1, $2)", m.version, m.name,
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
