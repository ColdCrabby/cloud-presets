package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ColdCrabby/cloud-presets/internal/auth"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/ColdCrabby/cloud-presets/internal/preset"
	"github.com/ColdCrabby/cloud-presets/internal/submit"
	"github.com/ColdCrabby/cloud-presets/internal/upload"
)

const validPrinterYAML = `schema_version: 1
id: prusa-mk4-0.4
name: Prusa MK4 — 0.4 mm nozzle
vendor: Prusa
model: MK4
bed_shape: rectangular
bed_width: 250
bed_depth: 210
bed_height: 220
origin_at_center: false
params:
  nozzle_diameter_mm: 0.4
  filament_diameter_mm: 1.75
  extruder_count: 1
  gcode_flavor: marlin
`

// fakeSubmitter records the request it received and returns a canned result.
type fakeSubmitter struct {
	slug    string
	slugErr error
	got     submit.Request
	result  submit.Result
	err     error
}

func (f *fakeSubmitter) ResolveVendorSlug(_ context.Context, _ string) (string, error) {
	return f.slug, f.slugErr
}

func (f *fakeSubmitter) Submit(_ context.Context, req submit.Request) (submit.Result, error) {
	f.got = req
	return f.result, f.err
}

func newUploadServer(t *testing.T, sub submit.Submitter) (*httptest.Server, *upload.Store) {
	t.Helper()
	v, err := preset.New()
	if err != nil {
		t.Fatalf("preset.New: %v", err)
	}
	store := upload.NewStore(0)
	_, handler := New(catalog.NewHolder(), nil, WithUploads(v, store, sub))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, store
}

