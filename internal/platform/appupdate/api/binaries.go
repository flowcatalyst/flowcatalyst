package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// binaryReadTimeout bounds reading one uploaded binary (512 MiB at 2 Mbit/s
// is about 35 minutes; a pipeline uploads from a build machine, faster).
const binaryReadTimeout = 30 * time.Minute

// registerBinaries mounts the binary and install-link routes.
func registerBinaries(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "getAppBinaryFile", "/api/app-updates/binary-files/{applicationCode}/{sha256}",
		"What the platform read from an uploaded binary (404 when the application has not uploaded it)", s.getBinaryFile)
	huma.Register(api, huma.Operation{
		OperationID: "putAppBinaryFile",
		Method:      http.MethodPut,
		Path:        "/api/app-updates/binary-files/{applicationCode}/{sha256}",
		Summary:     "Upload one binary (an APK) by its SHA-256",
		Description: "The body is streamed to disk, its SHA-256 checked against the address, and the file parsed: it must be a well-formed APK " +
			"signed with APK Signature Scheme v2 or v3. What it says about itself (package, version, runtime version, signing certificate) is " +
			"recorded and is what a publish uses. Idempotent.",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/octet-stream": {Schema: &huma.Schema{Type: "string", Format: "binary"}},
			},
		},
	}, s.putBinaryFile)
	apiroute.Post(g, "publishAppBinary", "/api/app-updates/binaries", "Publish an uploaded binary to a channel in a variant", http.StatusCreated, s.publishBinary)
	apiroute.Get(g, "listAppBinaries", "/api/app-updates/binaries", "List an application's binaries, newest version first", s.listBinaries)
	apiroute.Get(g, "getAppBinary", "/api/app-updates/binaries/{id}", "Get one binary", s.getBinary)
	apiroute.Get(g, "checkAppBinaryForRuntimeVersion", "/api/app-updates/binary-check",
		"Whether a binary of a runtime version is published on a channel", s.checkBinary)

	apiroute.Get(g, "listAppInstallLinks", "/api/app-updates/install-links", "List an application's install links (never their tokens)", s.listLinks)
	apiroute.Post(g, "createAppInstallLink", "/api/app-updates/install-links", "Create the install link of a phone app's channel", http.StatusCreated, s.createLink)
	apiroute.Post(g, "replaceAppInstallLink", "/api/app-updates/install-links/{id}/replace",
		"Give an install link a new token; the old address stops working", http.StatusOK, s.replaceLink)
	apiroute.Delete(g, "removeAppInstallLink", "/api/app-updates/install-links/{id}", "Remove an install link; its address stops working",
		http.StatusNoContent, s.removeLink)
}

type binaryFileInput struct {
	ApplicationCode string `path:"applicationCode"`
	SHA256          string `path:"sha256" doc:"Lower-case hex SHA-256 of the file"`
}

func (s *State) getBinaryFile(ctx context.Context, in *binaryFileInput) (*apicommon.Out[AppBinaryFileResponse], error) {
	if err := auth.CanPublishAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if !appupdate.ValidSHA256(in.SHA256) {
		return nil, usecase.Validation("INVALID_SHA256", "the address must end in a lower-case hex SHA-256")
	}
	f, err := s.Binaries.FindFile(ctx, string(app.ID), in.SHA256)
	if err != nil {
		return nil, usecase.Internal("REPO", "find binary file failed", err)
	}
	if f == nil {
		return nil, usecase.NotFound("BINARY_FILE_NOT_FOUND", "the application has not uploaded this binary")
	}
	return &apicommon.Out[AppBinaryFileResponse]{Body: AppBinaryFileResponse{
		SHA256: f.SHA256, SizeBytes: f.SizeBytes, Facts: factsResponse(&f.BinaryFacts),
	}}, nil
}

// putBinaryFileInput reads no body itself: huma would buffer it whole. The
// handler streams it from the request (Resolve keeps the context).
type putBinaryFileInput struct {
	ApplicationCode string `path:"applicationCode"`
	SHA256          string `path:"sha256" doc:"Lower-case hex SHA-256 of the body"`
	hctx            huma.Context
}

// Resolve keeps the request context for the handler, which streams the body.
func (in *putBinaryFileInput) Resolve(ctx huma.Context) []error {
	in.hctx = ctx
	return nil
}

