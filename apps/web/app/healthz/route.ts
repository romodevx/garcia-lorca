/**
 * Liveness endpoint for the Cloud Run startup probe.
 *
 * deploy/terraform/modules/cloud_run/main.tf hardcodes
 * `startup_probe { http_get { path = "/healthz" } }` for EVERY service. The nine
 * Go services answer it; without this route the web app returns 404, the probe
 * fails five times, and Cloud Run kills the container — a green build, a working
 * `docker run`, and a revision that never goes ready, with no application error
 * to read anywhere. See .claude/docs/PLAN-deploy-gcp-dev.md (E1).
 *
 * Deliberately self-contained: it reports that THIS process is serving, nothing
 * more. A probe that called the gateway or the database would turn one
 * downstream outage into a killed web container.
 */

// Never prerender: the probe must answer from the running process, not from a
// build-time artifact.
export const dynamic = "force-dynamic";

export async function GET(): Promise<Response> {
  return new Response("ok", {
    status: 200,
    headers: { "content-type": "text/plain; charset=utf-8", "cache-control": "no-store" },
  });
}
