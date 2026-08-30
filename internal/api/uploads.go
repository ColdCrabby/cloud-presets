package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/auth"
	ghapp "github.com/ColdCrabby/cloud-presets/internal/github"
	"github.com/ColdCrabby/cloud-presets/internal/preset"
	"github.com/ColdCrabby/cloud-presets/internal/submit"
	"github.com/ColdCrabby/cloud-presets/internal/upload"
)

// Upload limits. Drafts are held in memory and claimable by an unguessable id,
// so the request body, the number of files, and the total decompressed size are
// all bounded to keep an abandoned or hostile upload — including a zip bomb —
// from consuming the process. These endpoints are unauthenticated, so the caps
// are the only backstop.
const (
	maxUploadBytes = 16 << 20 // 16 MiB per request (compressed, over the wire)
	maxDraftBytes  = 64 << 20 // 64 MiB total decompressed across all files
	maxDraftFiles  = 500      // files per upload, including zip entries
	claimPathBase  = "/vendor/claim/"
)

// uploadDeps are the collaborators the upload/claim handlers need. They are
// assembled by api.New from options so tests can supply fakes and the OpenAPI
// export can omit them.
type uploadDeps struct {
	validator *preset.Validator
	store     *upload.Store
	submitter submit.Submitter // nil when the GitHub bot is not configured
}

// registerUploads wires the manual upload and claim endpoints onto mux. These
// are raw handlers rather than Huma operations because they deal in multipart
// bodies and a redirect — shapes outside Huma's typed model — and mirror how
// GET /v1/me is mounted. They therefore do not appear in the OpenAPI document;
// the vendor app calls them with fetch, as it already does for /v1/me.
//
// The claim endpoint is gated by the auth middleware when one is configured, so
// only a signed-in vendor can turn a parked draft into a pull request. Upload
// and read are intentionally open: like the slicer hand-off, the id authorizes
// nothing on its own, and authority is checked at claim time.
func registerUploads(mux *http.ServeMux, mw *auth.Middleware, deps uploadDeps) {
	if deps.store == nil {
		return
	}
	h := &uploadHandler{deps: deps}

	mux.HandleFunc("POST "+BasePath+"/uploads", h.create)
	mux.HandleFunc("GET "+BasePath+"/uploads/{id}", h.get)

	claim := http.HandlerFunc(h.claim)
	if mw != nil {
		mux.Handle("POST "+BasePath+"/uploads/{id}/claim", mw.RequireAuth(claim))
	} else {
		mux.Handle("POST "+BasePath+"/uploads/{id}/claim", claim)
	}
}

type uploadHandler struct {
	deps uploadDeps
}

// fileError is a per-file validation failure returned to the client so the
// admin app can point at the offending upload and field.
type fileError struct {
	File    string         `json:"file"`
	Kind    string         `json:"kind,omitempty"`
	Message string         `json:"message,omitempty"`
	Errors  []preset.Error `json:"errors,omitempty"`
}

// draftFileView is a file as reported back to the client. Content is included so
// the admin app can preview the exact YAML before claiming.
type draftFileView struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Vendor   string `json:"vendor,omitempty"`
	FileName string `json:"fileName"`
	Content  string `json:"content"`
}

// create handles POST /v1/uploads. It accepts a multipart form carrying either
// a .zip of presets laid out like the repository (printers/…, filaments/…) or
// individual preset files, validates each against the pinned schemas, parks the
// result as a draft, and either redirects to the admin claim page or returns the
// draft as JSON.
func (h *uploadHandler) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeProblem(w, http.StatusBadRequest, "could not read the multipart upload: "+err.Error())
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	files, fileErrs, err := h.collectFiles(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(fileErrs) > 0 {
		writeProblemWithErrors(w, http.StatusUnprocessableEntity,
			"one or more uploaded presets are invalid", fileErrs)
		return
	}
	if len(files) == 0 {
		writeProblem(w, http.StatusBadRequest,
			"no preset files were found in the upload; send preset YAML files or a .zip laid out like the presets repository")
		return
	}

	draft, err := h.deps.store.Create(files)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "could not store the upload: "+err.Error())
		return
	}

	claimURL := claimPathBase + draft.ID
	// A browser form (or an explicit ?redirect=1) wants to land on the claim
	// page; a programmatic client wants the id and the URL as JSON.
	if wantsRedirect(r) {
		http.Redirect(w, r, claimURL, http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        draft.ID,
		"claimUrl":  claimURL,
		"expiresAt": draft.ExpiresAt,
		"files":     draftFileViews(draft),
	})
}

