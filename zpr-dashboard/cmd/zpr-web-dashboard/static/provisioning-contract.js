const provisioningContract = (() => {
  const text = (value) => typeof value === "string" && value.length > 0 && value.length <= 1024;
  const instant = (value) => text(value) && Number.isFinite(Date.parse(value));
  const assetFields = Object.freeze(["organization", "asset_id", "name", "owner", "type", "profile", "recipient"]);
  const states = new Set(["invited", "pending_approval", "approved", "rejected", "cancelled", "expired", "approval_expired"]);

  function validInvitation(item, organization) {
    return item && text(item.id) && /^[A-Za-z0-9_-]{1,128}$/.test(item.id) &&
      item.asset && item.asset.organization === organization &&
      assetFields.every((key) => text(item.asset[key])) &&
      states.has(item.state) && Number.isSafeInteger(item.revision) && item.revision > 0 &&
      instant(item.created_at) && instant(item.expires_at) && text(item.created_by) &&
      (item.key_fingerprint === undefined || (text(item.key_fingerprint) && /^[a-f0-9]{64}$/.test(item.key_fingerprint))) &&
      ["claimed_at", "approval_expires_at", "decided_at"].every((key) => item[key] == null || instant(item[key])) &&
      (item.decision_by === undefined || text(item.decision_by)) &&
      (item.decision_reason === undefined || text(item.decision_reason)) &&
      !Object.hasOwn(item, "enrollment_code") &&
      (item.state !== "pending_approval" || Boolean(item.key_fingerprint && item.claimed_at && item.approval_expires_at));
  }

  return Object.freeze({ assetFields, validInvitation });
})();
