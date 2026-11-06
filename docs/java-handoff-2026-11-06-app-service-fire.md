# Java hand-off: an application looks up and fires its own scheduled jobs

Owner ruling 2026-11-06. Go commit: see `git log` on the branch that carries this file.

## Why

A Laravel app (RFP) firing a job by code got `permission required:
platform:messaging:scheduled-job:view`: it first calls
`GET /api/scheduled-jobs/by-code/{code}` (messaging view), then
`POST /api/scheduled-jobs/{id}/fire` (messaging fire). Both live only in the
broad messaging-admin roles, which an application service account must not hold.

## Ruling

An application's own service account, holding `platform:application-service`,
may look up and fire scheduled jobs that belong to its own application(s), and
nothing broader.

## New permission

`platform:application-service:scheduled-job:fire`

- Add to the seeded permission list (next to `...:scheduled-job:sync` and
  `...:scheduled-job-instance:write`) and to the `platform:application-service` role.
- The seeder never rewrites an existing role, so add a migration inserting it into
  `iam_role_permissions` for the role named `platform:application-service`
  (`ON CONFLICT DO NOTHING`; Go: migration `100_application_service_scheduled_job_fire.sql`,
  down deletes it).

## Endpoint rules

"Own application" = job has an `application_id` and the caller's
`applications` claim contains it or `all_applications` is set (the same rule as
reporting on instances).

1. `POST /api/scheduled-jobs/{id}/fire`
   - Caller needs `platform:messaging:scheduled-job:fire` OR the new permission, else 403 `PERMISSION_REQUIRED`.
   - Messaging fire holder: any job, unchanged.
   - New-permission-only holder: job must have an application the caller can access; otherwise
     **403 `APPLICATION_ACCESS_REQUIRED`** (other application's job, or a job with no application).
   - Missing job: 404. The per-client scope check (A2) still applies afterwards.
2. `GET /api/scheduled-jobs/{id}`
3. `GET /api/scheduled-jobs/by-code/{code}`
   - Caller needs messaging `...:scheduled-job:view` OR the new fire permission OR
     `platform:application-service:scheduled-job:sync`, else 403 `PERMISSION_REQUIRED`.
   - Messaging view holder: unchanged.
   - App-only holder: job outside its applications (or with no application) is **404**
     (same as a missing job, so other applications' jobs are not revealed).
   - Client access check on the job stays after that.

Not opened to app callers: list, instances listing, instance logs.

## Audit event

`ScheduledJobFiredManually` (`platform:admin:scheduled-job:fired-manually`): no wire
change; comment reworded to cover an application firing its own job. Principal is the SA.

## Tests to mirror

Auth unit: messaging fire; app fire + own app; app fire + other app (403); app fire +
job without application (403); `all_applications` claim; lookup with fire perm, with sync
perm, other app / no app (not visible); neither permission (403).
HTTP/integration: fire and by-code (and by-id) for own app (200), other app (fire 403,
lookup 404), no-app job, messaging holder unaffected, seeded role contains the permission.
