//! Trusted-attribute lifecycle for actors: refresh an actor's attributes from
//! the trusted services that vend them, persist the result, and only then
//! record the source revisions.
//!
//! This is application-service code, not part of `trusted_services`: it needs
//! both the [TrustedServicesMgr] and the [crate::actor_mgr::ActorMgr] (via
//! [Assembly]), and `trusted_services` is (for now...) deliberately kept free
//! of any knowledge of actors or the assembly. Used by the visa-request path
//! and by the reconciliation handlers in `event_mgr`.

use std::collections::{HashMap, HashSet};
use std::net::IpAddr;

use libeval::actor::{Actor, DeviceAuthority};
use libeval::attribute::Attribute;

use tracing::{debug, warn};

use crate::assembly::Assembly;
use crate::error::ServiceError;
use crate::logging::targets::VREQ;
use crate::trusted_services::{
    REVISION_NEVER, TrustedServicesMgr, derive_user_authority_attributes, lookup_identities,
    source_unqualified_key,
};

/// What a refresh pass found. Kept separate from the act of applying it so the caller
/// can persist the actor before any of it is committed -- see [refresh_and_persist_actor].
#[derive(Default)]
struct RefreshOutcome {
    /// The actor was modified and needs writing back.
    changed: bool,
    /// Source revisions to record once that write succeeds.
    revisions: Vec<(String, u64)>,
    /// Retired source records to forget only after pruning is persisted.
    removed_sources: HashSet<String>,
    /// Reasons lookup is indeterminate (unavailable sources or expired identities).
    /// The actor must not be evaluated, including conditions on absent cached keys.
    indeterminate: Vec<String>,
}

impl RefreshOutcome {
    /// Direct [TrustedServicesMgr] to record the pending source revisions against the
    /// actor's ZPR address. Deliberately not done during the refresh itself: a revision
    /// must never say "current" for an actor whose refreshed attributes did not reach
    /// the database, or the next request would trust the stale copy it loads.
    fn commit_revisions(&self, ts_mgr: &TrustedServicesMgr, zpr_addr: &IpAddr) {
        for source in &self.removed_sources {
            ts_mgr.forget_source_revision(zpr_addr, source);
        }
        for (source, revision) in &self.revisions {
            ts_mgr.record_revision(zpr_addr, source, *revision);
        }
    }
}

struct PendingActorWrite<'a> {
    manager: &'a TrustedServicesMgr,
    address: Option<IpAddr>,
}

impl Drop for PendingActorWrite<'_> {
    fn drop(&mut self) {
        if let Some(address) = self.address {
            self.manager.forget_actor_revisions(&address);
            warn!(target: VREQ, "actor {address} refresh dropped during persistence; invalidated source revisions; reload required");
        }
    }
}

/// Refresh the actor's attributes (see [refresh_expired_attributes]), persist it to the
/// store if anything changed, and only then record the source revisions. Used by the
/// request path and by the attribute-change reconciliation in `event_mgr`.
///
/// Returns TRUE if the actor was changed and written back.
/// Dropping an in-flight write invalidates this actor's cached source revisions;
/// it does not cancel a database mutation that has already been submitted.
///
/// ### Errors
/// - `StoreError` if the write back fails. No revision is recorded, so the next request
///   retries the whole refresh. The caller's actor may already be modified, and a
///   late repository failure may occur after refreshed claims have been stored;
///   this is not an atomic transaction across all actor records.
/// - `AttributesIndeterminate` if a lookup identity expired or a source could not
///   be reached. Revision-stale
///   attributes are stripped and persisted; expired TTL attributes are retained.
///   Callers must treat either result as a denial rather than evaluate it: absence is
///   not the same as "known to have no such attribute", and some policy conditions are
///   satisfied by a missing key.
/// - `AttributesIndeterminate` if a configuration change is observed during
///   refresh or persistence. Discard the working actor and retry from storage.
///   These optimistic checks do not serialize publication with database writes.
pub(crate) async fn refresh_and_persist_actor(
    asm: &Assembly,
    actor: &mut Actor,
) -> Result<bool, ServiceError> {
    let configuration = asm.ts_mgr.configuration();
    let policy = asm.policy_mgr.get_current();
    let outcome =
        refresh_expired_attributes(&asm.ts_mgr, &policy.lookup_identity_keys(), actor).await;
    let configuration_is_current = || {
        asm.ts_mgr.configuration_is_current(&configuration)
            && asm.policy_mgr.get_current().vinst() == policy.vinst()
    };
    if !configuration_is_current() {
        return Err(ServiceError::AttributesIndeterminate(
            "policy or trusted-source configuration changed during actor refresh; retry required"
                .into(),
        ));
    }
    let persisted = if outcome.changed {
        let mut pending = PendingActorWrite {
            manager: &asm.ts_mgr,
            address: actor.get_zpr_addr().copied(),
        };
        let result = asm.actor_mgr.update_actor(actor).await;
        pending.address = None;
        result
    } else {
        Ok(())
    };

    if !configuration_is_current() {
        if let Some(address) = actor.get_zpr_addr() {
            // A write may have completed during publication. Its claims must be
            // fetched again rather than certified by any previous revision.
            asm.ts_mgr.forget_actor_revisions(address);
        }
        persisted?;
        return Err(ServiceError::AttributesIndeterminate(
            "policy or trusted-source configuration changed during actor persistence; retry required".into(),
        ));
    }
    persisted?;

    // No ZPR address means refresh_expired_attributes did nothing at all (an actor
    // without an address has not completed a connection, so there is nothing to key
    // its revision records on).
    let Some(zpr_addr) = actor.get_zpr_addr().copied() else {
        return Ok(outcome.changed);
    };
    outcome.commit_revisions(&asm.ts_mgr, &zpr_addr);

    if !outcome.indeterminate.is_empty() {
        return Err(ServiceError::AttributesIndeterminate(format!(
            "actor {} attribute refresh indeterminate: {}",
            zpr_addr,
            outcome.indeterminate.join(", ")
        )));
    }
    Ok(outcome.changed)
}

