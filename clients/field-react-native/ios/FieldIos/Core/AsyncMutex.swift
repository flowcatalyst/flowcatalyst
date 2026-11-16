import Foundation

/// A FIFO async mutual-exclusion lock. Actors are reentrant at every `await`, so the session's "one platform call at
/// a time" and "one refresh at a time" rules (Kotlin `Mutex`) need an explicit lock.
final class AsyncMutex: @unchecked Sendable {
    private let state = NSLock()
    private var held = false
    private var waiters: [CheckedContinuation<Void, Never>] = []

    func lock() async {
        let wait: Bool = state.withLock {
            if held { return true }
            held = true
            return false
        }
        if !wait { return }
        await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
            let resumeNow: Bool = state.withLock {
                if held { waiters.append(c); return false }
                held = true
                return true
            }
            if resumeNow { c.resume() }
        }
    }

    func unlock() {
        let next: CheckedContinuation<Void, Never>? = state.withLock {
            if waiters.isEmpty { held = false; return nil }
            return waiters.removeFirst() // ownership passes to the waiter: `held` stays true
        }
        next?.resume()
    }
}
