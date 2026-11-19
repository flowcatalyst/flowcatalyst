# App updates

The platform serves **over-the-air updates** to phone apps built with Expo. Most releases of
such an app change only JavaScript and images; with app updates they reach phones without a new
store build or APK. The phone app uses the open-source `expo-updates` library and talks to the
platform over the open Expo Updates protocol; no hosted Expo service is involved.

## How it fits together

| Term | Meaning |
|---|---|
| **Application** | The platform application that owns the phone app, named by its code (`acme`). |
| **App key** | One phone app of the application (`driver-app`). An application may have several. |
| **Runtime version** | The native code an update needs, set when the binary is built (`1.4.0`). An update reaches only binaries of its runtime version. |
| **Channel** | Set in the binary (`production`, `staging`). Phones on a channel get that channel's updates. |
| **Branch** | One app key, platform (`android` or `ios`), runtime version and channel. Updates are published to a branch. |
| **Update** | One `expo export` of the app for one platform, published to a branch. Immutable. |

A phone asks at launch whether its branch has a newer update, downloads it, checks its
signature, and runs it on the next start. The newest update of a branch is live; a staged rollout
sends it to a share of the phones and the rest keep the one before.

Every manifest the platform sends is **signed**. The app embeds a certificate and refuses anything
the matching key did not sign; that is where its integrity comes from, so the update addresses are
public.

## Setting up an app

1. **Create a signing key and certificate** (once per key):

   ```sh
   fcdev updates keygen --key-id main --common-name "acme driver-app updates" --years 10 --out-dir ./keys
   ```

   Put `keys/private-key.pem` in the platform's secret store and name it in
   `FC_APP_UPDATES_SIGNING_KEYS` (`main=aws-sm://...`). Copy `keys/certificate.pem` into the app.
   The certificate's expiry stops every update: plan the next binary before it.

2. **Configure the app** (`app.json`):

   ```json
   {
     "expo": {
       "runtimeVersion": "1.4.0",
       "updates": {
         "url": "https://updates.example.com/app-updates/acme/driver-app/manifest",
         "requestHeaders": { "expo-channel-name": "production" },
         "codeSigningCertificate": "./certificate.pem",
         "codeSigningMetadata": { "keyid": "main", "alg": "rsa-v1_5-sha256" }
       }
     }
   }
   ```

   Build the binary as usual (`npx expo prebuild`, Gradle, Xcode).

3. **Give the pipeline a credential**: the application's own service account may publish, roll
   out and roll back its own application's updates (role `platform:application-service`).
   Operators hold `platform:app-update-admin`.

## Publishing

```sh
npx expo export --platform android --platform ios --output-dir dist
npx expo config --type public --json > dist/expoConfig.json
fcdev updates publish dist --app acme --app-key driver-app --channel production \
  --runtime-version 1.4.0 --message "Fix the delivery list" --commit "$GIT_SHA" --profile prod
```

The command uploads only the files the platform does not have, then publishes one update per
platform; each is live when the command returns. An export does not record its runtime version:
pass `--runtime-version`, or let the command read a plain `runtimeVersion` from `expoConfig.json`.
It refuses to publish without one, or from an export that is not well formed.

`--rollout 10` publishes to 10% of the branch's phones. A phone's share is fixed per update, so it
does not flap between launches.

## Rollouts and rollbacks

| Command | What phones get |
|---|---|
| `fcdev updates rollout <updateId> 50` | The newest update reaches 50% (only the newest update of a branch can be partial, and a new publish waits until it is at 100% or rolled back). Lowering a share does not take the update off phones that have it. |
| `fcdev updates rollback --app acme --app-key driver-app --os android --runtime-version 1.4.0 --channel production` | The update before the current one, published again as a new update (a phone only moves forward to a newer update). |
| `... rollback ... --to-embedded` | The bundle built into the binary. Publishing a new update ends it. |
| `fcdev updates republish <updateId>` | Any earlier update of the branch, again as a new update. |
| `fcdev updates list --app acme` / `branches --app acme` | What was published, by whom, and what each branch serves. |

Every publish, rollout change and rollback is in the audit log, and so is every binary published and
every install link created, replaced or removed.

## Handing out the Android app

People who install the app outside a store (sideloading) get it from an **install link**: one address
per phone app and channel that always offers the newest build. The platform keeps two builds of each
channel apart, as **variants**: `release`, the properly signed build everyone uses, and `debug`, for
testers and developers. The page offers both when both exist, the regular build first.

**The pipeline publishes each build** with the application's service account:

```sh
fcdev updates binaries publish android/app/build/outputs/apk/release/app-release.apk \
  --message "1.4.0" --commit "$GIT_SHA" --profile prod
fcdev updates binaries publish android/app/build/outputs/apk/debug/app-debug.apk --profile prod
```

The command reads the APK: its package, version, runtime version and signing certificate, and the
application, app key and channel its update URL and `expo-channel-name` name. It refuses a file that
is not a signed APK (v2 or v3 signature), and a `release` build that is debuggable or signed with the
Android debug keystore (that key is public; sign release builds with the app's own key). It uploads
the file once and publishes it; publishing the same file again changes nothing. `fcdev updates
binaries inspect <apk>` shows what it reads, without a platform; `binaries list --app acme` what is
published.

**An operator makes the link** (role App Update Administrator; an application's service account
cannot):

```sh
fcdev updates links create --app acme --app-key driver-app --channel production --profile prod
# https://platform.example.com/app-updates/acme/driver-app/install/3q2-…   (shown once)
```

Anyone holding the address can download the builds, so send it to the people who need it. To cut old
copies off, `fcdev updates links replace <linkId>`: the old address stops working at once and the
command prints the new one. `links remove <linkId>` takes the link away; `links list --app acme` shows
the links (never their addresses).

**What a person sees**: a page with the app's name, each build with its version, date and size and a
download button, the steps to install an APK on Android (allow the browser to install apps, open the
file, install), what to do when switching between the regular and the debug build needs the app
uninstalled first, and a QR code of the page so someone reading it on a computer can open it on the
phone. A link that was replaced or removed shows "This link does not work".

**Updates and binaries.** An update is for the binaries of one runtime version. `fcdev updates
binaries has --app acme --app-key driver-app --channel production --runtime-version "$RV"` exits 0
when a binary of that runtime version is published on the channel and 1 when none is, for a script to
branch on. `fcdev updates publish` warns when an Android update's runtime version has no binary on its
channel, and `--require-binary` refuses to publish it instead (use it when every phone of the channel
is installed from this platform).

## Trying it locally

`fcdev start` creates a development signing key (key id `main`) and serves the same routes.
`fcdev updates certificate > certificate.pem` gives the certificate for a development build. An
Android emulator reaches the host's fcdev at `http://10.0.2.2:<port>`, so the development
build's update URL is `http://10.0.2.2:8080/app-updates/<application>/<app key>/manifest`
(an `http://` URL needs cleartext traffic allowed in the development build). Publish with
`fcdev updates publish` against the local fcdev, as above without `--profile`.
