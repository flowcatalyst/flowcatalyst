// The passkey forms of the /ui sign-in pages. Three forms, found by attribute
// (the pages carry no inline script):
//
//   form[data-fc-passkey-signin]    email -> assertion -> session, then navigate
//   form[data-fc-passkey-register]  name  -> attestation, then reload the page
//   form[data-fc-passkey-reset]     the reset-password form of an account with
//                                   a passkey: its [data-fc-passkey-reset-run]
//                                   button runs an assertion against a
//                                   challenge bound to the reset link, puts the
//                                   answer in the form and submits it; the
//                                   password-reset confirm endpoint checks it
//
// Both talk to the platform's WebAuthn endpoints (/auth/webauthn/*), which
// check everything and set the session cookie; this file only drives the
// browser's ceremony with @simplewebauthn/browser (the library the current UI
// uses) and shows a failure in the form's [data-fc-passkey-error] element.
import {
  browserSupportsWebAuthn,
  startAuthentication,
  startRegistration,
} from "@simplewebauthn/browser";

async function post(path, body) {
  return postTo("/auth/webauthn" + path, body);
}

async function postTo(url, body) {
  const res = await fetch(url, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    const e = new Error(typeof err.message === "string" && err.message ? err.message : "Request failed");
    e.status = res.status;
    throw e;
  }
  return res.json();
}

function show(form, message) {
  const el = form.querySelector("[data-fc-passkey-error]");
  if (!el) return;
  el.textContent = message;
  el.hidden = !message;
}

function busy(form, on) {
  for (const b of form.querySelectorAll("button")) b.disabled = on;
}

// next is a path on this site; anything else is ignored (the server renders
// it already checked, this is the second look).
function go(next) {
  const u = new URL(next || "/ui/", location.origin);
  location.assign(u.origin === location.origin ? u.pathname + u.search : "/ui/");
}

async function signIn(form) {
  const email = new FormData(form).get("email")?.toString().trim();
  if (!email) return show(form, "Enter your email address.");
  const begin = await post("/authenticate/begin", { email });
  const credential = await startAuthentication({ optionsJSON: begin.options.publicKey });
  await post("/authenticate/complete", { stateId: begin.stateId, credential });
  go(form.dataset.next);
}

async function register(form) {
  const name = new FormData(form).get("name")?.toString().trim();
  if (!name) return show(form, "Please name this passkey before adding it.");
  const begin = await post("/register/begin", { displayName: name });
  const credential = await startRegistration({ optionsJSON: begin.options.publicKey });
  await post("/register/complete", { stateId: begin.stateId, name, credential });
  location.assign(form.dataset.done || location.pathname);
}

async function resetWithPasskey(form) {
  // The new password must be filled in before the passkey prompt.
  if (!form.reportValidity()) return;
  const token = new FormData(form).get("token")?.toString() ?? "";
  const begin = await postTo("/auth/password-reset/passkey/begin", { token });
  const credential = await startAuthentication({ optionsJSON: begin.options.publicKey });
  form.elements.namedItem("passkeyStateId").value = begin.stateId;
  form.elements.namedItem("passkeyCredential").value = JSON.stringify(credential);
  // A native submit: the server sends the proof on with the new password.
  HTMLFormElement.prototype.submit.call(form);
}

function wireReset() {
  for (const form of document.querySelectorAll("form[data-fc-passkey-reset]")) {
    const run = form.querySelector("[data-fc-passkey-reset-run]");
    if (!run) continue;
    if (!browserSupportsWebAuthn()) {
      run.disabled = true;
      show(form, "This browser doesn't support passkeys. Use a recent browser over HTTPS.");
      continue;
    }
    run.addEventListener("click", async (evt) => {
      evt.preventDefault();
      show(form, "");
      busy(form, true);
      try {
        await resetWithPasskey(form);
      } catch (err) {
        show(form, err?.name === "NotAllowedError" ? "The passkey check was cancelled." : "The passkey check failed.");
      }
      busy(form, false);
    });
  }
}

// The same messages as the current UI: a cancelled prompt is the usual case,
// a repeated authenticator the second; a failed sign-in never says why.
function failure(err, signingIn) {
  if (err?.name === "NotAllowedError") {
    return signingIn ? "Passkey sign-in was cancelled." : "Cancelled — no passkey was added.";
  }
  if (!signingIn && (err?.name === "InvalidStateError" || /previously registered/i.test(err?.message ?? ""))) {
    return "This device already has a passkey for your account — use a different device or security key to add another.";
  }
  if (signingIn) return "Passkey sign-in failed.";
  return err?.message || "Failed to add passkey";
}

function wire(selector, run, signingIn) {
  for (const form of document.querySelectorAll(selector)) {
    if (!browserSupportsWebAuthn()) {
      busy(form, true);
      show(form, "This browser doesn't support passkeys. Use a recent browser over HTTPS.");
      continue;
    }
    form.addEventListener("submit", async (evt) => {
      evt.preventDefault();
      show(form, "");
      busy(form, true);
      try {
        await run(form);
      } catch (err) {
        show(form, failure(err, signingIn));
        busy(form, false);
      }
    });
  }
}

wire("form[data-fc-passkey-signin]", signIn, true);
wire("form[data-fc-passkey-register]", register, false);
wireReset();
