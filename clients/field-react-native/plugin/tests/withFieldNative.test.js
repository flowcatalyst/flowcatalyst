const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { describe, it } = require("node:test");
const { compileModsAsync } = require("@expo/config-plugins");
const withFieldNative = require("../withFieldNative");

const { addMavenRepository, addFieldAndroidDependency, setFieldAndroidVersion, setMinSdk, setFaceId, constants } = withFieldNative;

const ROOT_GRADLE = `buildscript {\n  repositories { google() }\n}\nallprojects {\n    repositories {\n        google()\n        mavenCentral()\n    }\n}\n`;
const APP_GRADLE = `android {\n}\ndependencies {\n    implementation("com.facebook.react:react-android")\n}\n`;
const MANIFEST = `<?xml version="1.0" encoding="utf-8"?>\n<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.example.app">\n  <uses-permission android:name="android.permission.INTERNET"/>\n  <application android:name=".MainApplication"><activity android:name=".MainActivity"/></application>\n</manifest>\n`;

function project() {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "fcplugin-"));
	const a = path.join(root, "android");
	fs.mkdirSync(path.join(a, "app/src/main"), { recursive: true });
	fs.writeFileSync(path.join(a, "build.gradle"), ROOT_GRADLE);
	fs.writeFileSync(path.join(a, "app/build.gradle"), APP_GRADLE);
	fs.writeFileSync(path.join(a, "app/src/main/AndroidManifest.xml"), MANIFEST);
	fs.writeFileSync(path.join(a, "gradle.properties"), "org.gradle.jvmargs=-Xmx2g\nandroid.minSdkVersion=24\n");
	return root;
}

async function run(root, props, extra = {}) {
	const config = withFieldNative({ name: "app", slug: "app", android: { package: "com.example.app" }, ...extra }, props);
	await compileModsAsync(config, { projectRoot: root, platforms: ["android"], assertMissingModProviders: false });
	const a = path.join(root, "android");
	return {
		root: fs.readFileSync(path.join(a, "build.gradle"), "utf8"),
		app: fs.readFileSync(path.join(a, "app/build.gradle"), "utf8"),
		manifest: fs.readFileSync(path.join(a, "app/src/main/AndroidManifest.xml"), "utf8"),
		props: fs.readFileSync(path.join(a, "gradle.properties"), "utf8"),
		config,
	};
}

describe("pure modifiers", () => {
	it("adds the Maven repository once, inside allprojects.repositories, with env/property credentials", () => {
		const once = addMavenRepository(ROOT_GRADLE);
		assert.ok(once.includes(constants.MAVEN_URL));
		assert.ok(once.includes('findProperty("gpr.user") ?: System.getenv("GITHUB_ACTOR")'));
		assert.ok(once.includes('findProperty("gpr.key") ?: System.getenv("GITHUB_TOKEN")'));
		assert.ok(once.indexOf(constants.REPO_MARKER) > once.indexOf("allprojects"));
		assert.equal(addMavenRepository(once), once);
		assert.equal(once.split(constants.MAVEN_URL).length, 2);
	});

	it("creates allprojects when the root file has none", () => {
		const s = addMavenRepository("buildscript {}\n");
		assert.match(s, /allprojects \{\s+repositories \{/);
	});

	it("adds the dependency once and can change its version", () => {
		const s = addFieldAndroidDependency(APP_GRADLE);
		assert.ok(s.includes('implementation "io.flowcatalyst:field-android:' + constants.FIELD_ANDROID_VERSION + '"'));
		assert.equal(addFieldAndroidDependency(s), s);
		const v = addFieldAndroidDependency(s, "9.9.9");
		assert.ok(v.includes("field-android:9.9.9"));
		assert.ok(!v.includes("field-android:" + constants.FIELD_ANDROID_VERSION));
	});

	it("pins ext.fieldAndroidVersion idempotently", () => {
		const s = setFieldAndroidVersion(ROOT_GRADLE, "1.2.3");
		assert.ok(s.includes('ext.fieldAndroidVersion = "1.2.3"'));
		assert.equal(setFieldAndroidVersion(s, "1.2.4").match(/ext\.fieldAndroidVersion/g).length, 1);
	});

	it("raises minSdk to 26 but never lowers it", () => {
		const get = (items) => items.find((p) => p.key === "android.minSdkVersion").value;
		assert.equal(get(setMinSdk([])), "26");
		assert.equal(get(setMinSdk([{ type: "property", key: "android.minSdkVersion", value: "24" }])), "26");
		assert.equal(get(setMinSdk([{ type: "property", key: "android.minSdkVersion", value: "28" }])), "28");
	});

	it("sets Face ID text but keeps an existing one", () => {
		assert.equal(setFaceId({}, "x").NSFaceIDUsageDescription, "x");
		assert.equal(setFaceId({ NSFaceIDUsageDescription: "mine" }, "x").NSFaceIDUsageDescription, "mine");
	});
});

describe("plugin through compileModsAsync (real files)", () => {
	it("applies every Android mod", async () => {
		const out = await run(project(), {});
		assert.match(out.manifest, /android\.permission\.USE_BIOMETRIC/);
		assert.match(out.manifest, /android\.permission\.INTERNET/);
		assert.match(out.props, /android\.minSdkVersion=26/);
		assert.ok(out.root.includes(constants.MAVEN_URL));
		assert.ok(out.app.includes("io.flowcatalyst:field-android:"));
	});

	it("is idempotent when run twice over the same files", async () => {
		const root = project();
		await run(root, {});
		const second = await run(root, {});
		assert.equal(second.root.split(constants.MAVEN_URL).length, 2);
		assert.equal(second.app.split("io.flowcatalyst:field-android").length, 2);
		assert.equal(second.manifest.split("USE_BIOMETRIC").length, 2);
		assert.equal(second.props.match(/android\.minSdkVersion/g).length, 1);
	});

	it("honours fieldAndroidVersion and minSdkVersion options", async () => {
		const out = await run(project(), { fieldAndroidVersion: "0.3.0", minSdkVersion: 29 });
		assert.ok(out.app.includes("field-android:0.3.0"));
		assert.ok(out.root.includes('ext.fieldAndroidVersion = "0.3.0"'));
		assert.match(out.props, /android\.minSdkVersion=29/);
	});

	it("registers the iOS Face ID mod, and omits it with faceIdPermission: false", () => {
		assert.ok(withFieldNative({ name: "a", slug: "a" }, { faceIdPermission: "Unlock please" }).mods.ios.infoPlist);
		assert.equal(withFieldNative({ name: "a", slug: "a" }, { faceIdPermission: false }).mods.ios, undefined);
	});
});