/// Refresh the actor's attributes from the trusted service manager where
/// needed: any source with an expired attribute (TTL path), plus any source
/// whose snapshot revision differs from the actor's recorded one (revision
/// path, e.g. after an admin flush).
///
/// Claims owned by removed sources are pruned before lookup identity selection,
/// using persisted source-qualified keys and their same-source legacy aliases.
/// Removal is known absence, not an unavailable active source.
///
/// A successful lookup is authoritative for that source: anything the service
/// did not vend is dropped.
///
/// A failed TTL lookup retains expired attributes but reports the source as
/// indeterminate. The evaluator's expired-key guard does not protect conditions
/// on other keys that the unavailable source might now vend.
///
/// A failed lookup for a revision-stale source (ie, source reports a new
/// revision) is different: there is no expired copy to fall back on, so the
/// source's attributes are stripped and the source is reported as
/// *indeterminate*. The caller must deny rather than evaluate the remaining
/// claims -- absence is not "known to have no such attribute". Either failure
/// queues REVISION_NEVER so the next request retries even with an empty cache.
///
/// Nothing is recorded with the trusted-service manager here; see
/// [RefreshOutcome].
async fn refresh_expired_attributes(
    ts_mgr: &TrustedServicesMgr,
    lookup_keys: &[&str],
    actor: &mut Actor,
) -> RefreshOutcome {
    let mut outcome = RefreshOutcome::default();
    // Revision records are keyed on the ZPR address; an actor without one has not
    // completed a connection and cannot be refreshed.
    let Some(zpr_addr) = actor.get_zpr_addr().copied() else {
        return outcome;
    };
    // Signed BAS claims belong to the authenticated session, not a directory
    // provider snapshot. Retain their source only while that session is valid.
    let authentication_source = actor.device_authentication().and_then(|identity| {
        if let DeviceAuthority::AuthenticationService { service_id, .. } = identity.authority() {
            Some((service_id.clone(), identity.expires_at()))
        } else {
            None
        }
    });
    if let Some((source, expiration)) = &authentication_source {
        if *expiration <= std::time::SystemTime::now()
            || actor
                .attrs_iter()
                .any(|attr| attr.get_source() == source && attr.is_expired())
        {
            outcome
                .indeterminate
                .push(format!("authentication service '{source}' claims expired"));
            return outcome;
        }
    }

    // Qualified keys identify persisted source ownership even after restart.
    // Restrict alias removal to those keys: a source id can also be a built-in
    // claim issuer, so the source stamp alone is not sufficient ownership proof.
    let mut owned_keys: HashMap<String, HashSet<String>> = HashMap::new();
    for attr in actor.attrs_iter() {
        if let Some(key) = source_unqualified_key(attr) {
            let keys = owned_keys.entry(attr.get_source().to_string()).or_default();
            keys.insert(key.to_string());
            keys.insert(attr.get_key().to_string());
        }
    }
    outcome.removed_sources =
        ts_mgr.removed_sources_for_actor(&zpr_addr, owned_keys.keys().map(String::as_str));
    if let Some((source, _)) = &authentication_source {
        outcome.removed_sources.remove(source);
    }
    for source in &outcome.removed_sources {
        if let Some(keys) = owned_keys.get(source) {
            outcome.changed |=
                prune_from_source(actor, source, |attr| keys.contains(attr.get_key()));
        }
    }

    // The lookup-identity (key, value) set: the policy's lookup-identity keys
    // intersected with the actor's attributes. Every attribute on the actor is either
    // an authenticated claim or validated by a join policy, so this cannot include a
    // self-asserted identity. May legitimately be empty (e.g. an actor with no CN and
    // no policy-declared identity attribute); sources then match nothing.
    let identities = match lookup_identities(lookup_keys, actor.attrs_iter()) {
        Ok(identities) => identities,
        Err(error) => {
            warn!(target: VREQ, "attribute lookup rejected for actor {zpr_addr}: {error}");
            outcome.indeterminate.push(error.to_string());
            return outcome;
        }
    };

    let mut sources = HashSet::new();
    for attr in actor.attrs_iter() {
        if attr.is_expired() {
            sources.insert(attr.get_source().to_string());
        }
    }

    // Sources whose snapshot revision the actor has not caught up with, and the
    // revision to record once a refresh from them succeeds.
    let mut stale: HashMap<String, u64> = ts_mgr
        .stale_sources_for_actor(&zpr_addr)
        .into_iter()
        .collect();
    sources.extend(stale.keys().cloned());
    if let Some((source, _)) = &authentication_source {
        sources.remove(source);
        stale.remove(source);
    }

    for source in &sources {
        let stale_rev = stale.get(source);
        for ts_result in ts_mgr
            .get_attributes_from_source_for_actor(source, &identities)
            .await
        {
            match ts_result {
                Ok(ts_attrs) => {
                    // A source vending `user.*` attributes also installs the derived
                    // `user.zpr.authority = <source id>` (#324 follow-up), so the
                    // authority tracks the user record it vouches for: re-derived while
                    // the record is vended, pruned with the record when it disappears
                    // (the derived key is stamped with this source and, when absent
                    // from `returned`, falls to the pruning below like any other
                    // attribute the source stopped vending).
                    let ts_attrs = {
                        let mut ts_attrs = ts_attrs;
                        let authorities = derive_user_authority_attributes(source, &ts_attrs);
                        ts_attrs.extend(authorities);
                        ts_attrs
                    };
                    let returned: HashSet<String> =
                        ts_attrs.iter().map(|a| a.get_key().to_string()).collect();
                    for attr in ts_attrs {
                        if let Err(e) = actor.add_attribute(attr) {
                            warn!(target: VREQ, "failed to add attribute for actor {}: {}", zpr_addr, e);
                        } else {
                            outcome.changed = true;
                        }
                    }
                    // Both TTL and revision lookups replace the source observation,
                    // including fields whose old individual TTL has not elapsed.
                    outcome.changed |=
                        prune_from_source(actor, source, |a| !returned.contains(a.get_key()));
                    if let Some(&rev) = stale_rev {
                        outcome.revisions.push((source.clone(), rev));
                    }
                }
                Err(e) => {
                    warn!(target: VREQ, "ts service attr lookup failed for actor {}: {}", zpr_addr, e);
                    if stale_rev.is_some() {
                        // Stale-revision attributes must never satisfy an allow policy
                        // just because their TTL has not run out, so strip them -- but
                        // the strip alone is not fail-closed, hence the indeterminate
                        // report. The sentinel keeps the source stale so the next
                        // request retries (the strip would otherwise leave nothing
                        // marking it relevant).
                        outcome.changed |= prune_from_source(actor, source, |_| true);
                    }
                    outcome.revisions.push((source.clone(), REVISION_NEVER));
                    outcome.indeterminate.push(source.clone());
                }
            }
        }
    }
    outcome
}

