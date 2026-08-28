# Pre-demo rehearsal — run 48 hours before the meeting

These are **manual** on purpose. Each one needs the real cloud, a real clock, or a
service that genuinely goes to sleep — none of which a CI test can provide.
Calling them "tests" and leaving them in `apps/web/**.test.ts` would mean arriving
at the meeting believing something was covered when nothing ever ran it.

The automated tests live next to the code and run on every commit. This file is
the other half.

---

## R1 — The image actually boots
```sh
docker build -f apps/web/Dockerfile -t gl-web:check .
docker run --rm -p 3000:8080 -e GATEWAY_URL=https://example.invalid gl-web:check
curl -s -o /dev/null -w '%{http_code}\n' localhost:3000/healthz    # expect 200
```
Do this **before** touching Terraform. The build passing proves nothing about the
container starting.

## R2 — Terraform plans clean
```sh
make tf-plan
```
No `Cycle:` and no precondition failures.

## R3 — The web revision goes healthy
After `make tf-apply`, the `web` service must reach Ready. The shared module
probes `/healthz` five times, five seconds apart; a service that never answers is
killed with no application error to read.

## R4 — The school is really in there
```sh
API_URL=$(terraform -chdir=deploy/terraform output -raw web_url) node scripts/seed-demo.mjs
```
Then open the panel and confirm: ~400 pupils, 20 teachers, 4 levels, uniform
inventory with stock.

## R5 — THE SESSION-SURVIVAL REHEARSAL ⭐

**The single most important line in this file.**

1. Log in as the owner.
2. Wait **more than 20 minutes** without touching anything — long enough for the
   services to scale to zero.
3. Reload the page.
4. **You must still be logged in.**

If you are thrown back to the login screen, `IAM_JWT_SECRET` is not reaching the
`iam` service and it minted a new signing key when it woke up. That is the failure
that ends a demo mid-sentence, in front of the customer, with nothing on screen
explaining it. Everything else on this list is recoverable in the moment; this one
is not.

## R6 — Know your cold-start number
```sh
make cloud-cool
# wait ~15 minutes, then:
time curl -s -o /dev/null "$(terraform -chdir=deploy/terraform output -raw web_url)/login"
make cloud-warm
time curl -s -o /dev/null "$(terraform -chdir=deploy/terraform output -raw web_url)/login"
```
Write both numbers down. Cold is what a visitor gets on a random Tuesday; warm is
what the meeting gets. Knowing the gap is how you decide when to run `cloud-warm`.

## R7 — No dead ends in the menu
Click every tile the owner can see. Every one must open something real. Nothing
should land on a "coming soon" placeholder.

## R8 — It really did go back to zero
24 hours after the rehearsal:
```sh
make cloud-status                 # every MIN empty or 0
```
Then check the billing report shows no Cloud Run CPU for the idle period. If it
does, something is warm that should not be.

---

## On the day

```sh
make cloud-warm      # 15 minutes before
```
Click through each module once to warm the rest, then leave it alone.

```sh
make cloud-cool      # right after the meeting
```
If you forget, the next `terraform apply` undoes it for you — but do not rely on
that.
