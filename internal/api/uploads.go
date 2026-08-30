package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/auth"
	ghapp "github.com/ColdCrabby/cloud-presets/internal/github"
	"github.com/ColdCrabby/cloud-presets/internal/preset"
	"github.com/ColdCrabby/cloud-presets/internal/submit"
	"github.com/ColdCrabby/cloud-presets/internal/upload"
)

// Upload limits. Drafts are held in memory and claimable by an unguessable id,
// so the request body, the number of files, and the total decompressed size are
// all bounded to keep an abandoned or hostile upload — including a zip bomb —
// from consuming the process. Upload and read are unauthenticated (the id is the
// capability), so the caps are the only backstop there.
const (
	maxUploadBytes = 16 << 20 // 16 MiB per file (compressed, over the wire)
	maxDraftBytes  = 64 << 20 // 64 MiB total decompressed across all files
	maxDraftFiles  = 500      // files per upload, including zip entries
	claimPathBase  = "/vendor/claim/"
)

// uploadDeps are the collaborators the upload/claim handlers need. They are
// assembled by api.New from options; when the store is nil (the OpenAPI export,
// or a deployment with the validator unavailable) the operations are still
// registered so they appear in the spec, but every call returns 503.
type uploadDeps struct {
	validator *preset.Validator
	store     *upload.Store
	submitter submit.Submitter // nil when the GitHub bot is not configured
}

// DraftFileView is one uploaded preset as reported back to the client. Content
// is included so the admin app can preview the exact YAML before claiming.
type DraftFileView struct {
	Kind     string `json:"kind" doc:"Preset category: printer, filament, or process."`
	ID       string `json:"id" doc:"Preset id declared in the file body."`
	Name     string `json:"name" doc:"Human-readable preset name."`
	Vendor   string `json:"vendor,omitempty" doc:"Vendor string declared in the body, when applicable."`
	FileName string `json:"fileName" doc:"Leaf file name the preset is stored as (<id>.yaml)."`
	Content  string `json:"content" doc:"The original, unmodified uploaded YAML."`
}

// uploadForm is the multipart body of POST /v1/uploads: one or more preset files
// under the repeated form field "files". A .zip laid out like the presets repo
// is expanded server-side; bare YAML files take their kind from the file name or
// the `type` query parameter.
type uploadForm struct {
	Files []huma.FormFile `form:"files" contentType:"application/octet-stream" required:"true"`
}

// UploadInput is the request for POST /v1/uploads.
type UploadInput struct {
	Type    string `query:"type" enum:"printer,filament" doc:"Default preset type for bare files whose name does not encode one (printers/ or filaments/). Ignored for zip entries, whose kind comes from their path."`
	RawBody huma.MultipartFormFiles[uploadForm]
}

// UploadOutput is the parked draft, plus the admin URL to claim it.
type UploadOutput struct {
	Body struct {
		ID        string          `json:"id" doc:"Unguessable id of the parked draft."`
		ClaimURL  string          `json:"claimUrl" doc:"Admin app path to review and claim the draft."`
		ExpiresAt time.Time       `json:"expiresAt" doc:"When the draft is garbage-collected if unclaimed."`
		Files     []DraftFileView `json:"files" doc:"The validated preset files in the draft."`
	}
}

// GetUploadInput addresses a parked draft by id.
type GetUploadInput struct {
	ID string `path:"id" doc:"Draft id returned by POST /v1/uploads."`
}

// GetUploadOutput is a parked draft for review.
type GetUploadOutput struct {
	Body struct {
		ID        string          `json:"id"`
		CreatedAt time.Time       `json:"createdAt"`
		ExpiresAt time.Time       `json:"expiresAt"`
		Files     []DraftFileView `json:"files"`
	}
}

// ClaimInput claims a draft. The optional vendor slug is only honoured in dev
// (no GitHub bot); with the bot configured the slug is resolved from the
// manifest at head and the body is ignored. The body itself is optional — a
// claim with no body is valid, since authority comes from the session, not the
// request.
type ClaimInput struct {
	ID   string     `path:"id" doc:"Draft id to claim."`
	Body *claimBody `required:"false"`
}

