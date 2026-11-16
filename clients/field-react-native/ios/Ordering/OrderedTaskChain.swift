import Foundation

/// Runs async operations one after another, in the order they were enqueued.
///
/// Two `Task { }` blocks started one after the other are not guaranteed to reach an actor in start order. The
/// fire-and-forget bridge methods (`keepAlive`, `noteNonce`) have no promise to await, and a later call must
/// not be overtaken by an earlier one (`keepAlive(true)` then `keepAlive(false)` must end off, as on Android where the
/// calls run synchronously). Each new task awaits the previous one first.
final class OrderedTaskChain: @unchecked Sendable {
    private let lock = NSLock()
    private var last: Task<Void, Never>?

    /// Returns at once; `operation` runs after every operation enqueued before it has finished.
    func enqueue(_ operation: @escaping @Sendable () async -> Void) {
        lock.lock()
        let previous = last
        last = Task {
            await previous?.value
            await operation()
        }
        lock.unlock()
    }

    /// Suspends until everything enqueued so far has run (for tests).
    func drain() async {
        let t = lock.withLock { last }
        await t?.value
    }
}
