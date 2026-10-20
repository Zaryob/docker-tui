# Release plan

Releases are implemented and validated in this order. Publication and Git tags
are separate from the implementation work.

## v0.6.2 — Copy, build and log correctness

- Confine extracted container archives to the selected destination, including
  existing and archive-created symlinks; preserve regular copies and renaming.
- Make timestamp changes presentation-only for build output so a build is never
  canceled or executed again by a display setting.
- Give log rows stable identities distinct from source identities, preserve
  selection during refresh and retention, report partial source failures, and
  keep detail-view navigation local.
- Gate: focused regressions, race tests, vet and build.

## v0.6.3 — Integration and delivery reliability

- Add opt-in real-daemon integration tests using isolated, automatically cleaned
  Docker fixtures; run them in Linux CI.
- Require the same validation before both normal CI and tagged releases.
- Cancel stalled SSH interactive handshakes without shortening the lifetime of
  an established terminal session.
- Update the README for six tabs, structured logs and actual test commands.
- Gate: full checks, real-daemon tests and workflow review.

## v0.7.0 — Compose project operations (release prepared)

- Discover Compose projects through the selected daemon's Docker CLI context.
- Run project `up`, `down` and rebuild (`up --build`) with streamed progress,
  cancellation and explicit confirmation for `down`.
- Preserve access to a project after `down`, allow opening a local Compose file,
  and explain when project files are unavailable locally.
- Gate: command/context tests, UI lifecycle tests and isolated Compose smoke test.

## v0.8.0 — Persistent list preferences

- Save each tab's filter, sort column and sort direction across sessions.
- Persist the stopped-container visibility preference.
- Use a versioned user configuration, atomic writes, validated fields and
  graceful handling of missing or malformed files.
- Provide an explicit way to reset saved list preferences.
- Gate: persistence round trips, malformed-config tests and UI behavior tests.

## v0.9.0 — Registry login (planned)

- Reuse Docker credential helpers and registry credentials for private pulls.
- Keep secrets out of UI status, logs and saved list preferences.
- Design login/logout and authentication-error handling before implementation.
- This release is reserved for later work; no login implementation is included
  in v0.6.2–v0.8.0.
