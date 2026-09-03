package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	firebase "firebase.google.com/go/v4"

	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
	pgadapter "github.com/garcia-lorca/services/iam/internal/adapters/postgres"
)

// maxImportBatch is the provider's own ceiling for one ImportUsers call.
const maxImportBatch = 1000

/*
`iam import-users` — F3 of .claude/docs/PLAN-identity-platform.md.

Copies the accounts we already have into Identity Platform, WITH their bcrypt
hashes, so every user keeps the password they have today. That is the point:
nobody is locked out at cutover and nobody has to read an email before they can
work. With a demo running daily, "everybody resets first" was the largest risk
in the plan.

It ships inside the service binary rather than as its own image because it needs
exactly what the service already knows how to build: the database connection,
the tenant, the postgres adapter. A second binary would have been a second copy
of all of it.

Idempotent. The provider upserts by uid, and the uid we send IS our user id, so
running it twice converges instead of duplicating. Safe to re-run after fixing
one bad row.

	IDENTITY_PLATFORM_PROJECT=garcia-lorca ./iam import-users
*/
func runImportUsers(log *slog.Logger) error {
	project := os.Getenv("IDENTITY_PLATFORM_PROJECT")
	if project == "" {
		return fmt.Errorf("IDENTITY_PLATFORM_PROJECT must be set to import users")
	}
	ctx := context.Background()

	db, err := openDB()
	if err != nil {
		return err
	}
	tenantID := envOr("IAM_TENANT_ID", defaultTenant)
	users := pgadapter.NewUserRepo(db, tenantID)

	all, err := users.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("listing users: %w", err)
	}
	if len(all) == 0 {
		log.Info("nothing to import", "users", 0)
		return nil
	}

	// Read each user's hash through the repository rather than reaching into the
	// table. One query per user is the wrong shape for a request path and
	// exactly the right shape here: this runs once, over tens of accounts, and
	// it costs no new port method that the service would then carry forever.
	batch := make([]googleid.ImportUser, 0, len(all))
	for _, u := range all {
		_, hash, err := users.FindAuthByID(ctx, u.ID())
		if err != nil {
			return fmt.Errorf("reading credentials for %s: %w", u.ID(), err)
		}
		imp, err := googleid.ToImportUser(u, hash)
		if err != nil {
			// Refuse the whole run rather than skip a row. A user silently left
			// behind is a user who cannot sign in after the cutover, and nobody
			// would find out until they tried.
			return fmt.Errorf("preparing %s: %w", u.Email().String(), err)
		}
		batch = append(batch, imp)
	}

	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: project})
	if err != nil {
		return fmt.Errorf("identity provider init: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return fmt.Errorf("identity provider auth client: %w", err)
	}

	imported := 0
	for start := 0; start < len(batch); start += maxImportBatch {
		end := min(start+maxImportBatch, len(batch))
		chunk := batch[start:end]

		res, err := client.ImportUsers(ctx, googleid.ToSDKUsers(chunk), googleid.BcryptOption())
		if err != nil {
			return fmt.Errorf("importing users [%d:%d]: %w", start, end, err)
		}
		for _, e := range res.Errors {
			// Name the account, not just the index. An operator reading this
			// needs to know WHICH person cannot sign in.
			log.Error("user rejected by the provider",
				"email", chunk[e.Index].Email, "reason", e.Reason)
		}
		if res.FailureCount > 0 {
			return fmt.Errorf("provider rejected %d of %d users; nothing further imported", res.FailureCount, len(chunk))
		}
		imported += res.SuccessCount
	}

	log.Info("users imported with their existing passwords", "count", imported, "project", project)
	return nil
}