/// Removes every attribute on `actor` from `source` that matches `pred`.
/// Returns TRUE if anything was removed.
fn prune_from_source(actor: &mut Actor, source: &str, pred: impl Fn(&Attribute) -> bool) -> bool {
    let stale: Vec<String> = actor
        .attrs_iter()
        .filter(|a| a.get_source() == source && pred(a))
        .map(|a| a.get_key().to_string())
        .collect();

    for key in &stale {
        debug!(target: VREQ, "dropping attribute '{key}' no longer vended by source '{source}'");
        actor.remove_attribute(key);
    }
    !stale.is_empty()
}

/// Refresh (and persist) the attributes of each actor in `zpr_addrs`. Only sources that
/// are TTL-expired or revision-stale are actually fetched.
///
/// A failure is logged and skipped rather than aborting the pass: that actor's recorded
/// revision stays mismatched, so its next visa request refreshes it again. This includes
/// an unreachable trusted service (`AttributesIndeterminate`).
///
/// Returns `(refreshed, unresolved, failed)` counts for logging.
///
/// TODO: Sequential, and unbounded in the size of the set. Fine while the only trusted
/// service is the in-memory file store; revisit once services are network calls and
/// actor/visa counts are large.
pub(crate) async fn refresh_actors(asm: &Assembly, zpr_addrs: HashSet<IpAddr>) -> (u32, u32, u32) {
    let (mut refreshed, mut unresolved, mut failed) = (0u32, 0u32, 0u32);
    // Sequential for now, join_all it if sweeps ever get slow.
    for zpr_addr in zpr_addrs {
        match asm.actor_mgr.get_actor_by_zpr_addr(&zpr_addr).await {
            Ok(Some(mut actor)) => match refresh_and_persist_actor(asm, &mut actor).await {
                Ok(true) => refreshed += 1,
                Ok(false) => {}
                Err(e) => {
                    warn!(target: VREQ, "attribute reconcile: failed to refresh actor {zpr_addr}: {e}");
                    failed += 1;
                }
            },
            Ok(None) => unresolved += 1,
            Err(e) => {
                warn!(target: VREQ, "attribute reconcile: failed to load actor {zpr_addr}: {e}");
                failed += 1;
            }
        }
    }
    (refreshed, unresolved, failed)
}

