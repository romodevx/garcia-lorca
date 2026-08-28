/**
 * Disconnects this project's billing account when the budget is exceeded.
 *
 * WHY A KILL SWITCH AND NOT AN ALERT. A budget alert is an email. An email at
 * 3am does not stop a Pub/Sub retry storm or a runaway build, and the owner of
 * this project has already been through a bill that arrived silently — alerting
 * is precisely the control that failed. This one acts.
 *
 * WHAT HAPPENS WHEN IT FIRES. Every billable service in the project stops:
 * Cloud Run returns errors, images become unreachable. NOTHING IS DELETED, and
 * re-attaching the billing account in the console brings it all back. That is a
 * deliberate trade: a demo that is down and recoverable beats a bill that is not.
 *
 * It is a one-way door in the moment, so it is gated behind
 * var.enable_billing_killswitch and it only fires above 100% of the budget.
 */
const { CloudBillingClient } = require("@google-cloud/billing");

const billing = new CloudBillingClient();
const PROJECT_ID = process.env.TARGET_PROJECT_ID;

exports.stopBilling = async (cloudEvent) => {
  const raw = cloudEvent?.data?.message?.data;
  if (!raw) {
    console.log("no message payload; ignoring");
    return;
  }

  const budget = JSON.parse(Buffer.from(raw, "base64").toString());
  const cost = Number(budget.costAmount);
  const limit = Number(budget.budgetAmount);

  if (!Number.isFinite(cost) || !Number.isFinite(limit)) {
    console.log("payload without usable amounts; ignoring", budget);
    return;
  }

  // Budget notifications arrive on EVERY threshold crossing, including the
  // informational ones. Only the real ceiling triggers the cut.
  if (cost <= limit) {
    console.log(`spend ${cost} of ${limit}: under budget, nothing to do`);
    return;
  }

  const name = `projects/${PROJECT_ID}`;
  const [info] = await billing.getProjectBillingInfo({ name });

  if (!info.billingAccountName) {
    console.log(`${PROJECT_ID} already has no billing account; nothing to do`);
    return;
  }

  await billing.updateProjectBillingInfo({
    name,
    projectBillingInfo: { billingAccountName: "" },
  });

  // console.error so it lands as an error in Cloud Logging and is trivially
  // alertable — you want to find out from the logs, not from the silence.
  console.error(
    `BILLING DISABLED for ${PROJECT_ID}: spend ${cost} exceeded budget ${limit}. ` +
      `Re-attach the billing account in the console to restore service. Nothing was deleted.`,
  );
};
