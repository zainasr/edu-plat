package idempotency

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/httpx"
	"github.com/zainasr/edu-plat/services/api/internal/platform/idempotency/store/gen"
)

const (
	headerKey      = "Idempotency-Key"
	headerReplay   = "Idempotent-Replay"
	maxRequestBody = 1 << 20 // 1 MiB: the bodies of the endpoints below are small
	claimAttempts  = 3
)

// Operations are the routes that honour an Idempotency-Key, written as the router names them. The
// OpenAPI file lists the same set (parameter IdempotencyKey); TestINV_IdempotencyOperationsMatchTheContract
// keeps the two equal.
var Operations = []string{
	"POST /api/v1/challans/generate",
	"POST /api/v1/challans/release",
	"POST /api/v1/challans/{challanId}/void",
	"POST /api/v1/students",
}

// New returns middleware to register on the generated routes (so the matched route is known). whoIs says
// which school and which user the request is from; an anonymous request is passed through so the handler
// can answer 401. A request without an Idempotency-Key is passed through too: the key is optional.
// The key belongs to one user in one school: another user presenting it is refused, because a replay
// skips the handler and so skips its permission check.
func New(pool *pgxpool.Pool, whoIs func(*http.Request) (tenantID, actorID string, ok bool)) func(http.Handler) http.Handler {
	ops := make(map[string]bool, len(Operations))
	for _, o := range Operations {
		ops[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(headerKey)
			if key == "" || r.Method != http.MethodPost || !ops[r.Pattern] {
				next.ServeHTTP(w, r)
				return
			}
			tenantID, actorID, ok := whoIs(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if err := validateKey(key); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
			if err != nil {
				httpx.WriteError(w, r, apperr.Validation("request.invalid", "The request body could not be read or is larger than 1 MiB."))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			(&run{pool: pool, tenantID: tenantID, key: key, endpoint: r.Pattern,
				hash: requestHash(actorID, r.URL.Path, r.URL.RawQuery, body), next: next}).serve(w, r)
		})
	}
}

// run is one request that carries a key.
type run struct {
	pool     *pgxpool.Pool
	tenantID string
	key      string
	endpoint string
	hash     []byte
	next     http.Handler
}

func (x *run) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenant, err := db.ParseUUID(x.tenantID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	claimed, replay, err := x.claim(ctx, tenant)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if replay != nil {
		w.Header().Set("Content-Type", replay.contentType)
		w.Header().Set(headerReplay, "true")
		w.WriteHeader(replay.code)
		_, _ = w.Write(replay.body)
		return
	}

	// We hold the key. Run the handler into a buffer, store what it said, then send it. If the handler
	// panics the key is freed so the client can retry (the panic then continues up to net/http).
	finished := false
	defer func() {
		if !finished {
			x.finish(ctx, tenant, claimed, nil)
		}
	}()
	rec := &recorder{header: http.Header{}, code: http.StatusOK}
	x.next.ServeHTTP(rec, r)
	finished = true
	x.finish(ctx, tenant, claimed, rec)

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.code)
	_, _ = w.Write(rec.body.Bytes())
}

type stored struct {
	code        int
	contentType string
	body        []byte
}

// claim is the right to finish a key: its row and the owner token minted when it was taken.
type claim struct{ id, owner pgtype.UUID }

// claim takes the key, or says why it cannot. It returns the claim, or a stored response to
// replay, or an error (409 in progress, 422 reused, or a database error).
func (x *run) claim(ctx context.Context, tenant pgtype.UUID) (claim, *stored, error) {
	for range claimAttempts {
		var c claim
		var got bool
		err := db.WithTenant(ctx, x.pool, x.tenantID, func(tx pgx.Tx) error {
			row, err := gen.New(tx).ClaimKey(ctx, gen.ClaimKeyParams{TenantID: tenant, Key: x.key, Endpoint: x.endpoint, RequestHash: x.hash})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			c, got = claim{id: row.ID, owner: row.Owner}, err == nil
			return err
		})
		if err != nil {
			return claim{}, nil, err
		}
		if got {
			return c, nil, nil
		}

		var row gen.GetKeyRow
		var found bool
		err = db.WithTenant(ctx, x.pool, x.tenantID, func(tx pgx.Tx) error {
			var err error
			row, err = gen.New(tx).GetKey(ctx, gen.GetKeyParams{TenantID: tenant, Key: x.key})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			found = err == nil
			return err
		})
		if err != nil {
			return claim{}, nil, err
		}
		if !found {
			continue
		}
		ex := &Existing{Endpoint: row.Endpoint, Hash: row.RequestHash, Status: row.Status, Code: int(row.ResponseCode.Int32), Body: row.ResponseBody, Expired: row.Expired}
		switch decide(ex, x.endpoint, x.hash) {
		case Proceed:
			continue // it just failed or expired: try to take it
		case Replay:
			return claim{}, &stored{code: ex.Code, contentType: "application/json", body: ex.Body}, nil
		case InProgress:
			return claim{}, nil, errInProgress
		case Reused:
			return claim{}, nil, errReused
		}
	}
	return claim{}, nil, errInProgress
}

var (
	errInProgress = &apperr.Error{Status: http.StatusConflict, Code: "idempotency.in_progress",
		Message: "A request with this Idempotency-Key is still running. Try again in a moment."}
	errReused = &apperr.Error{Status: http.StatusUnprocessableEntity, Code: "idempotency.key_reused",
		Message: "This Idempotency-Key was already used with a different request. Use a new key for a new request."}
)

// finish stores a successful response (below 400) or frees the key. A client error means nothing was
// done, and a 5xx or panic means the run did not complete, so in both cases a retry should run again
// against the current state rather than replay an old failure (and a 403 must never be replayed to
// someone who has since been given the permission). It uses a context that outlives the client
// connection, so a dropped connection cannot leave the key stuck. Only the owner of the key can finish it.
func (x *run) finish(ctx context.Context, tenant pgtype.UUID, c claim, rec *recorder) {
	ctx = context.WithoutCancel(ctx)
	var lost bool
	err := db.WithTenant(ctx, x.pool, x.tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		var n int64
		var err error
		if rec != nil && rec.code < http.StatusBadRequest {
			n, err = q.CompleteKey(ctx, gen.CompleteKeyParams{
				TenantID: tenant, ID: c.id, Owner: c.owner,
				ResponseCode: pgtype.Int4{Int32: int32(rec.code), Valid: true}, ResponseBody: rec.body.Bytes(),
			})
		} else {
			n, err = q.FailKey(ctx, gen.FailKeyParams{TenantID: tenant, ID: c.id, Owner: c.owner})
		}
		lost = err == nil && n == 0
		return err
	})
	switch {
	case err != nil:
		// The client still gets the real response; the key stays in progress until it expires.
		slog.ErrorContext(ctx, "idempotency: could not store the response", "err", err)
	case lost:
		slog.WarnContext(ctx, "idempotency: the key was taken over before this request finished; its response was not stored")
	}
}

// recorder holds a handler's response until we decide what to do with it.
type recorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) WriteHeader(code int)        { r.code = code }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
