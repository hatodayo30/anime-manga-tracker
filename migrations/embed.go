// Package migrations は SQL マイグレーションファイルをバイナリに焼き込んで公開する。
//
// RDS では docker-entrypoint-initdb.d が使えないため、起動時に
// internal/infrastructure/postgres.Migrate から適用する。SQL をこのディレクトリに
// 置いたまま embed できるよう、パッケージ自体を migrations/ に同居させている
// （//go:embed は親ディレクトリを辿れない）。
package migrations

import "embed"

// FS は migrations/*.sql を保持する。
//
// 起動時に使うのは *.up.sql だけだが、*.down.sql も焼き込んでおく。ECS Exec で
// 実行中タスクに入って手でロールバックを流すとき、ファイルを持ち込まずに済む。
//
//go:embed *.sql
var FS embed.FS