// claimBody is the optional JSON body of a claim.
type claimBody struct {
	Vendor string `json:"vendor,omitempty" doc:"Vendor slug to attribute the change to (dev only; ignored when the GitHub bot is configured)."`
}

// ClaimOutput reports the result of claiming a draft.
type ClaimOutput struct {
	Body struct {
		Claimed        bool     `json:"claimed" doc:"Always true on success."`
		PRCreated      bool     `json:"prCreated" doc:"Whether a pull request was opened (false in dev without the bot)."`
		Vendor         string   `json:"vendor" doc:"Vendor slug the change was attributed to."`
		Files          []string `json:"files" doc:"Repository paths the presets were placed at."`
		PullRequestURL string   `json:"pullRequestUrl,omitempty" doc:"URL of the opened pull request, when one was created."`
		Branch         string   `json:"branch,omitempty" doc:"Head branch the change was committed to."`
		AlreadyExisted bool     `json:"alreadyExisted,omitempty" doc:"True when an idempotent retry returned an existing pull request."`
		Message        string   `json:"message,omitempty" doc:"Human-readable note, e.g. when no pull request was opened."`
	}
}

// registerUploads wires the manual upload and claim operations. They are
// first-class Huma operations, so they appear in the OpenAPI document and the
// generated client — the admin app calls them through the typed SDK, with the
// session token attached by the shared client, rather than by hand.
//
// The operations are always registered (even when the store is nil) so the spec
// is stable regardless of deployment configuration; handlers return 503 when the
// feature is not wired. The claim operation carries the Stytch auth middleware
// when one is configured: only a signed-in vendor can turn a parked draft into a
// pull request. Upload and read are intentionally open — like the slicer
// hand-off, the id authorizes nothing, and authority is checked at claim time.
func registerUploads(api huma.API, mw *auth.Middleware, deps uploadDeps) {
	h := &uploadHandler{deps: deps}

	huma.Register(api, huma.Operation{
		OperationID:   "uploadPresets",
		Method:        http.MethodPost,
		Path:          BasePath + "/uploads",
		DefaultStatus: http.StatusCreated,
		Summary:       "Upload presets to park for claiming",
		Description: "Accepts preset YAML files, or a .zip laid out like the presets " +
			"repository, validates each against the pinned schemas, and parks the " +
			"result as a short-lived draft. Returns the draft id and the admin URL to claim it.",
		Tags: []string{"Uploads"},
	}, h.upload)

	huma.Register(api, huma.Operation{
		OperationID: "getUpload",
		Method:      http.MethodGet,
		Path:        BasePath + "/uploads/{id}",
		Summary:     "Read a parked upload",
		Description: "Returns a parked upload draft so the admin app can render it for " +
			"review. The id is the only capability required; ownership is checked at claim time.",
		Tags: []string{"Uploads"},
	}, h.getDraft)

	claimOp := huma.Operation{
		OperationID: "claimUpload",
		Method:      http.MethodPost,
		Path:        BasePath + "/uploads/{id}/claim",
		Summary:     "Claim an upload and open a pull request",
		Description: "Authorizes the caller's organization against the vendor manifest " +
			"at the current head, re-validates the draft, and opens a pull request through " +
			"the bot. Requires a Stytch session JWT.",
		Tags: []string{"Uploads"},
	}
	if mw != nil {
		claimOp.Middlewares = huma.Middlewares{requireAuthOp(api, mw)}
	}
	huma.Register(api, claimOp, h.claim)
}

// requireAuthOp adapts the Stytch middleware to a Huma operation middleware. It
// reads the bearer token off the huma.Context, runs the same offline
// verification as RequireAuth, and either writes a 401 or injects the validated
// claims into the request context for the handler to read.
func requireAuthOp(api huma.API, mw *auth.Middleware) func(huma.Context, func(huma.Context)) {
	return func(hctx huma.Context, next func(huma.Context)) {
		token := auth.BearerFromHeader(hctx.Header("Authorization"))
		claims, err := mw.Verify(hctx.Context(), token)
		if err != nil {
			hctx.SetHeader("WWW-Authenticate", `Bearer error="invalid_token", error_description="`+auth.Reason(err)+`"`)
			_ = huma.WriteErr(api, hctx, http.StatusUnauthorized, auth.Reason(err))
			return
		}
		next(huma.WithContext(hctx, auth.WithClaims(hctx.Context(), claims)))
	}
}

