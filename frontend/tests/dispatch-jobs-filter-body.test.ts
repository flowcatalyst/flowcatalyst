import { describe, expect, it } from "vitest";
import { buildQuery, filterBody } from "../src/api/dispatch-jobs";

describe("dispatch job filter body: event type, subscription and connection", () => {
	it("sends the new filters under the server's names, lists as CSV", () => {
		const body = filterBody({
			eventTypes: ["a:b:c:d", "a:b:c:e"],
			subscriptionIds: ["sub_1", "sub_2"],
			subscriptionCode: "orders-*",
			subscriptionName: "Orders",
			connectionId: "con_1",
			connectionCode: "main",
		});
		expect(body).toEqual({
			eventTypes: "a:b:c:d,a:b:c:e",
			subscriptionIds: "sub_1,sub_2",
			subscriptionCode: "orders-*",
			subscriptionName: "Orders",
			connectionId: "con_1",
			connectionCode: "main",
		});
	});

	it("omits what is empty and puts the rest on the query string", () => {
		expect(filterBody({ eventTypes: [], subscriptionCode: "" })).toEqual({});
		const qs = new URLSearchParams(buildQuery({ eventTypes: ["x:y:z:w"], connectionCode: "c" }).slice(1));
		expect(qs.get("eventTypes")).toBe("x:y:z:w");
		expect(qs.get("connectionCode")).toBe("c");
	});
});