#[cfg(test)]
#[path = "actor_attributes_lifecycle_tests.rs"]
mod lifecycle_tests;

#[cfg(test)]
mod tests {
    use super::*;

    use crate::trusted_services::TrustedServiceInterface;
    use libeval::attribute::{AttributeSource, SOURCE_ZPR, key};
    use std::sync::Arc;
    use std::time::{Duration, SystemTime};

    /// A trusted service that records the lookup-identity sets it was asked about and
    /// returns one canned, freshly-expiring attribute. Its revision starts at 1 and
    /// bumps on every flush, like the real store.
    struct FakeTrustedService {
        calls: std::sync::Mutex<Vec<Vec<(String, String)>>>,
        revision: std::sync::atomic::AtomicU64,
    }

    const FAKE_SOURCE: &str = "fake";
    const FAKE_ATTR_KEY: &str = "user.dept";

    #[async_trait::async_trait]
    impl crate::trusted_services::TrustedServiceInterface for FakeTrustedService {
        async fn get_attributes_for_actor(
            &self,
            identities: &[(String, String)],
        ) -> Result<Vec<Attribute>, ServiceError> {
            self.calls.lock().unwrap().push(identities.to_vec());
            Ok(vec![
                AttributeSource::new(FAKE_SOURCE)
                    .builder(FAKE_ATTR_KEY)
                    .expires_in(Duration::from_secs(600))
                    .value("engineering"),
            ])
        }

        async fn flush(&self) -> Result<(), ServiceError> {
            self.revision
                .fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            Ok(())
        }

        fn current_revision(&self) -> u64 {
            self.revision.load(std::sync::atomic::Ordering::SeqCst)
        }

        fn get_source_id(&self) -> &str {
            FAKE_SOURCE
        }
    }

    /// A manager holding one FakeTrustedService, plus a handle on the fake for assertions.
    fn ts_mgr_with_fake() -> (TrustedServicesMgr, Arc<FakeTrustedService>) {
        let fake = Arc::new(FakeTrustedService {
            calls: std::sync::Mutex::new(Vec::new()),
            revision: std::sync::atomic::AtomicU64::new(1),
        });
        let mgr = TrustedServicesMgr::new();
        mgr.update_services(vec![fake.clone()]);
        (mgr, fake)
    }

    /// The ZPR address every test actor carries, and the calls made with it.
    const TEST_ADDR: &str = "fd5a:5052::42";

    /// Parse the shared test ZPR address.
    fn test_addr() -> IpAddr {
        TEST_ADDR.parse().unwrap()
    }

    /// Refresh an actor (with CN as the only lookup-identity key, matching the default
    /// builtin identity) and commit the resulting revisions, as the request path does
    /// once the actor is persisted. Returns whether the actor changed.
    async fn refresh_and_commit(mgr: &TrustedServicesMgr, actor: &mut Actor) -> bool {
        let outcome = refresh_expired_attributes(mgr, &[key::CN], actor).await;
        if let Some(zpr_addr) = actor.get_zpr_addr().copied() {
            outcome.commit_revisions(mgr, &zpr_addr);
        }
        outcome.changed
    }

    /// An actor with a CN, a ZPR address, and one attribute (expired or still fresh)
    /// from the given source.
    fn actor_with_attr(cn: &str, source: &str, expired: bool) -> Actor {
        let mut actor = Actor::new();
        actor
            .add_attribute(
                Attribute::builder(key::CN)
                    .expires_in(Duration::from_secs(600))
                    .value(cn),
            )
            .unwrap();
        actor
            .add_attribute(Attribute::builder(key::ZPR_ADDR).value(TEST_ADDR))
            .unwrap();
        let expires = if expired {
            SystemTime::now() - Duration::from_secs(1)
        } else {
            SystemTime::now() + Duration::from_secs(600)
        };
        actor
            .add_attribute(
                AttributeSource::new(source)
                    .builder(FAKE_ATTR_KEY)
                    .expires(expires)
                    .value("engineering"),
            )
            .unwrap();
        actor
    }

    /// An actor with a CN and a ZPR address, but no trusted-service attributes.
    fn actor_with_cn(cn: &str) -> Actor {
        let mut actor = Actor::new();
        actor
            .add_attribute(
                Attribute::builder(key::CN)
                    .expires_in(Duration::from_secs(600))
                    .value(cn),
            )
            .unwrap();
        actor
            .add_attribute(Attribute::builder(key::ZPR_ADDR).value(TEST_ADDR))
            .unwrap();
        actor
    }