type uploadHandler struct {
	deps uploadDeps
}

// fileError is a per-file validation failure, converted to structured Huma
// error details so the admin app can point at the offending upload and field.
type fileError struct {
	File    string
	Kind    string
	Message string
	Errors  []preset.Error
}

// upload handles POST /v1/uploads.
func (h *uploadHandler) upload(_ context.Context, in *UploadInput) (*UploadOutput, error) {
	if h.deps.store == nil {
		return nil, huma.Error503ServiceUnavailable("manual upload is not enabled on this server")
	}
	// The multipart form may spill large files to temp storage; clean it up.
	if form := in.RawBody.Form; form != nil {
		defer func() { _ = form.RemoveAll() }()
	}

	defaultKind := preset.Kind(strings.TrimSpace(in.Type))
	files, fileErrs, err := h.collectFiles(in.RawBody.Data().Files, defaultKind)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if len(fileErrs) > 0 {
		return nil, huma.Error422UnprocessableEntity(
			"one or more uploaded presets are invalid", validationDetails(fileErrs)...)
	}
	if len(files) == 0 {
		return nil, huma.Error400BadRequest(
			"no preset files were found in the upload; send preset YAML files or a .zip laid out like the presets repository")
	}

	draft, err := h.deps.store.Create(files)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not store the upload", err)
	}

	out := &UploadOutput{}
	out.Body.ID = draft.ID
	out.Body.ClaimURL = claimPathBase + draft.ID
	out.Body.ExpiresAt = draft.ExpiresAt
	out.Body.Files = draftFileViews(draft)
	return out, nil
}

// getDraft handles GET /v1/uploads/{id}.
func (h *uploadHandler) getDraft(_ context.Context, in *GetUploadInput) (*GetUploadOutput, error) {
	if h.deps.store == nil {
		return nil, huma.Error503ServiceUnavailable("manual upload is not enabled on this server")
	}
	draft, err := h.deps.store.Get(in.ID)
	if err != nil {
		return nil, huma.Error404NotFound("this upload was not found or has expired")
	}
	out := &GetUploadOutput{}
	out.Body.ID = draft.ID
	out.Body.CreatedAt = draft.CreatedAt
	out.Body.ExpiresAt = draft.ExpiresAt
	out.Body.Files = draftFileViews(draft)
	return out, nil
}

