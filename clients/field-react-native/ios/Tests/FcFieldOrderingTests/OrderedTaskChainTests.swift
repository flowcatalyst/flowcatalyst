import XCTest
@testable import FcFieldOrdering

final class OrderedTaskChainTests: XCTestCase {
    private actor Log {
        var items: [Int] = []
        func add(_ i: Int) { items.append(i) }
    }

    /// Operations that suspend for different times still run in enqueue order, and the last write wins.
    func testRunsInEnqueueOrder() async {
        let chain = OrderedTaskChain()
        let log = Log()
        for i in 0..<2000 {
            chain.enqueue {
                if i % 7 == 0 { await Task.yield() }
                if i % 13 == 0 { try? await Task.sleep(nanoseconds: 50_000) }
                await log.add(i)
            }
        }
        await chain.drain()
        let items = await log.items
        XCTAssertEqual(Array(0..<2000), items)
    }

    /// The keepAlive pattern: many on/off toggles from several threads' worth of rapid calls, final value is the last.
    func testLastToggleWins() async {
        let chain = OrderedTaskChain()
        let log = Log()
        for i in 0..<1000 { chain.enqueue { await log.add(i % 2) } } // 0,1,0,1,...
        chain.enqueue { await log.add(1) } // last call: on
        await chain.drain()
        let items = await log.items
        XCTAssertEqual(1, items.last)
        XCTAssertEqual(1001, items.count)
    }

    /// Without the chain the order is not guaranteed; this documents why the chain exists (it asserts nothing
    /// about the unordered case, only that the chained one never reorders).
    func testChainedOperationsNeverOverlap() async {
        let chain = OrderedTaskChain()
        let running = Log()
        for i in 0..<200 {
            chain.enqueue {
                await running.add(i)
                await Task.yield()
                await running.add(-i - 1)
            }
        }
        await chain.drain()
        let items = await running.items
        for i in 0..<200 { XCTAssertEqual([i, -i - 1], Array(items[(2 * i)..<(2 * i + 2)])) }
    }
}