    /// The (key, value) lookup set expected for a CN-only test actor.
    fn cn_ident(cn: &str) -> Vec<(String, String)> {
        vec![(key::CN.to_string(), cn.to_string())]
    }

    /// An expired attribute whose source is a registered trusted service is refreshed
    /// from that service, and only that service is queried.
    #[tokio::test]
    async fn test_refresh_expired_attributes_refreshes_from_source() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, true);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).unwrap().is_expired());

        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert!(!actor.get_attribute(FAKE_ATTR_KEY).unwrap().is_expired());
        assert_eq!(
            *fake.calls.lock().unwrap(),
            vec![cn_ident("someone.zpr.org")]
        );
    }

    /// The hot path: nothing expired and every source's revision already recorded means
    /// no trusted-service round trip at all.
    #[tokio::test]
    async fn test_refresh_expired_attributes_skips_when_nothing_expired() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_cn("someone.zpr.org");
        mgr.record_revision(&test_addr(), FAKE_SOURCE, fake.current_revision());

        assert!(!refresh_and_commit(&mgr, &mut actor).await);
        assert!(fake.calls.lock().unwrap().is_empty());
    }

    /// An actor holding no attribute from a configured source (the service vended
    /// nothing at connect time) is still refreshed from it, so attributes that show up
    /// later are picked up instead of being ignored forever.
    #[tokio::test]
    async fn test_refresh_queries_source_with_no_prior_attributes() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_cn("someone.zpr.org");

        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert_eq!(
            actor.get_attribute(FAKE_ATTR_KEY).unwrap().get_value(),
            ["engineering".to_string()]
        );
        assert_eq!(
            *fake.calls.lock().unwrap(),
            vec![cn_ident("someone.zpr.org")]
        );

        // The revision was recorded, so the next pass is a no-op.
        assert!(!refresh_and_commit(&mgr, &mut actor).await);
        assert_eq!(fake.calls.lock().unwrap().len(), 1);
    }

    /// #310 regression: a user-only actor (no CN at all, identified by a policy-declared
    /// identity attribute) still gets its attributes refreshed — the old code early-
    /// returned on `get_cn() == None`, silently never refreshing such actors.
    #[tokio::test]
    async fn test_refresh_works_for_actor_without_cn() {
        let (mgr, fake) = ts_mgr_with_fake();

        const USER_KEY: &str = "user.sub";
        let mut actor = Actor::new();
        actor
            .add_attribute(
                Attribute::builder(USER_KEY)
                    .expires_in(Duration::from_secs(600))
                    .value("google-sub-123"),
            )
            .unwrap();
        actor
            .add_attribute(Attribute::builder(key::ZPR_ADDR).value(TEST_ADDR))
            .unwrap();

        // Lookup keys include the builtin CN and the policy-declared user key; only the
        // latter is present on this actor, so it alone forms the lookup set.
        let outcome = refresh_expired_attributes(&mgr, &[key::CN, USER_KEY], &mut actor).await;
        assert!(outcome.changed);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_some());
        assert_eq!(
            *fake.calls.lock().unwrap(),
            vec![vec![(USER_KEY.to_string(), "google-sub-123".to_string())]]
        );

        // And the revisions commit against the address, so the refresh sticks.
        outcome.commit_revisions(&mgr, &test_addr());
        assert!(mgr.stale_sources_for_actor(&test_addr()).is_empty());
    }

    #[tokio::test]
    async fn refresh_does_not_forward_expired_subjects_alongside_live_identities() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_cn("device.example");
        actor
            .add_attribute(
                AttributeSource::new("former-issuer")
                    .builder("user.sub")
                    .expires(SystemTime::now() - Duration::from_secs(1))
                    .value("former-user"),
            )
            .unwrap();

        let outcome = refresh_expired_attributes(&mgr, &[key::CN, "user.sub"], &mut actor).await;
        assert!(!outcome.changed);
        assert_eq!(outcome.indeterminate.len(), 1);
        assert!(fake.calls.lock().unwrap().is_empty());
        assert!(actor.get_attribute("user.sub").unwrap().is_expired());
    }

    #[tokio::test]
    async fn empty_source_observation_expiry_forces_refresh_at_unchanged_revision() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_cn("device.example");
        mgr.record_revision(&test_addr(), FAKE_SOURCE, fake.current_revision());
        actor
            .add_attribute(
                AttributeSource::new(FAKE_SOURCE)
                    .builder("zpr.source_freshness__source__fake")
                    .expires(SystemTime::now() - Duration::from_secs(1))
                    .values(Vec::<String>::new()),
            )
            .unwrap();
        let outcome = refresh_expired_attributes(&mgr, &[key::CN], &mut actor).await;
        assert!(outcome.changed);
        assert!(outcome.indeterminate.is_empty());
        assert_eq!(
            *fake.calls.lock().unwrap(),
            vec![cn_ident("device.example")]
        );
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_some());
        assert!(
            actor
                .get_attribute("zpr.source_freshness__source__fake")
                .is_none()
        );
    }

    /// A trusted service that knows nothing about any actor: every lookup succeeds and
    /// returns no attributes.
    struct EmptyTrustedService;

    #[async_trait::async_trait]
    impl crate::trusted_services::TrustedServiceInterface for EmptyTrustedService {
        async fn get_attributes_for_actor(
            &self,
            _identities: &[(String, String)],
        ) -> Result<Vec<Attribute>, ServiceError> {
            Ok(Vec::new())
        }

        async fn flush(&self) -> Result<(), ServiceError> {
            Ok(())
        }

        // Holds no cache, so its data never changes: a constant revision is honest.
        fn current_revision(&self) -> u64 {
            1
        }

        fn get_source_id(&self) -> &str {
            FAKE_SOURCE
        }
    }

    /// A successful lookup is authoritative: an expired attribute the service no longer
    /// vends is dropped from the actor rather than kept in its expired state.
    #[tokio::test]
    async fn test_refresh_expired_attributes_prunes_unvended() {
        let mgr = TrustedServicesMgr::new();
        mgr.update_services(vec![Arc::new(EmptyTrustedService)]);
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, true);

        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_none());
        // The unexpired CN from another source is untouched.
        assert!(actor.get_attribute(key::CN).is_some());
    }

    #[tokio::test]
    async fn ttl_refresh_replaces_full_source_observation_not_just_expired_fields() {
        let mut message = capnp::message::Builder::new_default();
        {
            let mut policy = message.init_root::<zpr::policy::v1::policy::Builder>();
            let mut policies = policy.reborrow().init_com_policies(1);
            let mut com = policies.reborrow().get(0);
            com.set_allow(true);
            com.set_service_id("web");
            let mut scopes = com.reborrow().init_scope(1);
            let mut scope = scopes.reborrow().get(0);
            scope.set_protocol(6);
            scope.init_port().set_port_num(80);
            let mut conditions = com.reborrow().init_client_conds(1);
            let mut condition = conditions.reborrow().get(0);
            condition.set_key("user.badge");
            condition.set_op(zpr::policy::v1::AttrOp::Has);
        }
        let mut bytes = Vec::new();
        capnp::serialize::write_message(&mut bytes, &message).unwrap();
        let evaluator = libeval::eval::EvalContext::new(Arc::new(
            libeval::policy::Policy::new_from_policy_bytes(bytes::Bytes::from(bytes)).unwrap(),
        ));
        let packet =
            zpr::vsapi_types::PacketDesc::new_tcp(TEST_ADDR, "fd5a:5052::43", 1234, 80).unwrap();
        for empty_result in [true, false] {
            let (mgr, fake) = ts_mgr_with_fake();
            if empty_result {
                mgr.update_services(vec![Arc::new(EmptyTrustedService)]);
            }
            mgr.record_revision(&test_addr(), FAKE_SOURCE, 1);
            let mut actor = actor_with_cn("device.example");
            for (source, key, value) in [
                (FAKE_SOURCE, "user.badge", "old-permission"),
                (FAKE_SOURCE, key::USER_AUTHORITY, FAKE_SOURCE),
                ("other", "device.location", "hq"),
            ] {
                actor
                    .add_attribute(
                        AttributeSource::new(source)
                            .builder(key)
                            .expires_in(Duration::from_secs(600))
                            .value(value),
                    )
                    .unwrap();
            }
            actor.add_identity_key(usize::MAX, "user.badge").unwrap();
            actor
                .add_attribute(
                    AttributeSource::new(FAKE_SOURCE)
                        .builder("zpr.source_freshness__source__fake")
                        .expires(SystemTime::now() - Duration::from_secs(1))
                        .values(Vec::<String>::new()),
                )
                .unwrap();
            actor
                .add_attribute(Attribute::builder(key::SERVICES).value("web"))
                .unwrap();
            assert!(matches!(
                evaluator.eval_request(&actor, &actor, &packet).unwrap(),
                libeval::eval_result::PartialEvalResult::AllowWithoutRoute(_)
            ));
            let outcome = refresh_expired_attributes(&mgr, &[key::CN], &mut actor).await;
            assert!(outcome.changed);
            assert!(outcome.indeterminate.is_empty());
            assert!(actor.get_attribute("user.badge").is_none());
            assert!(!actor.identity_keys_iter().any(|key| key == "user.badge"));
            assert!(
                actor
                    .get_attribute("zpr.source_freshness__source__fake")
                    .is_none()
            );
            assert!(actor.get_attribute("device.location").is_some());
            assert!(actor.get_attribute(key::CN).is_some());
            assert!(matches!(
                evaluator.eval_request(&actor, &actor, &packet).unwrap(),
                libeval::eval_result::PartialEvalResult::Deny(_)
            ));
            assert_eq!(
                actor.get_attribute(key::USER_AUTHORITY).is_none(),
                empty_result
            );
            assert_eq!(actor.get_attribute(FAKE_ATTR_KEY).is_none(), empty_result);
            if !empty_result {
                assert_eq!(fake.calls.lock().unwrap().len(), 1);
            }
            outcome.commit_revisions(&mgr, &test_addr());
            assert!(mgr.stale_sources_for_actor(&test_addr()).is_empty());
        }
    }

    /// An unroutable source (no registered service) and an actor with no ZPR address
    /// must both fall through quietly rather than panic.
    #[tokio::test]
    async fn test_refresh_expired_attributes_handles_unrefreshable() {
        let (mgr, fake) = ts_mgr_with_fake();

        // SOURCE_ZPR is internal -- no trusted service vends it. The fake's revision is
        // recorded up front so it is not stale, isolating the unroutable-source path.
        let mut internal = actor_with_attr("someone.zpr.org", SOURCE_ZPR, true);
        mgr.record_revision(&test_addr(), FAKE_SOURCE, fake.current_revision());
        assert!(!refresh_and_commit(&mgr, &mut internal).await);
        assert!(fake.calls.lock().unwrap().is_empty());

        // No ZPR address means no revision key, so there is nothing to refresh.
        let mut no_addr = Actor::new();
        no_addr
            .add_attribute(
                AttributeSource::new(FAKE_SOURCE)
                    .builder(FAKE_ATTR_KEY)
                    .expires(SystemTime::now() - Duration::from_secs(1))
                    .value("engineering"),
            )
            .unwrap();
        assert!(!refresh_and_commit(&mgr, &mut no_addr).await);
        assert!(fake.calls.lock().unwrap().is_empty());
    }

    /// A revision-stale source (no record yet, or bumped by a flush) is refreshed even
    /// when none of its attributes have expired, and the recorded revision stops
    /// further refreshes until the next bump.
    #[tokio::test]
    async fn test_refresh_revision_stale_refreshes_and_records() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, false);

        // No record yet: stale, so the source is queried despite nothing being expired.
        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert_eq!(fake.calls.lock().unwrap().len(), 1);

        // Revision recorded: a second pass is a no-op.
        assert!(!refresh_and_commit(&mgr, &mut actor).await);
        assert_eq!(fake.calls.lock().unwrap().len(), 1);

        // A flush bumps the revision: stale again.
        fake.flush().await.unwrap();
        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert_eq!(fake.calls.lock().unwrap().len(), 2);
    }

    /// A revision-triggered refresh that returns zero attributes prunes everything the
    /// source previously vended — even attributes that had not expired — and records
    /// the revision so there is no further churn.
    #[tokio::test]
    async fn test_refresh_revision_stale_zero_attrs_prunes_all() {
        let mgr = TrustedServicesMgr::new();
        mgr.update_services(vec![Arc::new(EmptyTrustedService)]);
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, false);
        assert!(!actor.get_attribute(FAKE_ATTR_KEY).unwrap().is_expired());

        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_none());
        assert!(actor.get_attribute(key::CN).is_some());

        // The zero-attribute response was recorded: no further refresh churn.
        assert!(!refresh_and_commit(&mgr, &mut actor).await);
    }

    /// A trusted service whose lookups always fail, standing in for an outage.
    struct FailingTrustedService {
        calls: std::sync::atomic::AtomicUsize,
    }

    #[async_trait::async_trait]
    impl crate::trusted_services::TrustedServiceInterface for FailingTrustedService {
        async fn get_attributes_for_actor(
            &self,
            _identities: &[(String, String)],
        ) -> Result<Vec<Attribute>, ServiceError> {
            self.calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            Err(ServiceError::Internal("service is down".to_string()))
        }

        async fn flush(&self) -> Result<(), ServiceError> {
            Ok(())
        }

        // Never flushed in these tests; a constant revision is enough.
        fn current_revision(&self) -> u64 {
            1
        }

        fn get_source_id(&self) -> &str {
            FAKE_SOURCE
        }
    }

    /// A failed lookup for a revision-stale source fails closed: the source's
    /// attributes are stripped even though unexpired, and the source stays stale so
    /// the next request retries.
    #[tokio::test]
    async fn test_refresh_revision_stale_failure_strips_source() {
        let mgr = TrustedServicesMgr::new();
        let failing = Arc::new(FailingTrustedService {
            calls: std::sync::atomic::AtomicUsize::new(0),
        });
        mgr.update_services(vec![failing.clone()]);
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, false);

        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_none());
        assert!(actor.get_attribute(key::CN).is_some());

        // No revision was recorded on failure, so the next request retries the source
        // (nothing left to strip, so no change is reported).
        assert!(!refresh_and_commit(&mgr, &mut actor).await);
        assert_eq!(failing.calls.load(std::sync::atomic::Ordering::SeqCst), 2);
    }

    /// TTL failures retain the expired cache but cannot establish absence of other keys.
    #[tokio::test]
    async fn test_refresh_ttl_failure_retains_attributes() {
        let mgr = TrustedServicesMgr::new();
        let failing = Arc::new(FailingTrustedService {
            calls: std::sync::atomic::AtomicUsize::new(0),
        });
        mgr.update_services(vec![failing.clone()]);
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, true);

        // Mark the actor current for this source so only the TTL path triggers.
        mgr.record_revision(&test_addr(), FAKE_SOURCE, 1);

        let outcome = refresh_expired_attributes(&mgr, &[key::CN], &mut actor).await;
        assert!(!outcome.changed);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).unwrap().is_expired());
        assert_eq!(failing.calls.load(std::sync::atomic::Ordering::SeqCst), 1);
        assert_eq!(outcome.indeterminate, vec![FAKE_SOURCE.to_string()]);
        assert_eq!(
            outcome.revisions,
            vec![(FAKE_SOURCE.to_string(), REVISION_NEVER)]
        );
        outcome.commit_revisions(&mgr, &test_addr());
        assert_eq!(
            mgr.stale_sources_for_actor(&test_addr()),
            vec![(FAKE_SOURCE.to_string(), 1)]
        );
    }

    /// TS-1: a revision-stale source that cannot be reached is reported as indeterminate,
    /// so the request path denies instead of evaluating the stripped claim set. Stripping
    /// alone is not fail-closed -- a policy condition can be satisfied by a missing key.
    #[tokio::test]
    async fn test_refresh_revision_stale_failure_reports_indeterminate() {
        let mgr = TrustedServicesMgr::new();
        mgr.update_services(vec![Arc::new(FailingTrustedService {
            calls: std::sync::atomic::AtomicUsize::new(0),
        })]);
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, false);

        let outcome = refresh_expired_attributes(&mgr, &[key::CN], &mut actor).await;
        assert_eq!(outcome.indeterminate, vec![FAKE_SOURCE.to_string()]);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_none());
    }

    /// TS-2: a refresh does not record its revisions itself. The caller commits them only
    /// after the actor is persisted, so a failed write cannot leave a source marked
    /// current while the database still holds the stale attributes.
    #[tokio::test]
    async fn test_refresh_defers_revision_recording() {
        let (mgr, fake) = ts_mgr_with_fake();
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, false);

        let outcome = refresh_expired_attributes(&mgr, &[key::CN], &mut actor).await;
        assert_eq!(
            outcome.revisions,
            vec![(FAKE_SOURCE.to_string(), fake.current_revision())]
        );
        // Uncommitted: the source is still stale, so a dropped write costs a re-fetch
        // rather than a silently skipped one.
        assert!(!mgr.stale_sources_for_actor(&test_addr()).is_empty());

        outcome.commit_revisions(&mgr, &test_addr());
        assert!(mgr.stale_sources_for_actor(&test_addr()).is_empty());
    }

    /// #324 follow-up: the refresh path derives `user.zpr.authority` the same way the
    /// connect path does -- a source vending `user.*` attributes (the fake vends
    /// `user.dept`) installs the authority on refresh, so a user record that first
    /// appears post-connect still gets its authority.
    #[tokio::test]
    async fn test_refresh_installs_user_authority_with_user_attrs() {
        let (mgr, _fake) = ts_mgr_with_fake();
        let mut actor = actor_with_cn("someone.zpr.org");

        assert!(refresh_and_commit(&mgr, &mut actor).await);
        let authority = actor
            .get_attribute(key::USER_AUTHORITY)
            .expect("refresh should install user.zpr.authority");
        assert_eq!(authority.get_value(), [FAKE_SOURCE.to_string()]);
        assert_eq!(authority.get_source(), FAKE_SOURCE);
    }

    /// #324 follow-up: when a revision refresh stops vending the user record, the
    /// derived `user.zpr.authority` is pruned with it -- the authority tracks the
    /// attributes it vouches for.
    #[tokio::test]
    async fn test_refresh_prunes_user_authority_when_user_record_gone() {
        let mgr = TrustedServicesMgr::new();
        mgr.update_services(vec![Arc::new(EmptyTrustedService)]);
        // Actor connected while the source vended user attributes, so it carries both
        // the user attribute and the derived authority from that source.
        let mut actor = actor_with_attr("someone.zpr.org", FAKE_SOURCE, false);
        actor
            .add_attribute(
                AttributeSource::new(FAKE_SOURCE)
                    .builder(key::USER_AUTHORITY)
                    .expires_in(Duration::from_secs(600))
                    .value(FAKE_SOURCE),
            )
            .unwrap();

        // The (stale) source now vends nothing: user attribute AND authority go.
        assert!(refresh_and_commit(&mgr, &mut actor).await);
        assert!(actor.get_attribute(FAKE_ATTR_KEY).is_none());
        assert!(
            actor.get_attribute(key::USER_AUTHORITY).is_none(),
            "the derived authority must not outlive the user record it vouches for"
        );
    }
}
