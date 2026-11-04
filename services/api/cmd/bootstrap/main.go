// Command bootstrap creates a tenant and its owner. It runs as the migration owner, not the
// API role, so it is an operator tool, not an endpoint (DEC-014: no admin console yet).
//
// Usage (from services/api):
//
//	ADMIN_DATABASE_URL=postgresql://app:app@localhost:5432/app_dev \
//	  go run ./cmd/bootstrap -slug demo-school -name "Demo School" \
//	    -owner-mobile 03001234567 -owner-name "Zain Malik"
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("bootstrap failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var in iam.BootstrapInput
	flag.StringVar(&in.TenantSlug, "slug", "", "tenant slug, for example demo-school")
	flag.StringVar(&in.TenantName, "name", "", "tenant display name")
	flag.StringVar(&in.OwnerMobile, "owner-mobile", "", "owner mobile, 03XXXXXXXXX")
	flag.StringVar(&in.OwnerName, "owner-name", "", "owner full name")
	flag.Parse()

	url := os.Getenv("ADMIN_DATABASE_URL")
	if url == "" {
		return fmt.Errorf("ADMIN_DATABASE_URL is required (the migration owner, not app_rt)")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	tenantID, err := iam.Bootstrap(ctx, pool, in)
	if err != nil {
		return err
	}
	fmt.Printf("tenant %s created (slug %s). Sign in with OTP at POST /api/v1/auth/otp/request.\n", tenantID, in.TenantSlug)
	return nil
}
