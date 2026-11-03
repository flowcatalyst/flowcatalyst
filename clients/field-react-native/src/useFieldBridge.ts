import { useMemo, useRef } from "react";
import {
	INJECTED_BEFORE_CONTENT_LOADED,
	createBridgeHost,
	type BridgeMessageEvent,
	type InjectTarget,
} from "./bridge";
import { getNativeModule } from "./native";

export interface UseFieldBridgeOptions {
	allowedOrigins: string[];
	/** The app's own onMessage handler; gets every message that is not a bridge request. */
	onMessage?: (event: BridgeMessageEvent) => void;
}

/**
 * Spread the result on a react-native-webview <WebView>:
 *   <WebView source={{uri}} {...useFieldBridge({allowedOrigins: [origin]})} />
 */
export function useFieldBridge(options: UseFieldBridgeOptions) {
	const ref = useRef<InjectTarget | null>(null);
	const optsRef = useRef(options);
	optsRef.current = options;
	const originsKey = options.allowedOrigins.join("\n");
	const host = useMemo(
		() =>
			createBridgeHost({
				allowedOrigins: originsKey.split("\n").filter((s) => s.length > 0),
				native: getNativeModule,
				getTarget: () => ref.current,
				onMessage: (e) => optsRef.current.onMessage?.(e),
			}),
		[originsKey],
	);
	return {
		ref,
		onMessage: host.onMessage,
		injectedJavaScriptBeforeContentLoaded: INJECTED_BEFORE_CONTENT_LOADED,
	};
}
