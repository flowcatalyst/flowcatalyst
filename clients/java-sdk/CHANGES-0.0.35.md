# Java SDK 0.0.35 (unreleased)

Changes since `java-sdk/v0.0.34`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.35 when the owner decides.

Sources of truth: `api/openapi.lock.json`, `docs/field-worker-photo-spec.md`, `docs/wire-contract.md`
("Field identity", the admin endpoints).

## Field-worker photos

Additive. `fieldWorkers().setPhoto(id, bytes, contentType)` (the image as the request body),
`getPhoto(id, size, ifNoneMatch)` (returns `Photo(bytes, contentType, etag, notModified)`: bytes, the ETag
and whether the platform answered 304) and `removePhoto(id)`; `fieldPolicies().deletePhotos(clientId)`
(`DeleteClientPhotosResponse`); `FieldWorkerResponse.getPhoto()` (`FieldWorkerPhoto`);
`FieldPolicyResponse` and `PutFieldPolicyRequest` gain `photosEnabled` (default true) and
`workerMaySetOwnPhoto` (default false). A PUT replaces the policy, so a member left unset takes its
default.

The generated models come from the vendored spec as before; the transport reads every answer as bytes
now (it read text), keeps a request body of bytes as it is (`Transport.Bytes`) and returns an answer of
bytes (`Transport.Binary`, with a 304 as a success), which the photo calls need and no other call uses.

They need a platform that has the photo routes.
