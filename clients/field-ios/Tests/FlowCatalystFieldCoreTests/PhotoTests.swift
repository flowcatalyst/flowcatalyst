import XCTest
@testable import FlowCatalystFieldCore

/// The worker's own photo: the session's worker fields, getPhoto, setPhoto, removePhoto and the platform's refusals.
final class PhotoTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }
    let jpeg = Data([0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0xFF, 0xD9])
    let version = "0123456789abcdef"

    override func setUp() async throws { h = Harness() }
    override func tearDown() async throws { h.close() }

    private func signedIn(photoVersion: String? = nil, canSet: Bool = false) async throws -> FieldSession {
        let claims: JObject = ["device_mode": "PERSONAL"]
        p.on("/field/auth/login") { _ in self.p.token(claims: claims, photoVersion: photoVersion, canSetOwnPhoto: canSet) }
        p.on("/field/auth/refresh") { _ in self.p.token(claims: claims, photoVersion: photoVersion, canSetOwnPhoto: canSet) }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        return s
    }

    private func worker(_ s: FieldSession) async throws -> Worker {
        guard case .signedIn(let session) = try await s.restore() else { throw Unreachable() }
        return session.worker
    }

    private func image(_ etag: String) -> FakeResponse {
        var r = FakeResponse(status: 200).header("Content-Type", "image/jpeg").header("ETag", etag)
        r.bytes = jpeg
        return r
    }

    func testWorkerCarriesPhotoFieldsAcrossARestartAndOlderSessionsGetNone() async throws {
        let s = try await signedIn(photoVersion: version, canSet: true)
        let w = try await worker(s)
        XCTAssertEqual(version, w.photoVersion)
        XCTAssertTrue(w.canSetOwnPhoto)
        let again = try await worker(h.session())
        XCTAssertEqual(version, again.photoVersion)
        XCTAssertTrue(again.canSetOwnPhoto)
        var old = try XCTUnwrap(JSON.parseObject(Data(try XCTUnwrap(h.storage.get(FieldSession.storeSession)).utf8)))
        var ow = try XCTUnwrap(old.optObject("worker"))
        ow.removeValue(forKey: "photoVersion")
        ow.removeValue(forKey: "canSetOwnPhoto")
        old["worker"] = ow
        try h.storage.put(FieldSession.storeSession, value: JSON.string(old))
        let o = try await worker(h.session())
        XCTAssertNil(o.photoVersion)
        XCTAssertFalse(o.canSetOwnPhoto)
    }

    func testAChangedPhotoShowsAtTheNextRefreshAsSessionChanged() async throws {
        let s = try await signedIn(photoVersion: version)
        h.events.clear()
        p.on("/field/auth/refresh") { _ in self.p.token(claims: ["device_mode": "PERSONAL"], photoVersion: "fedcba9876543210") }
        let after = try await s.refresh()
        XCTAssertEqual("fedcba9876543210", after.worker.photoVersion)
        XCTAssertTrue(h.events.all.contains { if case .sessionChanged(let x) = $0 { return x?.worker.photoVersion == "fedcba9876543210" } else { return false } })
    }

    func testGetPhotoReadsTheBytesSignedLikeOtherCalls() async throws {
        let s = try await signedIn(photoVersion: version)
        p.on("/field/worker/photo") { _ in self.image("\"\(self.version)-thumb\"") }
        let got = try await s.getPhoto()
        let photo = try XCTUnwrap(got)
        XCTAssertEqual(jpeg, photo.bytes)
        XCTAssertEqual("image/jpeg", photo.contentType)
        XCTAssertEqual(version, photo.version)
        XCTAssertFalse(photo.notModified)
        let rec = try XCTUnwrap(p.all("/field/worker/photo").first)
        XCTAssertEqual("thumb", rec.body?.optString("size"))
        XCTAssertFalse(rec.body?.has("version") ?? true)
        XCTAssertNotNil(rec.bearer)
        XCTAssertEqual(p.url + "/field/worker/photo", rec.proof?.claims.optString("htu"))
        _ = try await s.getPhoto(size: .full)
        XCTAssertEqual("full", p.all("/field/worker/photo").last?.body?.optString("size"))
    }

    func testGetPhotoNotModifiedNoneAndRefusals() async throws {
        let s = try await signedIn(photoVersion: version)
        p.on("/field/worker/photo") { _ in FakeResponse(status: 304).header("ETag", "\"\(self.version)-thumb\"") }
        let gotSame = try await s.getPhoto(haveVersion: version)
        let same = try XCTUnwrap(gotSame)
        XCTAssertTrue(same.notModified)
        XCTAssertNil(same.bytes)
        XCTAssertEqual(version, same.version)
        XCTAssertEqual(version, p.all("/field/worker/photo").last?.body?.optString("version"))
        p.on("/field/worker/photo") { _ in self.p.error(404, "PHOTO_NOT_FOUND") }
        let none = try await s.getPhoto()
        XCTAssertNil(none)
        p.on("/field/worker/photo") { _ in self.p.error(404, "PHOTOS_DISABLED") }
        await failsWith("PHOTOS_DISABLED") { _ = try await s.getPhoto() }
        p.on("/field/worker/photo") { _ in self.p.error(429, "TOO_MANY_REQUESTS", retryAfter: 90) }
        let e = await failsWith("TOO_MANY_REQUESTS") { _ = try await s.getPhoto() }
        XCTAssertEqual(90, e?.retryAfterSeconds)
    }

    func testSetPhotoSendsTheBytesAsTheyAreAndUpdatesTheSession() async throws {
        let s = try await signedIn(canSet: true)
        p.on("/field/worker/photo/set") { _ in .json(200, #"{"photoVersion":"\#(self.version)"}"#) }
        h.events.clear()
        let after = try await s.setPhoto(jpeg, contentType: "image/jpeg")
        let rec = try XCTUnwrap(p.all("/field/worker/photo/set").first)
        XCTAssertEqual(jpeg, rec.raw)
        XCTAssertEqual("image/jpeg", rec.header("Content-Type"))
        XCTAssertNotNil(rec.proof)
        XCTAssertEqual(version, after.worker.photoVersion)
        let cur = try await worker(s)
        XCTAssertEqual(version, cur.photoVersion)
        XCTAssertEqual(1, h.events.all.filter { if case .sessionChanged = $0 { return true } else { return false } }.count)
        let restarted = try await worker(h.session())
        XCTAssertEqual(version, restarted.photoVersion)
        _ = try await s.setPhoto(jpeg)
        XCTAssertEqual("application/octet-stream", p.all("/field/worker/photo/set").last?.header("Content-Type"))
    }

    func testRemovePhotoClearsTheVersion() async throws {
        let s = try await signedIn(photoVersion: version, canSet: true)
        p.on("/field/worker/photo/remove") { _ in .json(200, #"{"photoVersion":null}"#) }
        let after = try await s.removePhoto()
        XCTAssertNil(after.worker.photoVersion)
        XCTAssertEqual(1, p.count("/field/worker/photo/remove"))
    }

    func testTheRefusalsComeByTheirCodes() async throws {
        let s = try await signedIn(canSet: true)
        let cases: [(Int, String)] = [
            (403, "OWN_PHOTO_NOT_ALLOWED"), (404, "PHOTOS_DISABLED"), (409, "PHOTOS_DISABLED_FOR_CLIENT"), (413, "PHOTO_TOO_LARGE"),
            (415, "PHOTO_UNSUPPORTED_TYPE"), (400, "PHOTO_TYPE_MISMATCH"), (400, "PHOTO_DIMENSIONS_TOO_LARGE"), (400, "PHOTO_UNREADABLE"),
            (400, "PHOTO_EMPTY"), (503, "PHOTO_BUSY"),
        ]
        for (status, code) in cases {
            p.on("/field/worker/photo/set") { _ in self.p.error(status, code) }
            let e = await failsWith(code) { _ = try await s.setPhoto(self.jpeg, contentType: "image/jpeg") }
            XCTAssertEqual(status, e?.status)
        }
        p.on("/field/worker/photo/set") { _ in self.p.error(429, "TOO_MANY_REQUESTS", retryAfter: 30) }
        let e = await failsWith("TOO_MANY_REQUESTS") { _ = try await s.setPhoto(self.jpeg) }
        XCTAssertEqual(30, e?.retryAfterSeconds)
        let after2 = try await worker(s)
        XCTAssertNil(after2.photoVersion)
    }

    func testAnImpersonatingSessionMayNotChangeThePhotoButMayRead() async throws {
        let s = try await signedIn(canSet: true)
        p.on("/field/auth/act-as") { _ in
            self.p.token(workerId: "w2", workerCode: "W002",
                         claims: ["device_mode": "PERSONAL", "act": ["sub": "w1", "typ": "field_worker", "name": "Sup"] as JObject],
                         photoVersion: self.version, canSetOwnPhoto: false)
        }
        let imp = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "1234"))
        XCTAssertNotNil(imp.actor)
        XCTAssertFalse(imp.worker.canSetOwnPhoto)
        let before = p.requests.count
        await failsWith("IMPERSONATING") { _ = try await s.setPhoto(self.jpeg, contentType: "image/jpeg") }
        await failsWith("IMPERSONATING") { _ = try await s.removePhoto() }
        XCTAssertEqual(before, p.requests.count)
        p.on("/field/worker/photo") { _ in self.image("\"\(self.version)-thumb\"") }
        let photo = try await s.getPhoto()
        XCTAssertNotNil(photo)
    }
}
