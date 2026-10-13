package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestInviteLifecycle(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	st, err := New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	email := "invite-" + time.Now().Format("150405.000000") + "@example.test"
	t.Cleanup(func() { _, _ = st.Pool.Exec(ctx, `DELETE FROM admin_users WHERE username=$1`, email) })

	raw, _, err := st.CreateInvite(ctx, "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !st.InviteValid(ctx, raw) {
		t.Fatal("fresh invite must be valid")
	}
	uid, err := st.AcceptInvite(ctx, raw, email, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AcceptInvite(ctx, raw, "other-"+email, "correct horse battery"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("reused invite: %v", err)
	}
	if _, err = st.Authenticate(ctx, email, "correct horse battery"); err != nil {
		t.Fatal(err)
	}

	again, _, _ := st.CreateInvite(ctx, "", time.Hour)
	if _, err = st.AcceptInvite(ctx, again, email, "correct horse battery"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate email: %v", err)
	}
	if !st.InviteValid(ctx, again) {
		t.Fatal("failed accept must not burn the invite")
	}
	expired, _, _ := st.CreateInvite(ctx, "", -time.Minute)
	if _, err = st.AcceptInvite(ctx, expired, "x-"+email, "correct horse battery"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expired invite: %v", err)
	}

	sess, _, _ := st.CreateAdminSession(ctx, uid, time.Hour)
	if err = st.SetUserActive(ctx, uid, false); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AdminSession(ctx, sess); err == nil {
		t.Fatal("disabled user session must end")
	}
	if _, err = st.Authenticate(ctx, email, "correct horse battery"); err == nil {
		t.Fatal("disabled user must not log in")
	}
}