// get handles GET /v1/uploads/{id}. It returns the parked draft so the admin app
// can render it for review. The id is the capability; no ownership is checked
// here, only at claim time.
func (h *uploadHandler) get(w http.ResponseWriter, r *http.Request) {
	draft, err := h.deps.store.Get(r.PathValue("id"))
	if err != nil {
		writeProblem(w, http.StatusNotFound, "this upload was not found or has expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        draft.ID,
		"createdAt": draft.CreatedAt,
		"expiresAt": draft.ExpiresAt,
		"files":     draftFileViews(draft),
	})
}

// claimRequest is the optional JSON body of a claim. In dev (no GitHub bot) the
// caller may name the vendor slug directly; with the bot configured the slug is
// resolved from the manifest and this is ignored.
type claimRequest struct {
	Vendor string `json:"vendor"`
}

// claim handles POST /v1/uploads/{id}/claim. It resolves the caller's writable
// vendor namespace, re-validates the draft against the pinned schemas, and — when
// the GitHub bot is configured — opens a pull request. In dev it returns the
// resolved change set without opening a PR, so the flow is exercisable end to end
// without credentials.
func (h *uploadHandler) claim(w http.ResponseWriter, r *http.Request) {
	draft, err := h.deps.store.Get(r.PathValue("id"))
	if err != nil {
		writeProblem(w, http.StatusNotFound, "this upload was not found or has expired")
		return
	}

	var body claimRequest
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body)
	}
	claims, hasClaims := auth.ClaimsFromContext(r.Context())

	// A process preset lives in the shared, project-owned namespace and is not
	// writable through the API by any vendor. Reject before touching GitHub.
	for _, f := range draft.Files {
		if f.Kind == preset.KindProcess {
			writeProblem(w, http.StatusForbidden,
				"process presets belong to the shared processes/ namespace and cannot be submitted through Vendor Admin")
			return
		}
	}

	slug, err := h.resolveVendor(r.Context(), claims, body.Vendor)
	if err != nil {
		if errors.Is(err, submit.ErrVendorNotFound) {
			writeProblem(w, http.StatusForbidden,
				"your organization does not own a vendor namespace; ask a maintainer to add your Stytch organization to a vendor.yaml")
			return
		}
		writeUpstreamProblem(w, err)
		return
	}

	files, fileErrs := h.buildRepoFiles(slug, draft)
	if len(fileErrs) > 0 {
		writeProblemWithErrors(w, http.StatusUnprocessableEntity,
			"the uploaded presets no longer validate for this vendor", fileErrs)
		return
	}

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}

	// Without the bot we cannot open a pull request. Return the resolved,
	// validated change set so the flow still completes visibly in dev.
	if h.deps.submitter == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"claimed":   true,
			"prCreated": false,
			"vendor":    slug,
			"files":     paths,
			"message":   "The GitHub App bot is not configured, so no pull request was opened. Configure it to enable submissions.",
		})
		return
	}

	req := submit.Request{
		Branch:        branchName(slug, files),
		Title:         fmt.Sprintf("%s: update %s via Vendor Admin", slug, pluralPresets(len(files))),
		Body:          prBody(slug, claims, hasClaims, paths),
		CommitMessage: commitMessage(slug, claims, hasClaims, len(files)),
		Files:         files,
	}
	res, err := h.deps.submitter.Submit(r.Context(), req)
	if err != nil {
		writeUpstreamProblem(w, err)
		return
	}

	// The pull request is now the record; the draft has served its purpose.
	h.deps.store.Delete(draft.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"claimed":        true,
		"prCreated":      true,
		"vendor":         slug,
		"files":          paths,
		"pullRequestUrl": res.URL,
		"branch":         res.Branch,
		"alreadyExisted": res.AlreadyExisted,
	})
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