// uploadForm builds a multipart body with every preset under the single "files"
// field the operation expects. Each element is a (fileName, content) pair; the
// file name matters because the validator enforces file-name-equals-id.
func multipartFiles(t *testing.T, files ...[2]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, fc := range files {
		w, err := mw.CreateFormFile("files", fc[0])
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(fc[1]))
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestUploadStoresDraftAndReturnsClaimURL(t *testing.T) {
	srv, store := newUploadServer(t, nil)
	body, ct := multipartFiles(t, [2]string{"prusa-mk4-0.4.yaml", validPrinterYAML})

	resp, err := http.Post(srv.URL+"/v1/uploads?type=printer", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", resp.StatusCode, readBody(resp))
	}
	var out struct {
		ID       string `json:"id"`
		ClaimURL string `json:"claimUrl"`
		Files    []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" || out.ClaimURL != "/vendor/claim/"+out.ID {
		t.Fatalf("unexpected claim url %q for id %q", out.ClaimURL, out.ID)
	}
	if len(out.Files) != 1 || out.Files[0].ID != "prusa-mk4-0.4" || out.Files[0].Kind != "printer" {
		t.Fatalf("unexpected files: %+v", out.Files)
	}
	if store.Len() != 1 {
		t.Fatalf("store holds %d drafts, want 1", store.Len())
	}
}

func TestUploadRejectsInvalidPreset(t *testing.T) {
	srv, _ := newUploadServer(t, nil)
	body, ct := multipartFiles(t, [2]string{"NOPE.yaml", "schema_version: 1\nid: NOPE\n"})

	resp, err := http.Post(srv.URL+"/v1/uploads?type=printer", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestUploadInfersKindFromZipLayout(t *testing.T) {
	srv, store := newUploadServer(t, nil)

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("printers/prusa-mk4-0.4.yaml")
	_, _ = w.Write([]byte(validPrinterYAML))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	body, ct := zipUpload(t, "presets.zip", zbuf.Bytes())
	resp, err := http.Post(srv.URL+"/v1/uploads", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", resp.StatusCode, readBody(resp))
	}
	if store.Len() != 1 {
		t.Fatalf("store holds %d drafts, want 1", store.Len())
	}
}

func TestClaimOpensPullRequest(t *testing.T) {
	sub := &fakeSubmitter{slug: "prusa", result: submit.Result{URL: "https://github.com/x/y/pull/1", Branch: "b"}}
	srv, store := newUploadServer(t, sub)

	d, err := store.Create([]upload.File{{
		Kind: preset.KindPrinter, ID: "prusa-mk4-0.4", Name: "Prusa MK4",
		Vendor: "Prusa", FileName: "prusa-mk4-0.4.yaml", Content: []byte(validPrinterYAML),
	}})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(srv.URL+"/v1/uploads/"+d.ID+"/claim", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, readBody(resp))
	}
	var out struct {
		PRCreated      bool   `json:"prCreated"`
		Vendor         string `json:"vendor"`
		PullRequestURL string `json:"pullRequestUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.PRCreated || out.Vendor != "prusa" || out.PullRequestURL == "" {
		t.Fatalf("unexpected claim response: %+v", out)
	}
	if got := sub.got.Files[0].Path; got != "vendors/prusa/printers/prusa-mk4-0.4.yaml" {
		t.Fatalf("committed path = %q", got)
	}
	if store.Len() != 0 {
		t.Fatalf("draft should be consumed after a successful claim, store holds %d", store.Len())
	}
}

func TestClaimWithoutSubmitterFallsBack(t *testing.T) {
	srv, store := newUploadServer(t, nil)
	d, _ := store.Create([]upload.File{{
		Kind: preset.KindPrinter, ID: "prusa-mk4-0.4", Name: "Prusa MK4",
		Vendor: "Prusa", FileName: "prusa-mk4-0.4.yaml", Content: []byte(validPrinterYAML),
	}})

	body := strings.NewReader(`{"vendor":"prusa"}`)
	resp, err := http.Post(srv.URL+"/v1/uploads/"+d.ID+"/claim", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, readBody(resp))
	}
	var out struct {
		PRCreated bool   `json:"prCreated"`
		Vendor    string `json:"vendor"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.PRCreated || out.Vendor != "prusa" {
		t.Fatalf("unexpected dev-fallback response: %+v", out)
	}
}

func TestClaimRejectsProcessPreset(t *testing.T) {
	sub := &fakeSubmitter{slug: "prusa"}
	srv, store := newUploadServer(t, sub)
	d, _ := store.Create([]upload.File{{
		Kind: preset.KindProcess, ID: "coldcrabby-standard-0.20", Name: "Standard",
		FileName: "coldcrabby-standard-0.20.yaml", Content: []byte("schema_version: 1\n"),
	}})

	resp, err := http.Post(srv.URL+"/v1/uploads/"+d.ID+"/claim", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestUploadZipEnforcesFileCap(t *testing.T) {
	srv, store := newUploadServer(t, nil)

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for i := 0; i < maxDraftFiles+5; i++ {
		id := fmt.Sprintf("acme-printer-%d", i)
		w, err := zw.Create("printers/" + id + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(w, "schema_version: 1\nid: %s\nname: Acme %d\nvendor: Acme\nmodel: X\n", id, i)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	body, ct := zipUpload(t, "presets.zip", zbuf.Bytes())
	resp, err := http.Post(srv.URL+"/v1/uploads", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (file cap)", resp.StatusCode)
	}
	if store.Len() != 0 {
		t.Fatalf("an over-cap upload must not park a draft, store holds %d", store.Len())
	}
}

func TestClaimUnknownDraftIs404(t *testing.T) {
	srv, _ := newUploadServer(t, &fakeSubmitter{slug: "prusa"})
	resp, err := http.Post(srv.URL+"/v1/uploads/deadbeef/claim", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestClaimRequiresAuthWhenMiddlewarePresent exercises the Stytch→Huma auth
// adapter: with a middleware configured, a claim carrying no (or an invalid)
// session token is rejected with 401 before any draft work happens.
func TestClaimRequiresAuthWhenMiddlewarePresent(t *testing.T) {
	// An empty JWKS is a valid key set with no keys, so the verifier constructs
	// offline and rejects every token — enough to prove the gate fires.
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(jwks.Close)

	verifier, err := auth.New(context.Background(), auth.Config{
		ProjectID: "project-test",
		JWKSURL:   jwks.URL,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	mw := auth.NewMiddleware(verifier)

	v, err := preset.New()
	if err != nil {
		t.Fatalf("preset.New: %v", err)
	}
	store := upload.NewStore(0)
	_, handler := New(catalog.NewHolder(), mw, WithUploads(v, store, &fakeSubmitter{slug: "prusa"}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name  string
		token string
	}{
		{name: "no token", token: ""},
		{name: "garbage token", token: "not-a-jwt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/uploads/whatever/claim", strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if wa := resp.Header.Get("WWW-Authenticate"); !strings.Contains(wa, "invalid_token") {
				t.Errorf("WWW-Authenticate = %q, want it to mention invalid_token", wa)
			}
		})
	}
}

// zipUpload wraps zip bytes in a multipart body under the "files" field.
func zipUpload(t *testing.T, name string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("files", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	return &body, mw.FormDataContentType()
}

func readBody(resp *http.Response) string {
	var b bytes.Buffer
	_, _ = b.ReadFrom(resp.Body)
	return b.String()
}
