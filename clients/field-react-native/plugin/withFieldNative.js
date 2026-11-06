// Expo config plugin for @flowcatalyst/field-react-native.
//
//   plugins: [["@flowcatalyst/field-react-native", { faceIdPermission: "Unlock with Face ID" }]]
//
// Android: USE_BIOMETRIC, minSdkVersion >= 26, the GitHub Packages Maven repository and the
// io.flowcatalyst:field-android dependency. iOS: NSFaceIDUsageDescription.
const {
	AndroidConfig,
	withAndroidManifest,
	withAppBuildGradle,
	withGradleProperties,
	withInfoPlist,
	withProjectBuildGradle,
} = require("@expo/config-plugins");

const MIN_SDK = 26;
const FIELD_ANDROID_VERSION = "0.1.0"; // keep equal to clients/field-android/VERSION
const MAVEN_URL = "https://maven.pkg.github.com/flowcatalyst/flowcatalyst";
const DEFAULT_FACE_ID = "Unlock the app with Face ID";
const REPO_MARKER = "// @flowcatalyst/field-react-native: GitHub Packages";
const DEP_MARKER = "// @flowcatalyst/field-react-native: field-android";
const BIOMETRIC = "android.permission.USE_BIOMETRIC";

function mavenBlock(url) {
	return [
		`        ${REPO_MARKER}`,
		"        maven {",
		`            url = uri("${url}")`,
		"            credentials {",
		'                username = findProperty("gpr.user") ?: System.getenv("GITHUB_ACTOR")',
		'                password = findProperty("gpr.key") ?: System.getenv("GITHUB_TOKEN")',
		"            }",
		"        }",
	].join("\n");
}

/** Adds the GitHub Packages repository to allprojects.repositories of the root build.gradle (idempotent). */
function addMavenRepository(contents, url = MAVEN_URL) {
	if (contents.includes(REPO_MARKER)) return contents;
	const re = /allprojects\s*\{\s*repositories\s*\{/;
	const m = re.exec(contents);
	if (m) {
		const at = m.index + m[0].length;
		return contents.slice(0, at) + "\n" + mavenBlock(url) + contents.slice(at);
	}
	return (
		contents.replace(/\s*$/, "\n") +
		"\nallprojects {\n    repositories {\n" +
		mavenBlock(url) +
		"\n    }\n}\n"
	);
}

/** Pins the field-android version the bridge library resolves (root ext). Idempotent. */
function setFieldAndroidVersion(contents, version) {
	const line = `ext.fieldAndroidVersion = "${version}"`;
	if (/ext\.fieldAndroidVersion\s*=/.test(contents)) {
		return contents.replace(/ext\.fieldAndroidVersion\s*=\s*"[^"]*"/, line);
	}
	return contents.replace(/\s*$/, "\n") + "\n" + line + "\n";
}

/** Adds `implementation io.flowcatalyst:field-android` to the app module so app code (a foreground service) can use FieldSession. */
function addFieldAndroidDependency(contents, version = FIELD_ANDROID_VERSION) {
	if (contents.includes(DEP_MARKER)) {
		return contents.replace(
			new RegExp(`(${DEP_MARKER.replace(/[/.]/g, "\\$&")}\\n\\s*implementation ")[^"]*(")`),
			`$1io.flowcatalyst:field-android:${version}$2`,
		);
	}
	const re = /dependencies\s*\{/;
	const m = re.exec(contents);
	const dep = `\n    ${DEP_MARKER}\n    implementation "io.flowcatalyst:field-android:${version}"`;
	if (!m) return contents.replace(/\s*$/, "\n") + `\ndependencies {${dep}\n}\n`;
	const at = m.index + m[0].length;
	return contents.slice(0, at) + dep + contents.slice(at);
}

/** gradle.properties items: android.minSdkVersion raised to at least `min`. */
function setMinSdk(items, min = MIN_SDK) {
	const out = items.slice();
	const i = out.findIndex((p) => p.type === "property" && p.key === "android.minSdkVersion");
	if (i < 0) {
		out.push({ type: "property", key: "android.minSdkVersion", value: String(min) });
	} else {
		const cur = parseInt(out[i].value, 10);
		if (!(cur >= min)) out[i] = { type: "property", key: "android.minSdkVersion", value: String(min) };
	}
	return out;
}

/** Sets NSFaceIDUsageDescription unless the app already provides one. */
function setFaceId(plist, text) {
	if (!plist.NSFaceIDUsageDescription) plist.NSFaceIDUsageDescription = text;
	return plist;
}

function withFieldNative(config, props = {}) {
	const opts = props || {};
	const version = opts.fieldAndroidVersion || FIELD_ANDROID_VERSION;
	const minSdk = Math.max(MIN_SDK, opts.minSdkVersion || 0);

	config = withAndroidManifest(config, (c) => {
		AndroidConfig.Permissions.ensurePermissions(c.modResults, [BIOMETRIC]);
		return c;
	});
	config = withGradleProperties(config, (c) => {
		c.modResults = setMinSdk(c.modResults, minSdk);
		return c;
	});
	config = withProjectBuildGradle(config, (c) => {
		if (c.modResults.language !== "groovy") {
			throw new Error("@flowcatalyst/field-react-native: only a Groovy android/build.gradle is supported");
		}
		let s = addMavenRepository(c.modResults.contents, opts.mavenUrl || MAVEN_URL);
		if (opts.fieldAndroidVersion) s = setFieldAndroidVersion(s, version);
		c.modResults.contents = s;
		return c;
	});
	config = withAppBuildGradle(config, (c) => {
		if (c.modResults.language !== "groovy") {
			throw new Error("@flowcatalyst/field-react-native: only a Groovy android/app/build.gradle is supported");
		}
		c.modResults.contents = addFieldAndroidDependency(c.modResults.contents, version);
		return c;
	});
	if (opts.faceIdPermission !== false) {
		const text = typeof opts.faceIdPermission === "string" ? opts.faceIdPermission : DEFAULT_FACE_ID;
		config = withInfoPlist(config, (c) => {
			c.modResults = setFaceId(c.modResults, text);
			return c;
		});
	}
	return config;
}

module.exports = withFieldNative;
module.exports.default = withFieldNative;
module.exports.addMavenRepository = addMavenRepository;
module.exports.setFieldAndroidVersion = setFieldAndroidVersion;
module.exports.addFieldAndroidDependency = addFieldAndroidDependency;
module.exports.setMinSdk = setMinSdk;
module.exports.setFaceId = setFaceId;
module.exports.constants = { MIN_SDK, FIELD_ANDROID_VERSION, MAVEN_URL, DEFAULT_FACE_ID, REPO_MARKER, DEP_MARKER };
