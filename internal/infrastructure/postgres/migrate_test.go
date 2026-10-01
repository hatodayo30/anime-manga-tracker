package postgres

import (
	"testing"
	"testing/fstest"

	"github.com/hatodayo30/anime-manga-tracker/migrations"
)

// loadMigrations はファイル名の辞書順ではなくバージョンの数値順に並べる
// （0010 と 0009 が混ざったときに辞書順でも一致してしまうため、桁数の違う番号で確かめる）。
func TestLoadMigrationsSortsByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"0010_tenth.up.sql":  {Data: []byte("SELECT 10;")},
		"0002_second.up.sql": {Data: []byte("SELECT 2;")},
		"0001_first.up.sql":  {Data: []byte("SELECT 1;")},
		// down は読み込み対象にしない。
		"0001_first.down.sql": {Data: []byte("SELECT -1;")},
	}

	got, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	want := []migration{
		{version: 1, name: "first", sql: "SELECT 1;"},
		{version: 2, name: "second", sql: "SELECT 2;"},
		{version: 10, name: "tenth", sql: "SELECT 10;"},
	}
	if len(got) != len(want) {
		t.Fatalf("loaded %d migrations, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("migration[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadMigrationsErrors(t *testing.T) {
	tests := map[string]fstest.MapFS{
		// embed から漏れている状態。黙って起動させない。
		"no up migrations": {
			"0001_init.down.sql": {Data: []byte("SELECT 1;")},
		},
		// 適用順が曖昧になる。
		"duplicate version": {
			"0001_init.up.sql":  {Data: []byte("SELECT 1;")},
			"0001_other.up.sql": {Data: []byte("SELECT 2;")},
		},
		"non numeric version": {
			"init_first.up.sql": {Data: []byte("SELECT 1;")},
		},
		"missing separator": {
			"0001.up.sql": {Data: []byte("SELECT 1;")},
		},
	}

	for name, fsys := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(fsys); err == nil {
				t.Fatal("loadMigrations returned nil error, want failure")
			}
		})
	}
}

// 実際に embed されている migrations/ が規約を満たしていることを確かめる。
// 番号の付け間違いや embed 漏れは起動時まで気付けないため、ここで落とす。
func TestLoadEmbeddedMigrations(t *testing.T) {
	got, err := loadMigrations(migrations.FS)
	if err != nil {
		t.Fatalf("loadMigrations(migrations.FS): %v", err)
	}

	for i, m := range got {
		if want := int64(i + 1); m.version != want {
			t.Errorf("migration[%d].version = %d, want %d (番号は1から連番にすること)", i, m.version, want)
		}
		if m.sql == "" {
			t.Errorf("%s is empty", m.filename())
		}
	}

	// baselineVersion を超えるマイグレーションが既存DBに流れることは意図どおりだが、
	// 下回っていたら baseline の対象を取りこぼしている。
	if last := got[len(got)-1].version; last < baselineVersion {
		t.Errorf("最新バージョン %d が baselineVersion %d を下回っている", last, baselineVersion)
	}
}