// claim handles POST /v1/uploads/{id}/claim. It resolves the caller's writable
// vendor namespace, re-validates the draft against the pinned schemas, and — when
// the GitHub bot is configured — opens a pull request. In dev it returns the
// resolved change set without opening a PR, so the flow is exercisable end to end
// without credentials.
func (h *uploadHandler) claim(ctx context.Context, in *ClaimInput) (*ClaimOutput, error) {
	if h.deps.store == nil {
		return nil, huma.Error503ServiceUnavailable("manual upload is not enabled on this server")
	}
	draft, err := h.deps.store.Get(in.ID)
	if err != nil {
		return nil, huma.Error404NotFound("this upload was not found or has expired")
	}

	claims, hasClaims := auth.ClaimsFromContext(ctx)

	// A process preset lives in the shared, project-owned namespace and is not
	// writable through the API by any vendor. Reject before touching GitHub.
	for _, f := range draft.Files {
		if f.Kind == preset.KindProcess {
			return nil, huma.Error403Forbidden(
				"process presets belong to the shared processes/ namespace and cannot be submitted through Vendor Admin")
		}
	}

	slug, err := h.resolveVendor(ctx, claims, requestedVendor(in.Body))
	if err != nil {
		if errors.Is(err, submit.ErrVendorNotFound) {
			return nil, huma.Error403Forbidden(
				"your organization does not own a vendor namespace; ask a maintainer to add your Stytch organization to a vendor.yaml")
		}
		return nil, upstreamError(err)
	}

	files, fileErrs := h.buildRepoFiles(slug, draft)
	if len(fileErrs) > 0 {
		return nil, huma.Error422UnprocessableEntity(
			"the uploaded presets no longer validate for this vendor", validationDetails(fileErrs)...)
	}

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}

	out := &ClaimOutput{}
	out.Body.Claimed = true
	out.Body.Vendor = slug
	out.Body.Files = paths

	// Without the bot we cannot open a pull request. Return the resolved,
	// validated change set so the flow still completes visibly in dev.
	if h.deps.submitter == nil {
		out.Body.PRCreated = false
		out.Body.Message = "The GitHub App bot is not configured, so no pull request was opened. Configure it to enable submissions."
		return out, nil
	}

	req := submit.Request{
		Branch:        branchName(slug, files),
		Title:         fmt.Sprintf("%s: update %s via Vendor Admin", slug, pluralPresets(len(files))),
		Body:          prBody(slug, claims, hasClaims, paths),
		CommitMessage: commitMessage(slug, claims, hasClaims, len(files)),
		Files:         files,
	}
	res, err := h.deps.submitter.Submit(ctx, req)
	if err != nil {
		return nil, upstreamError(err)
	}

	// The pull request is now the record; the draft has served its purpose.
	h.deps.store.Delete(draft.ID)
	out.Body.PRCreated = true
	out.Body.PullRequestURL = res.URL
	out.Body.Branch = res.Branch
	out.Body.AlreadyExisted = res.AlreadyExisted
	return out, nil
}

// requestedVendor safely reads the optional vendor slug from a possibly-nil body.
func requestedVendor(b *claimBody) string {
	if b == nil {
		return ""
	}
	return b.Vendor
}

// resolveVendor determines the vendor slug the caller may write to. With the bot
// configured, authority comes from the manifest at head. Without it (dev), the
// slug is taken from the request body or the caller's organization slug so the
// flow is testable without GitHub.
func (h *uploadHandler) resolveVendor(ctx context.Context, claims auth.Claims, requested string) (string, error) {
	if h.deps.submitter != nil {
		return h.deps.submitter.ResolveVendorSlug(ctx, claims.OrganizationID)
	}
	if s := strings.TrimSpace(requested); s != "" {
		return s, nil
	}
	if s := strings.TrimSpace(claims.OrganizationSlug); s != "" {
		return s, nil
	}
	return "", submit.ErrVendorNotFound
}

// buildRepoFiles maps each draft file to its path under the vendor's directory
// and re-validates it against the pinned schemas. The path is derived from the
// kind, so a vendor can never place a file outside its own namespace.
func (h *uploadHandler) buildRepoFiles(slug string, draft *upload.Draft) ([]submit.File, []fileError) {
	var files []submit.File
	var errs []fileError
	for _, f := range draft.Files {
		sub, ok := kindDir(f.Kind)
		if !ok {
			errs = append(errs, fileError{File: f.FileName, Kind: string(f.Kind),
				Message: "unsupported preset kind for a vendor submission"})
			continue
		}
		repoPath := path.Join("vendors", slug, sub, f.FileName)
		if h.deps.validator != nil {
			if res := h.deps.validator.ValidateFile(f.Kind, f.FileName, f.Content); !res.Valid() {
				errs = append(errs, fileError{File: repoPath, Kind: string(f.Kind), Errors: res.Errors})
				continue
			}
		}
		files = append(files, submit.File{Path: repoPath, Content: f.Content})
	}
	// Deterministic order so the branch hash and the commit are stable.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, errs
}

// collectFiles reads every uploaded file, expands any .zip, infers each file's
// kind, and validates it. It returns the parsed draft files and any per-file
// validation errors.
func (h *uploadHandler) collectFiles(uploaded []huma.FormFile, defaultKind preset.Kind) ([]upload.File, []fileError, error) {
	acc := &fileAccumulator{}
	for _, ff := range uploaded {
		content, err := io.ReadAll(io.LimitReader(ff, maxUploadBytes))
		_ = ff.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("could not read uploaded file %q: %w", ff.Filename, err)
		}

		if isZip(ff.Filename, content) {
			if err := h.filesFromZip(content, acc); err != nil {
				return nil, nil, err
			}
			continue
		}

		kind := kindForName(ff.Filename, defaultKind)
		file, ferr, ok := h.parseFile(ff.Filename, kind, content)
		if !ok {
			acc.addError(ferr)
			continue
		}
		if err := acc.add(file); err != nil {
			return nil, nil, err
		}
	}
	return acc.files, acc.fileErrs, nil
}

