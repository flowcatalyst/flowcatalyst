//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// End to end: the subcommand finds the database the server would use, moves
// an old-key value to the current key, and a second run finds nothing to do.
func TestReencryptSubcommand(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()
	oldSvc, _ := encryption.New(oldKey)
	ct, _ := oldSvc.Encrypt("totp-seed")
	id := fmt.Sprintf("mfa_kr%d", time.Now().UnixNano()%1_000_000_000)
	prn := "prn_kr" + id[6:]
	_, err := pool.Exec(ctx, `INSERT INTO iam_principals (id, type, name) VALUES ($1, 'USER', 'kr')`, prn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO iam_user_mfa_methods (id, principal_id, method, secret_encrypted) VALUES ($1, $2, 'TOTP', $3)`,
		id, prn, ct); err != nil {
		t.Fatal(err)
	}

	t.Setenv(encryption.EnvAppKey, newKey)
	t.Setenv(encryption.EnvAppKeyPrevious, oldKey)
	t.Setenv("FC_DATABASE_URL", pool.Config().ConnString())

	var out, errOut bytes.Buffer
	if code := runReencrypt([]string{"--dry-run"}, &out, &errOut); code != reencryptOK {
		t.Fatalf("dry run exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "WOULD_REENCRYPT") || !strings.Contains(out.String(), "iam_user_mfa_methods.secret_encrypted") {
		t.Fatalf("dry-run output: %s", out.String())
	}
	var stored string
	_ = pool.QueryRow(ctx, `SELECT secret_encrypted FROM iam_user_mfa_methods WHERE id = $1`, id).Scan(&stored)
	if stored != ct {
		t.Fatal("a dry run wrote")
	}

	out.Reset()
	if code := runReencrypt(nil, &out, &errOut); code != reencryptOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	_ = pool.QueryRow(ctx, `SELECT secret_encrypted FROM iam_user_mfa_methods WHERE id = $1`, id).Scan(&stored)
	newOnly, _ := encryption.New(newKey)
	if pt, err := newOnly.Decrypt(stored); err != nil || pt != "totp-seed" {
		t.Fatalf("after reencrypt the new key alone reads %q, %v", pt, err)
	}
}
