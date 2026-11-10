---
id: rep-upzli
status: closed
deps: []
links: []
created: 2026-11-10T01:14:05Z
type: bug
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-659
---
# Fix Discord webhook delivery in Docker and widen admin settings inputs (STA-659)

Ready for implementation.

1. Dockerfile: the final stage is FROM scratch and has no CA certificates, so every HTTPS request (Discord webhooks) fails TLS verification. Copy /etc/ssl/certs/ca-certificates.crt from the build stage into the final image at the same path. No other image changes.

2. Test result reason: the admin test result says only 'request failed' for transport errors, which hides the cause. Show a safe reason, e.g. 'webhook #1: request failed: tls: failed to verify certificate: ...'. Use the inner error of *url.Error (it does not contain the URL). The webhook URL must still never appear in the page or the logs; keep a test that asserts this. Background-send logs should include the same safe reason.

3. Admin settings inputs: on /admin/settings, the forum name input and the Discord webhooks textarea must be full width of their panel. Today .account-form .inp caps inputs at 280px and the global textarea rule caps at 32rem. Scope the change to the admin settings page (add a modifier class to that form); do not change the account page.

Non-goals: no other UI changes.