// collectFiles reads every uploaded part, expands any .zip, infers each file's
// kind, and validates it. It returns the parsed draft files and any per-file
// validation errors.
func (h *uploadHandler) collectFiles(r *http.Request) ([]upload.File, []fileError, error) {
	if r.MultipartForm == nil {
		return nil, nil, errors.New("the request was not a multipart upload")
	}
	acc := &fileAccumulator{}

	// Optional default kind for bare files whose name does not encode one.
	defaultKind := preset.Kind(strings.TrimSpace(r.FormValue("type")))

	for field, headers := range r.MultipartForm.File {
		for _, fh := range headers {
			opened, err := fh.Open()
			if err != nil {
				return nil, nil, fmt.Errorf("could not read uploaded file %q: %w", fh.Filename, err)
			}
			content, err := io.ReadAll(io.LimitReader(opened, maxUploadBytes))
			_ = opened.Close()
			if err != nil {
				return nil, nil, fmt.Errorf("could not read uploaded file %q: %w", fh.Filename, err)
			}

			if isZip(fh.Filename, content) {
				if err := h.filesFromZip(content, acc); err != nil {
					return nil, nil, err
				}
				continue
			}

			kind := kindFor(field, fh.Filename, defaultKind)
			file, ferr, ok := h.parseFile(fh.Filename, kind, content)
			if !ok {
				acc.addError(ferr)
				continue
			}
			if err := acc.add(file); err != nil {
				return nil, nil, err
			}
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
			Message: "could not determine the preset type; name the file printers/<id>.yaml, filaments/<id>.yaml, or pass type=printer|filament|process"}, false
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

// kindFor infers a preset kind for a bare (non-zip) part: an explicit form field
// name wins, then the file path, then the form-wide default.
func kindFor(field, filename string, fallback preset.Kind) preset.Kind {
	if k := preset.Kind(strings.TrimSpace(field)); k.Valid() {
		return k
	}
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

// isZip reports whether a part is a zip, by extension or by the PK magic bytes,
// so a zip uploaded without a helpful name is still expanded.
func isZip(name string, content []byte) bool {
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		return true
	}
	return len(content) >= 4 && content[0] == 'P' && content[1] == 'K' &&
		(content[2] == 0x03 || content[2] == 0x05 || content[2] == 0x07)
}

// wantsRedirect reports whether the client should be redirected to the claim
// page rather than handed JSON: an explicit redirect flag, or a browser form
// post that prefers HTML.
func wantsRedirect(r *http.Request) bool {
	if v := strings.TrimSpace(r.FormValue("redirect")); v != "" && v != "0" && v != "false" {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json")
}

func draftFileViews(d *upload.Draft) []draftFileView {
	views := make([]draftFileView, len(d.Files))
	for i, f := range d.Files {
		views[i] = draftFileView{
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

// writeJSON writes a JSON body with the given status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeProblemWithErrors renders an RFC 9457 problem document carrying per-file
// validation errors under an "errors" member.
func writeProblemWithErrors(w http.ResponseWriter, status int, detail string, errs []fileError) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"title":  http.StatusText(status),
		"detail": detail,
		"errors": errs,
	})
}

// writeUpstreamProblem maps a GitHub-layer error to the right status: a rate
// limit becomes 503 with Retry-After, a permission or not-found problem becomes
// 502, and anything else becomes 502 as well — never a success the client would
// have to reconcile later.
func writeUpstreamProblem(w http.ResponseWriter, err error) {
	var rle *ghapp.RateLimitedError
	if errors.As(err, &rle) {
		if d := rle.RetryAfter(time.Now()); d > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(d.Seconds()+0.999)))
		}
		writeProblem(w, http.StatusServiceUnavailable,
			"GitHub is rate limiting submissions right now; please try again shortly")
		return
	}
	var perr *ghapp.PermissionError
	var nferr *ghapp.NotFoundError
	if errors.As(err, &perr) || errors.As(err, &nferr) {
		writeProblem(w, http.StatusBadGateway,
			"the submission could not be completed on GitHub: "+err.Error())
		return
	}
	writeProblem(w, http.StatusBadGateway, "the submission could not be completed on GitHub: "+err.Error())
}