// fileAccumulator collects parsed files while enforcing the count and total-size
// caps on every addition. This is what bounds a zip: without it a single small
// archive could expand into an unbounded number of files, or gigabytes of
// decompressed content, before any limit was checked.
type fileAccumulator struct {
	files      []upload.File
	fileErrs   []fileError
	totalBytes int64
}

func (a *fileAccumulator) add(f upload.File) error {
	if len(a.files) >= maxDraftFiles {
		return fmt.Errorf("too many files in one upload (max %d)", maxDraftFiles)
	}
	a.totalBytes += int64(len(f.Content))
	if a.totalBytes > maxDraftBytes {
		return fmt.Errorf("uploaded presets exceed the %d MiB total limit", maxDraftBytes>>20)
	}
	a.files = append(a.files, f)
	return nil
}

func (a *fileAccumulator) addError(e fileError) { a.fileErrs = append(a.fileErrs, e) }

// filesFromZip expands a zip of presets into acc. Each entry's kind is inferred
// from its path within the archive (printers/…, filaments/…, processes/…),
// matching the repository layout. Entries whose kind cannot be inferred — a
// README, a vendor.yaml — are skipped silently. The count and aggregate-size
// caps are enforced per entry, so a zip bomb aborts mid-expansion rather than
// after fully decompressing.
func (h *uploadHandler) filesFromZip(content []byte, acc *fileAccumulator) error {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return fmt.Errorf("could not read the uploaded zip: %w", err)
	}
	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() || !isYAML(entry.Name) {
			continue
		}
		kind, ok := preset.KindFromPath(entry.Name)
		if !ok {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("could not read %q from the zip: %w", entry.Name, err)
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxUploadBytes))
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("could not read %q from the zip: %w", entry.Name, err)
		}
		file, ferr, valid := h.parseFile(entry.Name, kind, body)
		if !valid {
			acc.addError(ferr)
			continue
		}
		if err := acc.add(file); err != nil {
			return err
		}
	}
	return nil
}

// parseFile validates content of the given kind and, on success, extracts the
// identity fields needed to store and later commit it.
func (h *uploadHandler) parseFile(name string, kind preset.Kind, content []byte) (upload.File, fileError, bool) {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if !kind.Valid() {
		return upload.File{}, fileError{File: base,
			Message: "could not determine the preset type; name the file printers/<id>.yaml, filaments/<id>.yaml, or pass type=printer|filament"}, false
	}
	res := h.deps.validator.ValidateFile(kind, base, content)
	if !res.Valid() {
		return upload.File{}, fileError{File: base, Kind: string(kind), Errors: res.Errors}, false
	}
	id, _ := res.Sanitized["id"].(string)
	presetName, _ := res.Sanitized["name"].(string)
	vendor, _ := res.Sanitized["vendor"].(string)
	return upload.File{
		Kind:     kind,
		ID:       id,
		Name:     presetName,
		Vendor:   vendor,
		FileName: id + ".yaml",
		Content:  content,
	}, fileError{}, true
}

// --- helpers ---

// kindForName infers a bare file's kind from its path (printers/…, filaments/…),
// falling back to the form-wide default when the name carries no layout.
func kindForName(filename string, fallback preset.Kind) preset.Kind {
	if k, ok := preset.KindFromPath(filename); ok {
		return k
	}
	return fallback
}

// kindDir returns the repository sub-directory for a writable preset kind.
func kindDir(k preset.Kind) (string, bool) {
	switch k {
	case preset.KindPrinter:
		return "printers", true
	case preset.KindFilament:
		return "filaments", true
	default:
		return "", false
	}
}

