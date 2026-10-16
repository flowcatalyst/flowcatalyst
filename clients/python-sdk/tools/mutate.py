"""Manual mutation check for the retry policy, transport and token cache.

Applies each mutant to the source, runs the suite, restores the file. Every mutant must be
KILLED (suite fails). Run from anywhere: .venv/bin/python tools/mutate.py
"""

import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
SRC = ROOT / "packages/flowcatalyst/src/flowcatalyst"
PY = str(ROOT / ".venv/bin/python")
M = [
    # retry.py
    (
        "retry.py",
        "frozenset({408, 425, 429, 502, 503, 504})",
        "frozenset({408, 425, 429, 500, 502, 503, 504})",
        "500 retried by default",
    ),
    (
        "retry.py",
        "frozenset({408, 425, 429, 502, 503, 504})",
        "frozenset({408, 429, 502, 503, 504})",
        "425 dropped",
    ),
    (
        "retry.py",
        "frozenset({408, 425, 429, 502, 503, 504})",
        "frozenset({425, 429, 502, 503, 504})",
        "408 dropped",
    ),
    (
        "retry.py",
        "frozenset({408, 425, 429, 502, 503, 504})",
        "frozenset({408, 425, 429, 502, 503})",
        "504 dropped",
    ),
    (
        "retry.py",
        "frozenset({408, 425, 429, 502, 503, 504})",
        "frozenset({408, 425, 502, 503, 504})",
        "429 dropped",
    ),
    ("retry.py", "max_attempts: int = 3", "max_attempts: int = 4", "attempts 4"),
    ("retry.py", "base_delay: float = 0.1", "base_delay: float = 0.2", "base 200ms"),
    ("retry.py", "max_delay: float = 5.0", "max_delay: float = 10.0", "cap 10s"),
    (
        "retry.py",
        "retry_after_cap: float = 30.0",
        "retry_after_cap: float = 60.0",
        "retry-after cap",
    ),
    (
        "retry.py",
        "policy.retry_server_error and status == 500 and idempotent",
        "policy.retry_server_error and status == 500",
        "500 opt-in ignores idempotency",
    ),
    (
        "retry.py",
        "policy.retry_server_error and status == 500 and idempotent",
        "status == 500 and idempotent",
        "500 opt-in ignored",
    ),
    (
        "retry.py",
        "policy.base_delay * (2.0**exponent)",
        "policy.base_delay * (3.0**exponent)",
        "backoff factor 3",
    ),
    (
        "retry.py",
        "policy.base_delay * (2.0**exponent)",
        "policy.base_delay",
        "no exponential growth",
    ),
    ("retry.py", "min(policy.max_delay,", "max(policy.max_delay,", "cap becomes floor"),
    ("retry.py", "min(attempt - 1, 60)", "(attempt - 1)", "no overflow guard"),
    ("retry.py", "connect: float = 5.0", "connect: float = 10.0", "connect timeout"),
    ("retry.py", "read: float = 30.0", "read: float = 60.0", "read timeout"),
    ("retry.py", "total: float = 60.0", "total: float = 120.0", "total timeout"),
    (
        "retry.py",
        "if not math.isfinite(seconds) or seconds < 0:",
        "if seconds < 0:",
        "nan/inf retry-after accepted",
    ),
    ("retry.py", "return max(0.0, (when", "return ((when", "past date negative"),
    ("retry.py", "if self.max_attempts < 1", "if self.max_attempts < 0", "validation boundary"),
    # http.py
    (
        "http.py",
        "self._rng() * backoff_ceiling(self._retry, attempt)",
        "backoff_ceiling(self._retry, attempt)",
        "no jitter",
    ),
    (
        "http.py",
        "self._rng() * backoff_ceiling(self._retry, attempt)",
        "self._rng() + backoff_ceiling(self._retry, attempt)",
        "jitter additive",
    ),
    (
        "http.py",
        "return max(jittered, min(retry_after, self._retry.retry_after_cap))",
        "return max(jittered, retry_after)",
        "retry-after uncapped",
    ),
    (
        "http.py",
        "return max(jittered, min(retry_after, self._retry.retry_after_cap))",
        "return jittered",
        "retry-after ignored",
    ),
    (
        "http.py",
        "return max(jittered, min(retry_after, self._retry.retry_after_cap))",
        "return min(retry_after, self._retry.retry_after_cap)",
        "retry-after shortens backoff",
    ),
    (
        "http.py",
        "if attempt < self._retry.max_attempts and should_retry_status(",
        "if should_retry_status(",
        "unbounded status retry",
    ),
    (
        "http.py",
        "if attempt < self._retry.max_attempts and should_retry_status(",
        "if attempt < self._retry.max_attempts or should_retry_status(",
        "status retry or",
    ),
    ("http.py", "idempotent or isinstance(exc, _NOT_SENT)", "True", "POST retried after sent"),
    (
        "http.py",
        "idempotent or isinstance(exc, _NOT_SENT)",
        "idempotent",
        "connect error not retried for POST",
    ),
    ("http.py", "attempt < self._retry.max_attempts and (", "(", "unbounded transport retry"),
    (
        "http.py",
        "attempt -= 1  # the refresh retry",
        "attempt -= 0  # the refresh retry",
        "401 refresh spends budget",
    ),
    (
        "http.py",
        "if token is not None and self._auth is not None and not refreshed:",
        "if token is not None and self._auth is not None:",
        "unbounded 401 refresh",
    ),
    ("http.py", "refreshed = True", "refreshed = False", "refresh flag never set"),
    (
        "http.py",
        "await self._auth.refresh(rejected=token)",
        "await self._auth.get(force=False)",
        "401 does not force",
    ),
    (
        "http.py",
        'headers["Authorization"] = f"Bearer {token}"',
        'headers["Authorization"] = f"Token {token}"',
        "scheme",
    ),
    (
        "http.py",
        "is_idempotent = method in IDEMPOTENT_METHODS if idempotent is None else idempotent",
        "is_idempotent = method in IDEMPOTENT_METHODS",
        "idempotent override ignored",
    ),
    (
        "http.py",
        'IDEMPOTENT_METHODS = frozenset({"GET", "HEAD", "OPTIONS", "PUT", "DELETE"})',
        'IDEMPOTENT_METHODS = frozenset({"GET", "HEAD", "OPTIONS", "PUT", "DELETE", "POST"})',
        "POST idempotent",
    ),
    (
        "http.py",
        'IDEMPOTENT_METHODS = frozenset({"GET", "HEAD", "OPTIONS", "PUT", "DELETE"})',
        'IDEMPOTENT_METHODS = frozenset({"GET", "HEAD", "OPTIONS", "PUT"})',
        "DELETE not idempotent",
    ),
    (
        "http.py",
        "async with asyncio.timeout(timeouts.total):",
        "async with asyncio.timeout(None):",
        "no overall deadline",
    ),
    (
        "http.py",
        "(httpx.ConnectError, httpx.ConnectTimeout, httpx.PoolTimeout)",
        "(httpx.ConnectError, httpx.ConnectTimeout, httpx.PoolTimeout, httpx.ReadError)",
        "ReadError treated as not sent",
    ),
    ("http.py", "timeout=timeouts.to_httpx()", "timeout=None", "per-request timeouts dropped"),
    # tokens.py
    ("tokens.py", "DEFAULT_MARGIN = 60.0", "DEFAULT_MARGIN = 30.0", "margin 30"),
    (
        "tokens.py",
        "self._refresh_at = self._clock() + lifetime - margin",
        "self._refresh_at = self._clock() + lifetime",
        "no early expiry",
    ),
    (
        "tokens.py",
        "margin = min(self._margin, lifetime / 2)",
        "margin = self._margin",
        "no short-lived guard",
    ),
    (
        "tokens.py",
        "self._clock() < self._refresh_at",
        "self._clock() <= self._refresh_at",
        "boundary <=",
    ),
    (
        "tokens.py",
        "return self._token is not None and self._clock() < self._refresh_at",
        "return self._token is not None",
        "never expires",
    ),
    (
        "tokens.py",
        "if self._token is not None and self._token != rejected and self._fresh():",
        "if False:",
        "refresh never reuses newer token",
    ),
    (
        "tokens.py",
        "if self._token is not None and self._token != rejected and self._fresh():",
        "if self._token is not None and self._fresh():",
        "refresh reuses rejected token",
    ),
    (
        "tokens.py",
        "        async with self._lock:\n            if not force and self._token is not None and self._fresh():\n                return self._token\n            return await self._fetch()",
        "        if True:\n            if not force and self._token is not None and self._fresh():\n                return self._token\n            return await self._fetch()",
        "no lock in get",
    ),
    (
        "tokens.py",
        "        async with self._lock:\n            if self._token",
        "        if True:\n            if self._token",
        "no lock in refresh",
    ),
    (
        "tokens.py",
        "            if not force and self._token is not None and self._fresh():\n                return self._token\n            return await self._fetch()",
        "            return await self._fetch()",
        "no double-check under lock",
    ),
    (
        "tokens.py",
        "if not force and self._token is not None and self._fresh():\n            return self._token\n        async",
        "if self._token is not None and self._fresh():\n            return self._token\n        async",
        "force ignored on fast path",
    ),
    ("tokens.py", "idempotent=True,", "idempotent=False,", "token fetch not idempotent"),
    (
        "tokens.py",
        "if exc.status is not None and 400 <= exc.status < 500:",
        "if exc.status is not None:",
        "5xx wrapped as Auth",
    ),
    (
        "tokens.py",
        "if exc.status is not None and 400 <= exc.status < 500:",
        "if False:",
        "4xx not wrapped",
    ),
    ("tokens.py", '"grant_type": "client_credentials"', '"grant_type": "password"', "grant type"),
    ("tokens.py", "DEFAULT_LIFETIME = 300.0", "DEFAULT_LIFETIME = 3000.0", "default lifetime"),
    (
        "tokens.py",
        "float(expires) if expires > 0 else DEFAULT_LIFETIME",
        "float(expires)",
        "non-positive lifetime accepted",
    ),
    ("tokens.py", "self._token = token\n", "self._token = None\n", "token never cached"),
]
only = sys.argv[1:]
survivors = []
orig = {}
for f, a, b, label in M:
    p = SRC / ("auth/" + f if f == "tokens.py" else f)
    src = p.read_text()
    if a not in src:
        print("NOT FOUND:", label)
        survivors.append(label + " (pattern missing)")
        continue
    p.write_text(src.replace(a, b, 1))
    try:
        r = subprocess.run(
            [PY, "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=120,
            env={**__import__("os").environ, "PYTHONDONTWRITEBYTECODE": "1"},
        )
        killed = r.returncode != 0
    except subprocess.TimeoutExpired:
        killed = True
    finally:
        p.write_text(src)
    print(("KILLED  " if killed else "SURVIVED"), label)
    if not killed:
        survivors.append(label)
print(f"\n{len(M) - len(survivors)}/{len(M)} killed; survivors: {survivors}")
