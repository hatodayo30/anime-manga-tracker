---
description: 変更をcommit・push・PR作成する
argument-hint: [commit message (省略可)]
allowed-tools: Bash(git status:*), Bash(git diff:*), Bash(git add:*), Bash(git commit:*), Bash(git branch:*), Bash(git rev-parse:*), Bash(git push:*), Bash(gh pr create:*), Bash(gh pr view:*)
---

## Context

- 現在のブランチ: !`git branch --show-current`
- 未 commit の変更: !`git status --short`
- ステージ前の差分: !`git diff`
- 直近のコミット: !`git log --oneline -5`

## Task

以下を順番に実行してください。途中で失敗したら止まって状況を報告し、次に進まないでください。

### 1. commit

- 上記の差分を確認し、意味のある単位でステージ（`git add`）して commit する
- コミットメッセージは `$ARGUMENTS` が指定されていればそれを使う。指定がなければ差分の内容から日本語で簡潔なメッセージを作成する（Conventional Commits 形式: `feat: ...` / `fix: ...` / `refactor: ...` など）
- このリポジトリは Go 1.25（net/http ベース、handler→service→repository→model のレイヤード構成）+ PostgreSQL(pgx/v5) + Vanilla JS フロントエンドの構成。コミット単位が複数レイヤーにまたがる場合、無理に分割はせず 1 コミットでよい

### 2. push

- 現在のブランチが main/master であれば、変更内容から適切なブランチ名（例: `feat/xxx`, `fix/xxx`）を考えて新しいブランチを作成して commit し直す
- リモートに push する（`git push -u origin <branch>`）

### 3. PR 作成

- `gh pr create` で PR を作成する
- タイトルはコミットメッセージに準拠
- 本文には「変更内容」「変更理由」「動作確認方法（あれば）」を日本語で簡潔に書く
- base ブランチは明示的に指定しない場合はリポジトリのデフォルトブランチにする

### 4. 完了報告

- 作成した PR の URL を報告する
- push 後、`ci.yml`（gofmt/go vet/go build）が自動で走るので、「Actions タブで結果を確認してください」の一言を添える