func isYAML(name string) bool {
	l := strings.ToLower(name)
	return strings.HasSuffix(l, ".yaml") || strings.HasSuffix(l, ".yml")
}

// isZip reports whether a file is a zip, by extension or by the PK magic bytes,
// so a zip uploaded without a helpful name is still expanded.
func isZip(name string, content []byte) bool {
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		return true
	}
	return len(content) >= 4 && content[0] == 'P' && content[1] == 'K' &&
		(content[2] == 0x03 || content[2] == 0x05 || content[2] == 0x07)
}

// validationDetails turns per-file validation failures into Huma error details,
// so the response carries a structured "errors" array the admin app can render
// against the specific file and field.
func validationDetails(errs []fileError) []error {
	var out []error
	for _, fe := range errs {
		if len(fe.Errors) == 0 {
			out = append(out, &huma.ErrorDetail{Location: fe.File, Message: fe.Message})
			continue
		}
		for _, e := range fe.Errors {
			loc := fe.File
			if e.Path != "" {
				loc = fe.File + "#" + e.Path
			}
			out = append(out, &huma.ErrorDetail{Location: loc, Message: e.Message})
		}
	}
	return out
}

func draftFileViews(d *upload.Draft) []DraftFileView {
	views := make([]DraftFileView, len(d.Files))
	for i, f := range d.Files {
		views[i] = DraftFileView{
			Kind:     string(f.Kind),
			ID:       f.ID,
			Name:     f.Name,
			Vendor:   f.Vendor,
			FileName: f.FileName,
			Content:  string(f.Content),
		}
	}
	return views
}

func pluralPresets(n int) string {
	if n == 1 {
		return "1 preset"
	}
	return fmt.Sprintf("%d presets", n)
}

// branchName derives a deterministic branch from the vendor and the change
// content, so a retry re-drives the same branch instead of opening a duplicate.
func branchName(slug string, files []submit.File) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write(f.Content)
		h.Write([]byte{0})
	}
	return fmt.Sprintf("vendor-admin/%s/%s", slug, hex.EncodeToString(h.Sum(nil))[:12])
}

// prBody and commitMessage record provenance explicitly, because the commit is
// authored by the bot: the vendor slug, the Stytch organization, and a stable
// member id — never an email. See docs/vendor-workflow.md ("Change to Pull
// Request").
func prBody(slug string, claims auth.Claims, hasClaims bool, paths []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Proposed through the Cold Crabby Vendor Admin app for **%s**.\n\n", slug)
	b.WriteString("Files:\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- `%s`\n", p)
	}
	if hasClaims {
		b.WriteString("\n---\n")
		fmt.Fprintf(&b, "Vendor: %s\n", slug)
		fmt.Fprintf(&b, "Stytch-Organization: %s\n", claims.OrganizationID)
		fmt.Fprintf(&b, "Stytch-Member: %s\n", claims.Subject)
	}
	return b.String()
}

func commitMessage(slug string, claims auth.Claims, hasClaims bool, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: update %s via Vendor Admin\n", slug, pluralPresets(n))
	if hasClaims {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Vendor: %s\n", slug)
		fmt.Fprintf(&b, "Stytch-Organization: %s\n", claims.OrganizationID)
		fmt.Fprintf(&b, "Stytch-Member: %s\n", claims.Subject)
	}
	return b.String()
}

// upstreamError maps a GitHub-layer error to the right Huma status: a rate limit
// becomes 503 with Retry-After, and anything else becomes 502 — never a success
// the client would have to reconcile later.
func upstreamError(err error) error {
	var rle *ghapp.RateLimitedError
	if errors.As(err, &rle) {
		e := huma.Error503ServiceUnavailable(
			"GitHub is rate limiting submissions right now; please try again shortly")
		if d := rle.RetryAfter(time.Now()); d > 0 {
			secs := int(d.Seconds() + 0.999)
			return huma.ErrorWithHeaders(e, http.Header{"Retry-After": []string{fmt.Sprintf("%d", secs)}})
		}
		return e
	}
	return huma.Error502BadGateway("the submission could not be completed on GitHub: " + err.Error())
}