// putBinaryFile stores one binary without holding it in memory: the body is
// streamed to a spool file (bounded by MaxBinaryBytes) while hashed, checked
// against the address (400 DIGEST_MISMATCH), parsed as a signed APK (400 with
// the parser's code: NOT_AN_APK, UNSIGNED, SIGNATURE_INVALID, ...), then
// streamed into the store and registered with what was read from it. A file
// the application has already stored is not read again.
func (s *State) putBinaryFile(ctx context.Context, in *putBinaryFileInput) (*apicommon.Out[AppBinaryFileResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanPublishAppUpdates(ac); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if !appupdate.ValidSHA256(in.SHA256) {
		return nil, usecase.Validation("INVALID_SHA256", "the address must end in the body's lower-case hex SHA-256")
	}
	maxBytes := s.maxBinaryBytes()
	if cl := in.hctx.Header("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > maxBytes {
			return nil, tooLarge(maxBytes)
		}
	}
	existing, err := s.Binaries.FindFile(ctx, string(app.ID), in.SHA256)
	if err != nil {
		return nil, usecase.Internal("REPO", "find binary file failed", err)
	}
	if existing != nil {
		if ok, err := s.Store.Exists(ctx, in.SHA256); err == nil && ok {
			return &apicommon.Out[AppBinaryFileResponse]{Body: AppBinaryFileResponse{
				SHA256: existing.SHA256, SizeBytes: existing.SizeBytes, Facts: factsResponse(&existing.BinaryFacts),
			}}, nil
		}
	}

	_ = in.hctx.SetReadDeadline(time.Now().Add(binaryReadTimeout))
	spool, err := os.CreateTemp(s.SpoolDir, "fc-app-binary-*.apk")
	if err != nil {
		return nil, usecase.Internal("SPOOL", "create spool file failed", err)
	}
	defer func() {
		_ = spool.Close()
		if err := os.Remove(spool.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("app binaries: spool file not removed", "path", spool.Name(), "err", err)
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(spool, h), io.LimitReader(in.hctx.BodyReader(), maxBytes+1))
	_ = in.hctx.SetReadDeadline(time.Time{})
	switch {
	case err != nil:
		return nil, usecase.Validation("UPLOAD_INCOMPLETE", "the upload ended before the whole file was received; send it again")
	case n > maxBytes:
		return nil, tooLarge(maxBytes)
	case n == 0:
		return nil, usecase.Validation("EMPTY_FILE", "the body is empty")
	case hex.EncodeToString(h.Sum(nil)) != in.SHA256:
		return nil, usecase.Validation("DIGEST_MISMATCH", "the body's SHA-256 does not match the address")
	}

	info, err := apk.Parse(spool, n)
	if err != nil {
		if ae, ok := errors.AsType[*apk.Error](err); ok {
			return nil, usecase.Validation(ae.Code, ae.Message)
		}
		return nil, usecase.Internal("APK_PARSE", "reading the APK failed", err)
	}
	facts, err := factsFromAPK(info)
	if err != nil {
		return nil, err
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return nil, usecase.Internal("SPOOL", "rewind spool file failed", err)
	}
	if err := s.Store.Put(ctx, in.SHA256, spool); err != nil {
		if errors.Is(err, artifact.ErrDigestMismatch) {
			return nil, usecase.Validation("DIGEST_MISMATCH", "the body's SHA-256 does not match the address")
		}
		return nil, usecase.Internal("UPDATE_STORE", "store put failed", err)
	}
	f := appupdate.BinaryFile{SHA256: in.SHA256, SizeBytes: n, BinaryFacts: facts}
	created, err := s.Binaries.RecordFile(ctx, string(app.ID), f, ac.PrincipalID)
	if err != nil {
		return nil, usecase.Internal("REPO", "record binary file failed", err)
	}
	return &apicommon.Out[AppBinaryFileResponse]{Body: AppBinaryFileResponse{
		SHA256: f.SHA256, SizeBytes: f.SizeBytes, Created: created, Facts: factsResponse(&f.BinaryFacts),
	}}, nil
}

func tooLarge(maxBytes int64) error {
	return usecase.Validation("FILE_TOO_LARGE", fmt.Sprintf("a binary may be at most %d bytes (FC_APP_UPDATES_MAX_BINARY_BYTES)", maxBytes))
}

// factsFromAPK is what the platform records of a parsed APK. A runtime
// version the update routes would refuse (they validate the header a phone
// sends) is refused here, so a binary is never recorded as carrying one no
// update could reach.
func factsFromAPK(info *apk.Info) (appupdate.BinaryFacts, error) {
	f := appupdate.BinaryFacts{
		Platform: appupdate.PlatformAndroid, PackageName: info.Package, VersionCode: info.VersionCode,
		MinSDK: info.MinSDK, Debuggable: info.Debuggable,
		SigningCertSHA256: info.Signing.CertificateSHA256, SigningCertSubject: info.Signing.CertificateSubject,
		DebugCertificate: info.Signing.DebugCertificate, SigningSchemes: info.Signing.Schemes,
	}
	opt := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	f.VersionName, f.UpdateURL, f.UpdateKeyID = opt(info.VersionName), opt(info.UpdateURL), opt(info.UpdateCodeSigningKeyID)
	f.UpdateChannel = opt(info.Channel())
	if info.RuntimeVersion != "" {
		if !appupdate.ValidRuntimeVersion(info.RuntimeVersion) {
			return f, usecase.Validation("INVALID_RUNTIME_VERSION",
				fmt.Sprintf("the APK's runtime version %q is not one the update routes accept (1-128 letters, digits and . _ : + -)", info.RuntimeVersion))
		}
		f.RuntimeVersion = opt(info.RuntimeVersion)
	}
	if len(f.PackageName) > 255 || (f.VersionName != nil && len(*f.VersionName) > 255) {
		return f, usecase.Validation("MALFORMED_MANIFEST", "the APK's package or version name is longer than 255 characters")
	}
	if f.UpdateChannel != nil && len(*f.UpdateChannel) > 255 {
		f.UpdateChannel = nil
	}
	if f.UpdateKeyID != nil && len(*f.UpdateKeyID) > 255 {
		f.UpdateKeyID = nil
	}
	return f, nil
}

func (s *State) publishBinary(ctx context.Context, in *apicommon.In[PublishBinaryRequest]) (*publishBinaryOutput, error) {
	if err := auth.CanPublishAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	b := in.Body
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.PublishBinary(s.Binaries, s.Apps), operations.PublishBinaryCommand{
		ApplicationCode: b.ApplicationCode, AppKey: b.AppKey, Platform: b.Platform, Channel: b.Channel,
		Variant: b.Variant, SHA256: b.SHA256, Message: b.Message, SourceCommit: b.SourceCommit,
	}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	return &publishBinaryOutput{Status: status, Body: PublishBinaryResponse{Binary: binaryResponse(res.Binary, false), Created: res.Created}}, nil
}

type publishBinaryOutput struct {
	Status int
	Body   PublishBinaryResponse
}

type listBinariesInput struct {
	ApplicationCode string `query:"applicationCode" required:"true"`
	AppKey          string `query:"appKey"`
	Platform        string `query:"platform"`
	Channel         string `query:"channel"`
	Variant         string `query:"variant"`
	RuntimeVersion  string `query:"runtimeVersion"`
	apicommon.PageQuery
}

func (s *State) listBinaries(ctx context.Context, in *listBinariesInput) (*apicommon.Out[apicommon.OffsetPage[AppBinaryResponse]], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(in.AppKey, in.Platform, in.RuntimeVersion, in.Channel); err != nil {
		return nil, err
	}
	if in.Variant != "" && !appupdate.ValidVariant(in.Variant) {
		return nil, usecase.Validation("INVALID_VARIANT", "variant is not a variant name")
	}
	rows, total, newest, err := s.Binaries.ListBinaries(ctx, appupdate.BinaryFilter{
		ApplicationID: string(app.ID), AppKey: in.AppKey, Platform: in.Platform, Channel: in.Channel,
		Variant: in.Variant, RuntimeVersion: in.RuntimeVersion, Limit: in.LimitVal(), Offset: in.OffsetVal(),
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "list binaries failed", err)
	}
	out := make([]AppBinaryResponse, 0, len(rows))
	for i := range rows {
		out = append(out, binaryResponse(&rows[i], newest[rows[i].ID]))
	}
	return &apicommon.Out[apicommon.OffsetPage[AppBinaryResponse]]{Body: apicommon.NewOffsetPage(out, in.PageIndex(), in.PageSizeVal(), total)}, nil
}

func (s *State) getBinary(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[AppBinaryResponse], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	b, err := operations.ReachBinary(ctx, s.Binaries, in.ID)
	if err != nil {
		return nil, err
	}
	rows, _, newest, err := s.Binaries.ListBinaries(ctx, appupdate.BinaryFilter{
		ApplicationID: b.ApplicationID, AppKey: b.AppKey, Platform: b.Platform, Channel: b.Channel, Variant: b.Variant, Limit: 1,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "read newest binary failed", err)
	}
	return &apicommon.Out[AppBinaryResponse]{Body: binaryResponse(b, len(rows) == 1 && newest[b.ID])}, nil
}

type checkBinaryInput struct {
	ApplicationCode string `query:"applicationCode" required:"true"`
	AppKey          string `query:"appKey" required:"true"`
	Platform        string `query:"platform" required:"true" enum:"android,ios"`
	Channel         string `query:"channel" required:"true"`
	RuntimeVersion  string `query:"runtimeVersion" required:"true"`
}

// checkBinary answers an update pipeline: is a binary of this runtime version
// published on this channel (any variant)? 200 either way; the answer is in
// the body.
func (s *State) checkBinary(ctx context.Context, in *checkBinaryInput) (*apicommon.Out[AppBinaryCheckResponse], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if err := operations.ValidateBranch(in.ApplicationCode, in.AppKey, in.Platform, in.RuntimeVersion, in.Channel); err != nil {
		return nil, err
	}
	rows, total, newest, err := s.Binaries.ListBinaries(ctx, appupdate.BinaryFilter{
		ApplicationID: string(app.ID), AppKey: in.AppKey, Platform: in.Platform, Channel: in.Channel,
		RuntimeVersion: in.RuntimeVersion, Limit: 1,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "check binaries failed", err)
	}
	out := AppBinaryCheckResponse{Published: total > 0, Count: total}
	if len(rows) > 0 {
		r := binaryResponse(&rows[0], newest[rows[0].ID])
		out.Newest = &r
	}
	return &apicommon.Out[AppBinaryCheckResponse]{Body: out}, nil
}

type listLinksInput struct {
	ApplicationCode string `query:"applicationCode" required:"true"`
	AppKey          string `query:"appKey"`
}

func (s *State) listLinks(ctx context.Context, in *listLinksInput) (*apicommon.Out[AppInstallLinksResponse], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(in.AppKey, "", "", ""); err != nil {
		return nil, err
	}
	links, err := s.Links.ListLinks(ctx, string(app.ID), in.AppKey)
	if err != nil {
		return nil, usecase.Internal("REPO", "list install links failed", err)
	}
	out := make([]AppInstallLinkResponse, 0, len(links))
	for i := range links {
		out = append(out, linkResponse(&links[i]))
	}
	return &apicommon.Out[AppInstallLinksResponse]{Body: AppInstallLinksResponse{Links: out}}, nil
}

func (s *State) createLink(ctx context.Context, in *apicommon.In[CreateInstallLinkRequest]) (*apicommon.Out[InstallLinkWithTokenResponse], error) {
	if err := auth.CanManageAppInstallLinks(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.CreateInstallLink(s.Links, s.Apps), operations.CreateInstallLinkCommand{
		ApplicationCode: in.Body.ApplicationCode, AppKey: in.Body.AppKey, Channel: in.Body.Channel,
	}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[InstallLinkWithTokenResponse]{Body: s.withToken(res)}, nil
}

func (s *State) replaceLink(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[InstallLinkWithTokenResponse], error) {
	if err := auth.CanManageAppInstallLinks(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.ReplaceInstallLink(s.Links),
		operations.ReplaceInstallLinkCommand{LinkID: in.ID}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[InstallLinkWithTokenResponse]{Body: s.withToken(res)}, nil
}

func (s *State) removeLink(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanManageAppInstallLinks(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, operations.RemoveInstallLink(s.Links),
		operations.RemoveInstallLinkCommand{LinkID: in.ID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) withToken(res operations.InstallLinkResult) InstallLinkWithTokenResponse {
	l := res.Link
	return InstallLinkWithTokenResponse{
		Link: linkResponse(l), Token: res.Token,
		URL: appupdate.InstallURL(s.InstallBaseURL, l.ApplicationCode, l.AppKey, res.Token),
	}
}
