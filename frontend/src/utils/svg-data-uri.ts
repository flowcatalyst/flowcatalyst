/**
 * A stored logo SVG as a data: URI for an <img src>. An SVG shown through
 * <img> never runs script (no event handlers, no <script>, no foreignObject
 * HTML), which inserting it with v-html would. Every page that shows a theme
 * logo SVG uses this, as the server-rendered sign-in page does.
 */
export function svgDataUri(svg: string): string {
	const bytes = new TextEncoder().encode(svg);
	let binary = "";
	for (const b of bytes) {
		binary += String.fromCharCode(b);
	}
	return `data:image/svg+xml;base64,${btoa(binary)}`;
}
