import Foundation

/// Thread-safe holder of the current `FieldState` (Kotlin `StateFlow`): readable without `await`, observable as a stream.
final class StateBox: @unchecked Sendable {
    private let lock = NSLock()
    private var current: FieldState = .signedOut
    private var observers: [UUID: AsyncStream<FieldState>.Continuation] = [:]

    var value: FieldState { lock.withLock { current } }

    func set(_ s: FieldState) {
        let conts: [AsyncStream<FieldState>.Continuation] = lock.withLock {
            current = s
            return Array(observers.values)
        }
        for c in conts { c.yield(s) }
    }

    func stream() -> AsyncStream<FieldState> {
        AsyncStream { cont in
            let id = UUID()
            let initial: FieldState = lock.withLock {
                observers[id] = cont
                return current
            }
            cont.yield(initial)
            cont.onTermination = { [weak self] _ in
                guard let self else { return }
                _ = self.lock.withLock { self.observers.removeValue(forKey: id) }
            }
        }
    }
}

/// Returned by `FieldSession.addListener`. The listener stays registered until `cancel()` (Kotlin: the returned
/// `Closeable`); dropping the token does not unregister it.
public final class FieldListenerToken: @unchecked Sendable {
    private let onCancel: @Sendable () -> Void
    private let lock = NSLock()
    private var done = false

    init(onCancel: @escaping @Sendable () -> Void) { self.onCancel = onCancel }

    public func cancel() {
        let first: Bool = lock.withLock {
            if done { return false }
            done = true
            return true
        }
        if first { onCancel() }
    }
}

final class ListenerBox: @unchecked Sendable {
    private let lock = NSLock()
    private var listeners: [(id: UUID, fn: @Sendable (FieldEvent) -> Void)] = []

    func add(_ fn: @escaping @Sendable (FieldEvent) -> Void) -> FieldListenerToken {
        let id = UUID()
        lock.withLock { listeners.append((id, fn)) }
        return FieldListenerToken { [weak self] in
            guard let self else { return }
            self.lock.withLock { self.listeners.removeAll { $0.id == id } }
        }
    }

    func emit(_ e: FieldEvent) {
        let snapshot = lock.withLock { listeners.map(\.fn) }
        for l in snapshot { l(e) }
    }
}
