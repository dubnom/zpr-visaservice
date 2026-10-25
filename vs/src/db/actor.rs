//! Redis/ValKey operations for actor state.
//!
//! Note that a ZADDR is a munged version of the ZPR address - colons replaced with dashes.
//! Note that the MUNGED_SERVICENAME is a db::KeyString - colons and '%' replaced with percent encoding.
//!
//! This updates:
//! - actor:<ZADDR>                - a hash for each connected actor
//! - actor:<ZADDR>:attrs          - a hash of attributes for each actor maps attribute keys to Attribug in JSON.
//! - actor:<ZADDR>:services       - a set of service names offered by the actor.
//! - service-providers:<MUNGED_SERVICENAME> - a set of ZPR addresses for actors providing the service.
//! - nodes                        - set of IP addresses  of all connected nodes.
//! - adapters                     - set of IP addresses  of all connected adapters.

use dashmap::DashMap;
use libeval::actor::{Actor, DeviceAuthentication};
use libeval::attribute::Attribute;
use libeval::attribute::key;
use libeval::pubkey::decode_public_key;
use std::collections::HashSet;
use std::net::IpAddr;
use std::sync::Arc;
use tracing::{debug, error, warn};
use zpr::vsapi_types::PublicKey;

use crate::db::{DbConnection, DbOp, HashGuard, KeyString, ZAddr, gen_timestamp};
use crate::error::StoreError;
use crate::logging::targets::DB;

const KEY_ACTOR: &str = "actor";
const KEY_SERVICE_PROVIDERS: &str = "service-providers";
const KEY_NODES: &str = "nodes";
const KEY_ADAPTERS: &str = "adapters";

pub enum Role {
    Node,
    Adapter,
}

pub struct ActorRepo {
    db: Arc<dyn DbConnection>,

    cn_idx: DashMap<String, IpAddr>, // CN -> ZPR_ADDRESS
}

/// Location of a service in the ZPRnet.
pub struct ServiceEntry {
    /// Name of service (sometimes called "id")
    pub name: String,
    pub zpr_addr: IpAddr,
}

impl ServiceEntry {
    pub fn new(name: String, zpr_addr: IpAddr) -> Self {
        ServiceEntry { name, zpr_addr }
    }
}

impl ActorRepo {
    pub fn new(db_handle: Arc<dyn DbConnection>) -> Self {
        ActorRepo {
            db: db_handle,
            cn_idx: DashMap::new(),
        }
    }

    /// Undo all the redis additions performed by `add_actor`.
    async fn clean_up(&self, zpraddr: &IpAddr, cn: Option<&str>) -> Result<(), StoreError> {
        //let mut vk_conn = self.db.conn.clone();

        let zpraddr_str = zpraddr.to_string();

        let base_key = actor_key_for(&zpraddr);
        let attrs_key = attrs_key_for(&zpraddr);
        let services_key = actor_services_key_for(&zpraddr);

        // Sanity check- remove any existing records for this actor.
        // Including any stale service records.

        let ops = vec![
            DbOp::Del(base_key.clone()),
            DbOp::Del(attrs_key.clone()),
            DbOp::SRem {
                set_key: KEY_NODES.into(),
                member: zpraddr_str.clone(),
            },
            DbOp::SRem {
                set_key: KEY_ADAPTERS.into(),
                member: zpraddr_str.clone(),
            },
        ];
        self.db.atomic_pipeline(&ops).await?;

        if self.db.exists(&services_key).await? {
            let service_names: HashSet<String> = self.db.smembers(&services_key).await?;
            if !service_names.is_empty() {
                let ops = service_names
                    .iter()
                    .map(|name| DbOp::SRem {
                        set_key: service_key_for(name),
                        member: zpraddr_str.clone(),
                    })
                    .collect::<Vec<_>>();
                self.db.atomic_pipeline(&ops).await?;
            }
            self.db.del(&services_key).await?;
        }

        if let Some(cn_val) = cn {
            self.cn_idx.remove(cn_val);
        }
        Ok(())
    }

    pub async fn add_actor(&self, actor: &Actor) -> Result<(), StoreError> {
        match self.try_add_actor(actor).await {
            Ok(_) => {
                self.add_to_cache(actor);
                Ok(())
            }
            Err(e) => {
                // Attempt to clean up after ourselves...
                warn!(target: DB, "add_actor failed, attempting cleanup");
                if let Some(zpraddr) = actor.get_zpr_addr() {
                    match self.clean_up(zpraddr, actor.get_cn()).await {
                        Ok(_) => (),
                        Err(cleanup_err) => {
                            error!(target: DB, "actor insert failed and so did clean up for addr={}: {}", zpraddr, cleanup_err);
                        }
                    }
                }
                Err(e)
            }
        }
    }

    /// List the ZPR addresses of all connected actors.
    pub async fn list_zpr_addrs(&self) -> Result<Vec<IpAddr>, StoreError> {
        let mut addrs = Vec::new();

        let node_addr_strs: HashSet<String> = self.db.smembers(KEY_NODES).await?;
        for addr_str in node_addr_strs {
            let addr: IpAddr = match addr_str.parse() {
                Ok(addr) => addr,
                Err(err) => {
                    warn!(target: DB, "invalid zpr address in set {}: {} ({})", KEY_NODES, addr_str, err);
                    continue;
                }
            };
            addrs.push(addr);
        }

        let adapter_addr_strs: HashSet<String> = self.db.smembers(KEY_ADAPTERS).await?;
        for addr_str in adapter_addr_strs {
            let addr: IpAddr = match addr_str.parse() {
                Ok(addr) => addr,
                Err(err) => {
                    warn!(target: DB, "invalid zpr address in set {}: {} ({})", KEY_ADAPTERS, addr_str, err);
                    continue;
                }
            };
            addrs.push(addr);
        }

        Ok(addrs)
    }

    /// Update existing actor data. The passed actor is authoritative: attributes it no
    /// longer carries are dropped from the store (an attribute refresh can remove
    /// attributes, and a stale copy left behind would resurface on the next load).
    /// Attributes, identity metadata and service registrations are replaced in one
    /// transaction guarded by the previously read service set and the actor's
    /// authentication record. A conflict requires discarding the stale actor.
    pub async fn update_actor(&self, actor: &Actor) -> Result<(), StoreError> {
        self.update_actor_guarded(actor, false).await
    }

    /// Only the successful authentication path may replace device identity.
    /// Compare against the stored record captured at the start of this update.
    pub async fn update_authenticated_actor(&self, actor: &Actor) -> Result<(), StoreError> {
        if actor.device_authentication().is_none() {
            return Err(StoreError::MissingRequired(
                "authenticated actor update requires device authentication".into(),
            ));
        }
        self.update_actor_guarded(actor, true).await
    }

    async fn update_actor_guarded(
        &self,
        actor: &Actor,
        replace_authentication: bool,
    ) -> Result<(), StoreError> {
        let zpraddr = match actor.get_zpr_addr() {
            Some(addr) => addr.clone(),
            None => {
                return Err(StoreError::MissingRequired(
                    "attempt to update actor with no ZPR address".into(),
                ));
            }
        };

        let zpraddr_str = zpraddr.to_string();
        let base_key = actor_key_for(&zpraddr);
        let attrs_key = attrs_key_for(&zpraddr);
        let services_key = actor_services_key_for(&zpraddr);

        if !self.db.exists(&base_key).await? {
            return Err(StoreError::NotFound(format!(
                "update called on non-existant actor: {zpraddr}"
            )));
        }

        let stored_authentication = self.db.hget(&base_key, "device_authentication").await?;
        let authentication_json = serde_json::to_string(&actor.device_authentication())?;
        if !replace_authentication
            && stored_authentication.as_deref().unwrap_or("null") != authentication_json
        {
            warn!(target: DB, "actor {zpraddr} authentication changed; discard stale actor and reload");
            return Err(StoreError::Conflict(format!(
                "actor {zpraddr} authentication changed; discard stale actor and reload"
            )));
        }

        let ts = gen_timestamp();

        // The actor is not allowed to change roles:
        if actor.is_node() {
            if !self.db.sismember(KEY_NODES, &zpraddr_str).await? {
                return Err(StoreError::InvalidData(format!(
                    "update fails since node actor not already in node role: {zpraddr}"
                )));
            }
        } else {
            if !self.db.sismember(KEY_ADAPTERS, &zpraddr_str).await? {
                return Err(StoreError::InvalidData(format!(
                    "update fails since adapter actor not already in adapter role: {zpraddr}"
                )));
            }
        }

        //
        // actor:<ZADDR>:attrs
        //                |- <key> -> JSON(<Attribute>)
        //
        // Write the attributes. We write out the attributes in JSON.
        // There may be a bunch of attributes so we take the time to set up a pipeline.
        // The hash is dropped first so removed attributes do not linger; the pipeline is
        // atomic, so no reader sees the actor without its attributes.
        let mut ops = vec![DbOp::Del(attrs_key.clone())];
        for attr in actor.attrs_iter() {
            ops.push(DbOp::HSet {
                hash_key: attrs_key.clone(),
                field: attr.get_key().to_string(),
                value: serde_json::to_string(attr)?,
            });
        }
        let identity_keys = actor
            .identity_keys_iter()
            .map(String::as_str)
            .collect::<Vec<_>>();
        // Identity keys must describe the same attribute image, even when a
        // caller is canceled after Redis applies the pipeline.
        ops.push(DbOp::HSet {
            hash_key: base_key.clone(),
            field: "identity_keys".into(),
            value: serde_json::to_string(&identity_keys)?,
        });
        ops.push(DbOp::HSet {
            hash_key: base_key.clone(),
            field: "device_authentication".into(),
            value: authentication_json,
        });
        ops.push(DbOp::HSet {
            hash_key: base_key.clone(),
            field: "utime".into(),
            value: ts,
        });
        //
        // service-providers:<NAME> -> SET[ <zpr_addr> ]
        //
        // actor:<ZADDR>:services -> SET[ <service_name> ]
        //

        // Remove only this actor's previous registrations.
        let existing_services: HashSet<String> = self.db.smembers(&services_key).await?;
        for service_name in &existing_services {
            ops.push(DbOp::SRem {
                set_key: service_key_for(service_name),
                member: zpraddr_str.clone(),
            });
        }
        ops.push(DbOp::Del(services_key.clone()));

        // Add services back based on what actor actually still has.
        for service_name in actor.services_iter() {
            debug!(target: DB, "adding service for actor: addr={zpraddr} service={service_name}");
            let svc_key_str = service_key_for(&service_name);
            ops.push(DbOp::SAdd {
                set_key: svc_key_str,
                member: zpraddr_str.clone(),
            });
            ops.push(DbOp::SAdd {
                set_key: services_key.clone(),
                member: service_name.to_string(),
            });
        }
        if !self
            .db
            .atomic_pipeline_if_set_and_hash_match(
                &services_key,
                &existing_services,
                Some(HashGuard {
                    key: &base_key,
                    field: "device_authentication",
                    expected: stored_authentication.as_deref(),
                }),
                &ops,
            )
            .await?
        {
            warn!(target: DB, "actor {zpraddr} authentication or services changed during update; discard stale actor and reload");
            return Err(StoreError::Conflict(format!(
                "actor {zpraddr} authentication or services changed during update; discard stale actor and reload"
            )));
        }

        debug!(target: DB, "update actor in DB: addr={zpraddr} cn={:?} node?={}", actor.get_cn(), actor.is_node());
        Ok(())
    }

    /// Update our in-memory cache when we add an actor.
    pub fn add_to_cache(&self, actor: &Actor) {
        if let Some(cn_val) = actor.get_cn() {
            if let Some(zpr_addr) = actor.get_zpr_addr() {
                self.cn_idx.insert(cn_val.to_string(), zpr_addr.clone());
            }
        }
    }

    /// Get a list of connected service providers, one entry per actor address.
    pub async fn list_services(&self) -> Result<Vec<ServiceEntry>, StoreError> {
        let mut service_entries = Vec::new();
        let svc_keys = self
            .db
            .scan_match_all(format!("{KEY_SERVICE_PROVIDERS}:*"))
            .await?;
        for svc_key in svc_keys {
            let munged_svc_name = KeyString::from_raw(
                svc_key
                    .trim_start_matches(&format!("{KEY_SERVICE_PROVIDERS}:"))
                    .into(),
            );
            let svc_name = String::try_from(munged_svc_name).map_err(|_| {
                StoreError::InvalidData(format!("invalid service name encoding for key {svc_key}"))
            })?;
            for addr_str in self.db.smembers(&svc_key).await? {
                let addr = addr_str.parse().map_err(|e| {
                    StoreError::InvalidData(format!(
                        "invalid zpr_addr in service entry {}: {}",
                        svc_key, e
                    ))
                })?;
                service_entries.push(ServiceEntry::new(svc_name.clone(), addr));
            }
        }
        service_entries
            .sort_by(|left, right| (&left.name, left.zpr_addr).cmp(&(&right.name, right.zpr_addr)));
        Ok(service_entries)
    }

    /// Return all provider addresses registered for a service ID in stable order.
    pub async fn get_zpr_addrs_for_service(
        &self,
        service_name: &str,
    ) -> Result<Vec<IpAddr>, StoreError> {
        let svc_key = service_key_for(service_name);
        let mut addrs = self
            .db
            .smembers(&svc_key)
            .await?
            .into_iter()
            .map(|addr_str| {
                addr_str.parse::<IpAddr>().map_err(|e| {
                    StoreError::InvalidData(format!(
                        "invalid zpr_addr in service entry {}: {}",
                        svc_key, e
                    ))
                })
            })
            .collect::<Result<Vec<_>, _>>()?;
        addrs.sort();
        Ok(addrs)
    }

    /// Return the first provider for callers that still require a singular result.
    pub async fn get_zpr_addr_for_service(
        &self,
        service_name: &str,
    ) -> Result<Option<IpAddr>, StoreError> {
        Ok(self
            .get_zpr_addrs_for_service(service_name)
            .await?
            .into_iter()
            .next())
    }

    /// Load specific attributes by name. Missing attributes are omitted.
    pub async fn get_actor_attrs(
        &self,
        zpr_addr: &IpAddr,
        attr_keys: &[&str],
    ) -> Result<Vec<Attribute>, StoreError> {
        let attrs_key = attrs_key_for(zpr_addr);
        let mut attrs = Vec::new();
        for key in attr_keys {
            if let Some(attr_json) = self.db.hget(&attrs_key, key).await? {
                let attr: Attribute = serde_json::from_str(&attr_json)?;
                attrs.push(attr);
            }
        }
        Ok(attrs)
    }

    /// Load and decode the actor's A2A DH public key, returning None if absent.
    pub async fn get_a2a_dh_pubkey_by_zpr_addr(
        &self,
        zpr_addr: &IpAddr,
    ) -> Result<Option<PublicKey>, StoreError> {
        let attrs = self
            .get_actor_attrs(zpr_addr, &[key::A2A_DH_PUBKEY])
            .await?;
        let Some(attr) = attrs.first() else {
            return Ok(None);
        };
        let value = attr.get_single_value().map_err(|e| {
            StoreError::InvalidData(format!(
                "invalid {} for actor {zpr_addr}: {e}",
                key::A2A_DH_PUBKEY
            ))
        })?;
        let pubkey = decode_public_key(value).map_err(|e| {
            StoreError::InvalidData(format!(
                "invalid {} for actor {zpr_addr}: {e}",
                key::A2A_DH_PUBKEY
            ))
        })?;
        Ok(Some(pubkey))
    }

    /// Get a list of services offered by the actor.
    pub async fn list_services_for_actor(
        &self,
        zpr_addr: &IpAddr,
    ) -> Result<Vec<String>, StoreError> {
        let services_key = actor_services_key_for(zpr_addr);
        let service_names: HashSet<String> = self.db.smembers(&services_key).await?;
        Ok(service_names.into_iter().collect())
    }

    /// Add an actor record which must only be called after initial authentication (there
    /// will likely be changes to an actor later from trusted services or re-authentication,
    /// but the updates should use a different function.)
    async fn try_add_actor(&self, actor: &Actor) -> Result<(), StoreError> {
        let zpraddr = match actor.get_zpr_addr() {
            Some(addr) => addr.clone(),
            None => {
                return Err(StoreError::MissingRequired(
                    "attempt to add actor with no ZPR address".into(),
                ));
            }
        };

        self.clean_up(&zpraddr, actor.get_cn()).await?;

        let zpraddr_str = zpraddr.to_string();
        let base_key = actor_key_for(&zpraddr);
        let attrs_key = attrs_key_for(&zpraddr);
        let services_key = actor_services_key_for(&zpraddr);

        let ts = gen_timestamp();

        //
        // actor:<ZADDR>:attrs
        //                |- <key> -> JSON(<Attribute>)
        //
        // Write the attributes. We write out the attributes in JSON.
        let mut ops = Vec::new();
        for attr in actor.attrs_iter() {
            ops.push(DbOp::HSet {
                hash_key: attrs_key.clone(),
                field: attr.get_key().into(),
                value: serde_json::to_string(&attr)?,
            });
        }

        //
        // actor:<ZADDR>
        //         |- identity_keys -> JSON(<IdentityKeysVec>)
        //         |- device_authentication -> JSON(<Option<DeviceAuthentication>>)
        //         |- ctime -> string
        //         |- utime -> string
        //

        // Get the identity keys as a vec, write as JSON array
        let identity_keys = actor
            .identity_keys_iter()
            .map(String::as_str)
            .collect::<Vec<_>>();
        ops.push(DbOp::HSet {
            hash_key: base_key.clone(),
            field: "identity_keys".into(),
            value: serde_json::to_string(&identity_keys)?,
        });
        ops.push(DbOp::HSet {
            hash_key: base_key.clone(),
            field: "device_authentication".into(),
            value: serde_json::to_string(&actor.device_authentication())?,
        });
        self.db.atomic_pipeline(&ops).await?;
        self.db.hset_nx(&base_key, "ctime", &ts).await?; // set create time only if not already there.
        self.db.hset(&base_key, "utime", &ts).await?; // always set update time

        //
        // service-providers:<NAME> -> SET[ <zpr_addr> ]
        //
        // actor:<ZADDR>:services -> SET[ <service_name> ]
        //

        let existing_services: HashSet<String> = self.db.smembers(&services_key).await?;
        let remove_ops = existing_services
            .iter()
            .map(|service_name| DbOp::SRem {
                set_key: service_key_for(service_name),
                member: zpraddr.to_string(),
            })
            .collect::<Vec<_>>();
        if !remove_ops.is_empty() {
            self.db.atomic_pipeline(&remove_ops).await?;
        }
        self.db.del(&services_key).await?;

        for service_name in actor.services_iter() {
            debug!(target: DB, "adding service for actor: addr={zpraddr} service={service_name}");
            let svc_key_str = service_key_for(&service_name);
            self.db.sadd(&svc_key_str, &zpraddr.to_string()).await?;
            self.db.sadd(&services_key, &service_name).await?;
        }

        //
        // One of:
        //    nodes    -> SET [ <zpr_address_string> ]
        //    adapters -> SET [ <zpr_address_string> ]
        //
        if actor.is_node() {
            self.db.sadd(KEY_NODES, &zpraddr_str).await?;
        } else {
            self.db.sadd(KEY_ADAPTERS, &zpraddr_str).await?;
        }

        debug!(target: DB, "added actor to DB: addr={zpraddr} cn={:?} node?={}", actor.get_cn(), actor.is_node());
        Ok(())
    }

    /// Remove actor from the state database, including all services.
    pub async fn rm_actor_by_zpr_addr(&self, zpra: &std::net::IpAddr) -> Result<(), StoreError> {
        let opt_cn = match self.get_actor_attrs(zpra, &[key::CN]).await {
            Ok(attr_list) => {
                if attr_list.is_empty() {
                    None
                } else {
                    if let Ok(cn_val) = attr_list[0].get_single_value() {
                        Some(cn_val.to_string())
                    } else {
                        None
                    }
                }
            }
            Err(_) => None,
        };

        self.clean_up(zpra, opt_cn.as_deref()).await?;
        debug!(target: DB, "removed actor from DB: addr={zpra}");
        Ok(())
    }

    /// Look up actor by ZPR address. Creates a new actor instance from the DB data if found.
    /// Attributes and identity metadata are read from one database snapshot.
    /// The snapshot can become stale immediately after that read.
    ///
    /// ## Errors
    /// - Returns `StoreError::NotFound` if no actor found for the given ZPR address.
    pub async fn get_actor_by_zpr_addr(
        &self,
        zpra: &std::net::IpAddr,
    ) -> Result<Actor, StoreError> {
        let base_key = actor_key_for(&zpra);
        let (metadata, attrs_map) = self
            .db
            .hash_pair_snapshot(&base_key, &attrs_key_for(zpra))
            .await?;
        if metadata.is_empty() {
            return Err(StoreError::NotFound(format!("actor not found: {}", zpra)));
        }

        let mut actor = Actor::new();

        // Load attributes from json.  The attributes are in 'actor:<ZADDR>:attrs' hash
        // each key is an attribute name, and the value is the JSON representation of the attribute.
        for (_key, attr_json) in attrs_map.iter() {
            let attr: Attribute = serde_json::from_str(attr_json)?;
            actor.add_attribute(attr)?;
        }

        // Then get the identity attribute key values.
        let identity_keys_json = metadata.get("identity_keys").ok_or_else(|| {
            StoreError::InvalidData(format!("actor {zpra} missing identity_keys"))
        })?;
        let identity_keys: Vec<String> = serde_json::from_str(identity_keys_json)?;
        for idkey in identity_keys.iter() {
            actor.add_identity_key(usize::MAX, idkey)?; // 0 means no expiration
        }
        if let Some(json) = metadata.get("device_authentication") {
            let identity: Option<DeviceAuthentication> = serde_json::from_str(json)?;
            if let Some(identity) = identity {
                actor.set_device_authentication(identity)?;
            }
        }
        Ok(actor)
    }

    /// Returns only the CN from a consistent metadata/attribute snapshot without
    /// decoding the full actor. Captured CNs may be stale after the read.
    ///
    /// Returns `StoreError::NotFound` if no actor exists for the address or the actor has no CN attribute.
    pub async fn get_cn_by_zpr_addr(&self, zpra: &IpAddr) -> Result<String, StoreError> {
        let base_key = actor_key_for(zpra);
        let (metadata, attrs) = self
            .db
            .hash_pair_snapshot(&base_key, &attrs_key_for(zpra))
            .await?;
        if metadata.is_empty() {
            return Err(StoreError::NotFound(format!("actor not found: {}", zpra)));
        }
        let cn_json = attrs
            .get(key::CN)
            .ok_or_else(|| StoreError::NotFound(format!("actor at {} has no CN", zpra)))?;
        let cn_attr: Attribute = serde_json::from_str(cn_json)?;
        match cn_attr.get_single_value() {
            Ok(cn_val) => Ok(cn_val.to_string()),
            Err(_) => Ok(cn_attr.get_value_as_string()),
        }
    }

    /// Look up actor by CN attribute, recovering from persisted actors if the cache is stale.
    ///
    /// ## Errors
    // - Returns `StoreError::NotFound` if no actor found for the given CN.
    pub async fn get_actor_by_cn(&self, cn: &str) -> Result<Actor, StoreError> {
        if let Some(addr) = self.cn_idx.get(cn).map(|entry| *entry) {
            match self.get_actor_by_zpr_addr(&addr).await {
                Ok(actor) if actor.get_cn() == Some(cn) => return Ok(actor),
                Ok(_) | Err(StoreError::NotFound(_)) => {
                    self.cn_idx.remove(cn);
                }
                Err(err) => return Err(err),
            }
        }

        for addr in self.list_zpr_addrs().await? {
            match self.get_actor_by_zpr_addr(&addr).await {
                Ok(actor) if actor.get_cn() == Some(cn) => {
                    self.cn_idx.insert(cn.to_string(), addr);
                    return Ok(actor);
                }
                Ok(_) | Err(StoreError::NotFound(_)) => {}
                Err(err) => return Err(err),
            }
        }
        Err(StoreError::NotFound(format!(
            "actor not found for CN: {}",
            cn
        )))
    }

    /// This uses our "nodes" and "adapters" sets to list the CN values of all connected actors.
    pub async fn list_actor_cns(&self, by_roles: Option<Role>) -> Result<Vec<String>, StoreError> {
        let mut cns = Vec::new();

        let set_keys = match by_roles {
            Some(Role::Node) => vec![KEY_NODES.to_string()],
            Some(Role::Adapter) => vec![KEY_ADAPTERS.to_string()],
            None => vec![KEY_NODES.to_string(), KEY_ADAPTERS.to_string()],
        };

        for set_key in set_keys {
            let addr_strs: HashSet<String> = self.db.smembers(&set_key).await?;
            for addr_str in addr_strs {
                // The sets hold IP addresses as strings (not munged addresses)
                let addr: IpAddr = match addr_str.parse() {
                    Ok(addr) => addr,
                    Err(err) => {
                        warn!(target: DB, "invalid zpr address in set {}: {} ({})", set_key, addr_str, err);
                        continue;
                    }
                };
                let a_key = attrs_key_for(&addr);

                // Pull the CN attribute out of the actor hash (which is JSON Attribute)
                if let Some(cn_attr_json) = self.db.hget(&a_key, key::CN).await? {
                    let cn_attr: Attribute = serde_json::from_str(&cn_attr_json)?;
                    match cn_attr.get_single_value() {
                        Ok(cn_val) => cns.push(cn_val.to_string()),
                        Err(_) => cns.push(cn_attr.get_value_as_string()),
                    }
                } else {
                    warn!(target: DB, "missing CN attribute for actor at key = {a_key}");
                }
            }
        }
        Ok(cns)
    }
}

/// returns 'actor:<ZADDR>'
fn actor_key_for(zpr_addr: &IpAddr) -> String {
    let zaddr: ZAddr = zpr_addr.into();
    format!("{KEY_ACTOR}:{zaddr}")
}

/// returns 'actor:<ZADDR>:attrs'
fn attrs_key_for(zpr_addr: &IpAddr) -> String {
    let zaddr: ZAddr = zpr_addr.into();
    format!("{KEY_ACTOR}:{zaddr}:attrs")
}

/// returns 'service:<MUNGED_SERVICENAME>'
fn service_key_for(service_name: &str) -> String {
    let svc_name_clean = KeyString::from(service_name);
    format!("{KEY_SERVICE_PROVIDERS}:{}", svc_name_clean.as_str())
}

/// returns 'actor:<ZADDR>:services'
fn actor_services_key_for(zpr_addr: &IpAddr) -> String {
    let zaddr: ZAddr = zpr_addr.into();
    format!("{KEY_ACTOR}:{zaddr}:services")
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::db::DbConnection;
    use crate::db::db_fake::FakeDb;
    use crate::test_helpers::{make_actor_defexp, make_actor_with_services_defexp};
    use libeval::attribute::{ROLE_ADAPTER, ROLE_NODE, key};

    fn authenticated_device(subject: &str) -> Actor {
        let mut actor =
            make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::71", &["directory"], subject);
        actor
            .set_device_authentication(
                DeviceAuthentication::new(
                    libeval::actor::DeviceAuthority::AuthenticationService {
                        service_id: "bas".into(),
                        issuer: "zpr/bas".into(),
                    },
                    subject.into(),
                    uuid::Uuid::new_v4().to_string(),
                    std::time::SystemTime::now() + std::time::Duration::from_secs(600),
                )
                .unwrap(),
            )
            .unwrap();
        actor
    }

    #[tokio::test]
    async fn session_guard_rejects_stale_actor_after_reauthentication_and_repo_restart() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let first = authenticated_device("same-device");
        let second = authenticated_device("same-device");
        let address = *first.get_zpr_addr().unwrap();
        repo.add_actor(&first).await.unwrap();
        let stale = repo.get_actor_by_zpr_addr(&address).await.unwrap();
        repo.update_authenticated_actor(&second).await.unwrap();
        let restored = ActorRepo::new(db.clone());
        let before = db.hgetall(actor_key_for(&address)).await.unwrap();
        assert!(matches!(
            restored.update_actor(&stale).await,
            Err(StoreError::Conflict(_))
        ));
        assert_eq!(db.hgetall(actor_key_for(&address)).await.unwrap(), before);
        assert_eq!(
            restored
                .get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication(),
            second.device_authentication()
        );
        restored.update_actor(&second).await.unwrap();
    }

    #[tokio::test]
    async fn session_guard_checks_authentication_at_atomic_write_boundary() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let first = authenticated_device("old-device");
        let second = authenticated_device("new-device");
        let address = *first.get_zpr_addr().unwrap();
        repo.add_actor(&first).await.unwrap();
        let gate = db.hold_pipelines_for_test().await;
        let update = tokio::task::unconstrained(repo.update_actor(&first));
        tokio::pin!(update);
        assert!(futures::poll!(&mut update).is_pending());
        // Model a replacement publication after the update's preflight reads.
        db.hset(
            &actor_key_for(&address),
            "device_authentication",
            &serde_json::to_string(&second.device_authentication()).unwrap(),
        )
        .await
        .unwrap();
        db.hset(&actor_key_for(&address), "utime", "replacement-time")
            .await
            .unwrap();
        let metadata = db.hgetall(actor_key_for(&address)).await.unwrap();
        let attrs = db.hgetall(attrs_key_for(&address)).await.unwrap();
        drop(gate);
        assert!(matches!(update.await, Err(StoreError::Conflict(_))));
        assert_eq!(db.hgetall(actor_key_for(&address)).await.unwrap(), metadata);
        assert_eq!(db.hgetall(attrs_key_for(&address)).await.unwrap(), attrs);
        assert_eq!(
            repo.list_services_for_actor(&address).await.unwrap(),
            ["directory"]
        );
    }

    #[tokio::test]
    async fn session_guard_prevents_resurrection_after_deletion_before_apply() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor = make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::71", &[], "legacy");
        let address = *actor.get_zpr_addr().unwrap();
        repo.add_actor(&actor).await.unwrap();
        let gate = db.hold_pipelines_for_test().await;
        let update = tokio::task::unconstrained(repo.update_actor(&actor));
        tokio::pin!(update);
        assert!(futures::poll!(&mut update).is_pending());
        db.del(&actor_key_for(&address)).await.unwrap();
        db.del(&attrs_key_for(&address)).await.unwrap();
        drop(gate);
        assert!(matches!(update.await, Err(StoreError::Conflict(_))));
        assert!(!db.exists(&actor_key_for(&address)).await.unwrap());
        assert!(!db.exists(&attrs_key_for(&address)).await.unwrap());
    }

    #[tokio::test]
    async fn session_guard_has_no_timestamp_write_after_atomic_reply() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor = authenticated_device("device");
        let address = *actor.get_zpr_addr().unwrap();
        repo.add_actor(&actor).await.unwrap();
        let gate = db.hold_pipeline_replies_for_test().await;
        let update = tokio::task::unconstrained(repo.update_actor(&actor));
        tokio::pin!(update);
        assert!(futures::poll!(&mut update).is_pending());
        db.del(&actor_key_for(&address)).await.unwrap();
        db.del(&attrs_key_for(&address)).await.unwrap();
        drop(gate);
        update.await.unwrap();
        assert!(!db.exists(&actor_key_for(&address)).await.unwrap());
        assert!(!db.exists(&attrs_key_for(&address)).await.unwrap());
    }

    #[tokio::test]
    async fn session_guard_legacy_refresh_cannot_remove_or_create_authentication() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let legacy = make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::71", &[], "legacy");
        let authenticated = authenticated_device("device");
        let address = *legacy.get_zpr_addr().unwrap();
        repo.add_actor(&legacy).await.unwrap();
        assert!(matches!(
            repo.update_actor(&authenticated).await,
            Err(StoreError::Conflict(_))
        ));
        repo.update_authenticated_actor(&authenticated)
            .await
            .unwrap();
        assert!(matches!(
            repo.update_actor(&legacy).await,
            Err(StoreError::Conflict(_))
        ));
        assert!(matches!(
            repo.update_authenticated_actor(&legacy).await,
            Err(StoreError::MissingRequired(_))
        ));
        assert_eq!(
            repo.get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication(),
            authenticated.device_authentication()
        );
    }

    #[tokio::test]
    async fn session_guard_legacy_missing_field_remains_refreshable() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor = make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::71", &[], "legacy");
        let address = *actor.get_zpr_addr().unwrap();
        repo.add_actor(&actor).await.unwrap();
        let base = actor_key_for(&address);
        let mut metadata = db.hgetall(base.clone()).await.unwrap();
        metadata.remove("device_authentication");
        metadata.insert("ctime".into(), "original-create-time".into());
        db.del(&base).await.unwrap();
        for (field, value) in metadata {
            db.hset(&base, &field, &value).await.unwrap();
        }
        repo.update_actor(&actor).await.unwrap();
        repo.update_actor(&actor).await.unwrap();
        assert_eq!(
            db.hget(&base, "ctime").await.unwrap().as_deref(),
            Some("original-create-time")
        );
        assert_eq!(
            db.hget(&base, "device_authentication")
                .await
                .unwrap()
                .as_deref(),
            Some("null")
        );
    }

    #[tokio::test]
    async fn session_guard_reauthentication_conflicts_with_concurrent_replacement() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let first = authenticated_device("device");
        let second = authenticated_device("device");
        let third = authenticated_device("device");
        let address = *first.get_zpr_addr().unwrap();
        repo.add_actor(&first).await.unwrap();
        let gate = db.hold_pipelines_for_test().await;
        let update = tokio::task::unconstrained(repo.update_authenticated_actor(&second));
        tokio::pin!(update);
        assert!(futures::poll!(&mut update).is_pending());
        db.hset(
            &actor_key_for(&address),
            "device_authentication",
            &serde_json::to_string(&third.device_authentication()).unwrap(),
        )
        .await
        .unwrap();
        drop(gate);
        assert!(matches!(update.await, Err(StoreError::Conflict(_))));
        assert_eq!(
            repo.get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication(),
            third.device_authentication()
        );
    }

    #[tokio::test]
    async fn device_authentication_survives_repository_restart_and_claim_replacement() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let mut actor = authenticated_device("verified-device");
        let address = *actor.get_zpr_addr().unwrap();
        let identity = actor.device_authentication().unwrap().clone();
        repo.add_actor(&actor).await.unwrap();
        actor
            .add_attribute(Attribute::builder(key::CN).value("enriched-cn"))
            .unwrap();
        actor
            .add_attribute(Attribute::builder("device_authentication").value("forged-root"))
            .unwrap();
        repo.update_actor(&actor).await.unwrap();

        let loaded = ActorRepo::new(db.clone())
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap();
        assert_eq!(loaded.device_authentication(), Some(&identity));
        assert_eq!(loaded.get_cn(), Some("enriched-cn"));
        assert!(
            loaded
                .attrs_iter()
                .all(|attr| attr.get_value_as_string() != identity.session_id())
        );
        let json = serde_json::to_value(&loaded).unwrap();
        assert!(json.get("device_authentication").is_none());
        assert!(!json.to_string().contains(identity.session_id()));
        assert_eq!(
            serde_json::from_str::<DeviceAuthentication>(
                &db.hget(&actor_key_for(&address), "device_authentication")
                    .await
                    .unwrap()
                    .unwrap()
            )
            .unwrap(),
            identity
        );
    }

    #[tokio::test]
    async fn device_authentication_missing_metadata_never_uses_claims_as_a_root() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor =
            make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::71", &[], "legacy-device");
        let address = *actor.get_zpr_addr().unwrap();
        repo.add_actor(&actor).await.unwrap();
        assert!(
            repo.get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication()
                .is_none()
        );
        let mut metadata = db.hgetall(actor_key_for(&address)).await.unwrap();
        metadata.remove("device_authentication");
        db.del(&actor_key_for(&address)).await.unwrap();
        for (field, value) in metadata {
            db.hset(&actor_key_for(&address), &field, &value)
                .await
                .unwrap();
        }
        assert!(
            repo.get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication()
                .is_none()
        );
    }

    #[tokio::test]
    async fn device_authentication_malformed_metadata_is_an_explicit_read_error() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor = authenticated_device("verified-device");
        let address = *actor.get_zpr_addr().unwrap();
        repo.add_actor(&actor).await.unwrap();
        for json in [
            "invalid json".to_string(),
            "{\"subject\":\"incomplete\"}".to_string(),
            {
                let mut value = serde_json::to_value(actor.device_authentication()).unwrap();
                value["subject"] = "".into();
                value.to_string()
            },
        ] {
            db.hset(&actor_key_for(&address), "device_authentication", &json)
                .await
                .unwrap();
            assert!(repo.get_actor_by_zpr_addr(&address).await.is_err());
        }
    }

    #[tokio::test]
    async fn device_authentication_replacement_and_removal_do_not_retain_old_session() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let first = authenticated_device("device-one");
        let second = authenticated_device("device-two");
        let address = *first.get_zpr_addr().unwrap();
        repo.add_actor(&first).await.unwrap();
        repo.add_actor(&second).await.unwrap();
        assert_eq!(
            repo.get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication(),
            second.device_authentication()
        );
        repo.rm_actor_by_zpr_addr(&address).await.unwrap();
        let legacy =
            make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::71", &[], "legacy-device");
        repo.add_actor(&legacy).await.unwrap();
        assert!(
            repo.get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication()
                .is_none()
        );
    }

    #[tokio::test]
    async fn device_authentication_rejected_update_preserves_prior_identity_and_claims() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let first = authenticated_device("device-one");
        let second = authenticated_device("device-two");
        let address = *first.get_zpr_addr().unwrap();
        repo.add_actor(&first).await.unwrap();
        db.set_set_ex_fault(crate::db::FaultMode::Reject);
        assert!(repo.update_authenticated_actor(&second).await.is_err());
        let loaded = repo.get_actor_by_zpr_addr(&address).await.unwrap();
        assert_eq!(loaded.get_cn(), first.get_cn());
        assert_eq!(
            loaded.device_authentication(),
            first.device_authentication()
        );
    }

    #[tokio::test]
    async fn device_authentication_snapshot_does_not_mix_sessions_during_update() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let first = authenticated_device("device-one");
        let second = authenticated_device("device-two");
        let address = *first.get_zpr_addr().unwrap();
        repo.add_actor(&first).await.unwrap();
        let gate = db.hold_hash_replies_for_test().await;
        let read = tokio::task::unconstrained(repo.get_actor_by_zpr_addr(&address));
        tokio::pin!(read);
        assert!(futures::poll!(&mut read).is_pending());
        repo.update_authenticated_actor(&second).await.unwrap();
        drop(gate);
        let captured = read.await.unwrap();
        assert_eq!(captured.get_cn(), first.get_cn());
        assert_eq!(
            captured.device_authentication(),
            first.device_authentication()
        );
        let current = repo.get_actor_by_zpr_addr(&address).await.unwrap();
        assert_eq!(current.get_cn(), second.get_cn());
        assert_eq!(
            current.device_authentication(),
            second.device_authentication()
        );
    }

    struct ServiceUpdateFixture {
        db: Arc<FakeDb>,
        repo: ActorRepo,
        address: IpAddr,
        peer: IpAddr,
        updated: Actor,
    }

    impl ServiceUpdateFixture {
        async fn new(services: &[&str]) -> Self {
            let db = Arc::new(FakeDb::new());
            let repo = ActorRepo::new(db.clone());
            let address: IpAddr = "fd5a:5052::51".parse().unwrap();
            let peer: IpAddr = "fd5a:5052::52".parse().unwrap();
            for (addr, cn) in [(address, "first"), (peer, "peer")] {
                repo.add_actor(&make_actor_with_services_defexp(
                    ROLE_ADAPTER,
                    &addr.to_string(),
                    &["shared:svc", "old%svc"],
                    cn,
                ))
                .await
                .unwrap();
            }
            let updated = make_actor_with_services_defexp(
                ROLE_ADAPTER,
                &address.to_string(),
                services,
                "first",
            );
            Self {
                db,
                repo,
                address,
                peer,
                updated,
            }
        }

        async fn assert_services(&self, expected: &[&str]) {
            let loaded = self
                .repo
                .get_actor_by_zpr_addr(&self.address)
                .await
                .unwrap();
            let actual: HashSet<_> = loaded.services_iter().map(str::to_string).collect();
            assert_eq!(actual, expected.iter().map(|s| s.to_string()).collect());
            let indexed: HashSet<_> = self
                .repo
                .list_services_for_actor(&self.address)
                .await
                .unwrap()
                .into_iter()
                .collect();
            assert_eq!(indexed, actual);
            for service in ["shared:svc", "old%svc", "new:svc"] {
                let mut providers = vec![];
                if expected.contains(&service) {
                    providers.push(self.address);
                }
                if service != "new:svc" {
                    providers.push(self.peer);
                }
                providers.sort();
                assert_eq!(
                    self.repo.get_zpr_addrs_for_service(service).await.unwrap(),
                    providers
                );
            }
        }
    }

    #[tokio::test]
    async fn concurrent_service_updates_reject_stale_indexes_across_repositories() {
        let f = ServiceUpdateFixture::new(&["new:svc"]).await;
        let second_repo = ActorRepo::new(f.db.clone());
        let second_actor = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            &f.address.to_string(),
            &["shared:svc"],
            "first",
        );
        let guard = f.db.hold_pipelines_for_test().await;
        let mut first = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        let mut second = Box::pin(tokio::task::unconstrained(
            second_repo.update_actor(&second_actor),
        ));
        assert!(futures::poll!(first.as_mut()).is_pending());
        assert!(futures::poll!(second.as_mut()).is_pending());
        drop(guard);
        first.await.unwrap();
        let error = second
            .await
            .expect_err("stale service-index snapshot must not commit");
        assert!(matches!(error, StoreError::Conflict(_)));
        assert!(error.to_string().contains("reload"));
        f.assert_services(&["new:svc"]).await;
        second_repo.update_actor(&second_actor).await.unwrap();
        f.assert_services(&["shared:svc"]).await;
    }

    #[tokio::test]
    async fn concurrent_service_removal_cannot_leave_new_provider_membership() {
        let f = ServiceUpdateFixture::new(&["new:svc"]).await;
        let empty =
            make_actor_with_services_defexp(ROLE_ADAPTER, &f.address.to_string(), &[], "first");
        let guard = f.db.hold_pipelines_for_test().await;
        let mut first = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        let mut second = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&empty)));
        assert!(futures::poll!(first.as_mut()).is_pending());
        assert!(futures::poll!(second.as_mut()).is_pending());
        drop(guard);
        first.await.unwrap();
        assert!(second.await.is_err());
        f.assert_services(&["new:svc"]).await;
        f.repo.update_actor(&empty).await.unwrap();
        f.assert_services(&[]).await;
    }

    #[tokio::test]
    async fn concurrent_updates_with_unchanged_service_set_remain_compatible() {
        let f = ServiceUpdateFixture::new(&["shared:svc", "old%svc"]).await;
        let guard = f.db.hold_pipelines_for_test().await;
        let mut first = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        let mut second = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        assert!(futures::poll!(first.as_mut()).is_pending());
        assert!(futures::poll!(second.as_mut()).is_pending());
        drop(guard);
        first.await.unwrap();
        second.await.unwrap();
        f.assert_services(&["shared:svc", "old%svc"]).await;
    }

    #[tokio::test]
    async fn cn_snapshot_survives_removal_after_capture() {
        let f = ServiceUpdateFixture::new(&[]).await;
        let guard = f.db.hold_hash_replies_for_test().await;
        let mut read = Box::pin(tokio::task::unconstrained(
            f.repo.get_cn_by_zpr_addr(&f.address),
        ));
        assert!(futures::poll!(read.as_mut()).is_pending());
        f.repo.rm_actor_by_zpr_addr(&f.address).await.unwrap();
        drop(guard);
        assert_eq!(read.await.unwrap(), "first");
        assert!(matches!(
            f.repo.get_cn_by_zpr_addr(&f.address).await,
            Err(StoreError::NotFound(_))
        ));
    }

    #[tokio::test]
    async fn cn_snapshot_does_not_switch_to_replacement_actor_after_capture() {
        let f = ServiceUpdateFixture::new(&[]).await;
        let guard = f.db.hold_hash_replies_for_test().await;
        let mut read = Box::pin(tokio::task::unconstrained(
            f.repo.get_cn_by_zpr_addr(&f.address),
        ));
        assert!(futures::poll!(read.as_mut()).is_pending());
        f.repo.rm_actor_by_zpr_addr(&f.address).await.unwrap();
        let replacement = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            &f.address.to_string(),
            &["new:svc"],
            "replacement",
        );
        f.repo.add_actor(&replacement).await.unwrap();
        drop(guard);
        assert_eq!(read.await.unwrap(), "first");
        assert_eq!(
            f.repo.get_cn_by_zpr_addr(&f.address).await.unwrap(),
            "replacement"
        );
    }

    #[tokio::test]
    async fn cn_snapshot_does_not_decode_unrelated_claims_or_identity_metadata() {
        let f = ServiceUpdateFixture::new(&[]).await;
        f.db.hset(&attrs_key_for(&f.address), "unrelated", "bad JSON")
            .await
            .unwrap();
        f.db.hset(&actor_key_for(&f.address), "identity_keys", "bad JSON")
            .await
            .unwrap();
        assert_eq!(
            f.repo.get_cn_by_zpr_addr(&f.address).await.unwrap(),
            "first"
        );
        assert!(f.repo.get_actor_by_zpr_addr(&f.address).await.is_err());
    }

    #[tokio::test]
    async fn cn_snapshot_does_not_accept_orphaned_attributes_without_actor_metadata() {
        let f = ServiceUpdateFixture::new(&[]).await;
        f.db.del(&actor_key_for(&f.address)).await.unwrap();
        assert!(matches!(
            f.repo.get_cn_by_zpr_addr(&f.address).await,
            Err(StoreError::NotFound(message)) if message.contains("actor not found")
        ));
    }

    #[tokio::test]
    async fn cn_snapshot_preserves_missing_cn_and_malformed_cn_errors() {
        let f = ServiceUpdateFixture::new(&[]).await;
        f.db.del(&attrs_key_for(&f.address)).await.unwrap();
        assert!(matches!(
            f.repo.get_cn_by_zpr_addr(&f.address).await,
            Err(StoreError::NotFound(message)) if message.contains("has no CN")
        ));
        f.db.hset(&attrs_key_for(&f.address), key::CN, "bad JSON")
            .await
            .unwrap();
        assert!(matches!(
            f.repo.get_cn_by_zpr_addr(&f.address).await,
            Err(StoreError::Serialization(_))
        ));
    }

    #[tokio::test]
    async fn actor_load_keeps_identity_metadata_with_its_attribute_snapshot() {
        let f = ServiceUpdateFixture::new(&["new:svc"]).await;
        let mut updated = f.updated.clone();
        updated
            .add_attribute(Attribute::builder("device.snapshot").value("new"))
            .unwrap();
        updated
            .add_identity_key(usize::MAX, "device.snapshot")
            .unwrap();
        let guard = f.db.hold_hash_replies_for_test().await;
        let mut read = Box::pin(tokio::task::unconstrained(
            f.repo.get_actor_by_zpr_addr(&f.address),
        ));
        assert!(futures::poll!(read.as_mut()).is_pending());
        f.repo.update_actor(&updated).await.unwrap();
        drop(guard);
        let old = read
            .await
            .expect("an intervening update must not mix identity metadata");
        assert!(!old.provides("new:svc"));
        assert!(old.get_attribute("device.snapshot").is_none());
        assert!(!old.identity_keys_iter().any(|key| key == "device.snapshot"));
        let new = f.repo.get_actor_by_zpr_addr(&f.address).await.unwrap();
        assert!(new.provides("new:svc"));
        assert!(new.identity_keys_iter().any(|key| key == "device.snapshot"));
    }

    #[tokio::test]
    async fn actor_load_returns_complete_snapshot_when_removed_after_read() {
        let f = ServiceUpdateFixture::new(&[]).await;
        let guard = f.db.hold_hash_replies_for_test().await;
        let mut read = Box::pin(tokio::task::unconstrained(
            f.repo.get_actor_by_zpr_addr(&f.address),
        ));
        assert!(futures::poll!(read.as_mut()).is_pending());
        f.repo.rm_actor_by_zpr_addr(&f.address).await.unwrap();
        drop(guard);
        let old = read
            .await
            .expect("a captured snapshot survives subsequent removal");
        assert!(old.provides("shared:svc"));
        assert!(matches!(
            f.repo.get_actor_by_zpr_addr(&f.address).await,
            Err(StoreError::NotFound(_))
        ));
    }

    #[tokio::test]
    async fn actor_load_missing_identity_metadata_is_explicit_invalid_data() {
        let f = ServiceUpdateFixture::new(&[]).await;
        f.db.del(&actor_key_for(&f.address)).await.unwrap();
        f.db.hset(&actor_key_for(&f.address), "ctime", "present")
            .await
            .unwrap();
        assert!(matches!(
            f.repo.get_actor_by_zpr_addr(&f.address).await,
            Err(StoreError::InvalidData(message)) if message.contains("missing identity_keys")
        ));
    }

    #[tokio::test]
    async fn actor_load_snapshot_preserves_type_and_json_errors() {
        let f = ServiceUpdateFixture::new(&[]).await;
        let attrs = attrs_key_for(&f.address);
        f.db.set(&attrs, "not a hash").await.unwrap();
        assert!(matches!(
            f.repo.get_actor_by_zpr_addr(&f.address).await,
            Err(StoreError::Redis(_))
        ));
        f.db.del(&attrs).await.unwrap();
        f.db.hset(&attrs, "broken", "not JSON").await.unwrap();
        assert!(matches!(
            f.repo.get_actor_by_zpr_addr(&f.address).await,
            Err(StoreError::Serialization(_))
        ));
    }

    #[tokio::test]
    async fn canceled_service_update_before_pipeline_preserves_claims_and_indexes() {
        let f = ServiceUpdateFixture::new(&["new:svc"]).await;
        let guard = f.db.hold_pipelines_for_test().await;
        let mut update = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        assert!(futures::poll!(update.as_mut()).is_pending());
        drop(update);
        drop(guard);
        f.assert_services(&["shared:svc", "old%svc"]).await;
    }

    #[tokio::test]
    async fn canceled_service_update_after_pipeline_keeps_claims_and_indexes_consistent() {
        let f = ServiceUpdateFixture::new(&["new:svc", "shared:svc"]).await;
        let guard = f.db.hold_pipeline_replies_for_test().await;
        let mut update = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        assert!(futures::poll!(update.as_mut()).is_pending());
        f.assert_services(&["new:svc", "shared:svc"]).await;
        drop(update);
        drop(guard);
        f.assert_services(&["new:svc", "shared:svc"]).await;
        f.repo.update_actor(&f.updated).await.unwrap();
        f.assert_services(&["new:svc", "shared:svc"]).await;
    }

    #[tokio::test]
    async fn canceled_service_removal_after_pipeline_preserves_other_providers() {
        let f = ServiceUpdateFixture::new(&[]).await;
        let guard = f.db.hold_pipeline_replies_for_test().await;
        let mut update = Box::pin(tokio::task::unconstrained(f.repo.update_actor(&f.updated)));
        assert!(futures::poll!(update.as_mut()).is_pending());
        drop(update);
        drop(guard);
        f.assert_services(&[]).await;
    }

    #[tokio::test]
    async fn rejected_service_update_preserves_all_indexes_then_retries() {
        let f = ServiceUpdateFixture::new(&["new:svc"]).await;
        f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
        assert!(f.repo.update_actor(&f.updated).await.is_err());
        f.assert_services(&["shared:svc", "old%svc"]).await;
        f.db.set_set_ex_fault(crate::db::FaultMode::None);
        f.repo.update_actor(&f.updated).await.unwrap();
        f.assert_services(&["new:svc"]).await;
    }

    #[tokio::test]
    async fn test_add_and_get_actor_roundtrip() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_with_services_defexp(
            ROLE_NODE,
            "fd5a:5052::1",
            &["svc:one", "svc%two"],
            "actor-1",
        );
        let zpr_addr: IpAddr = "fd5a:5052::1".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();
        let loaded = repo.get_actor_by_zpr_addr(&zpr_addr).await.unwrap();

        assert!(loaded.is_node());
        assert_eq!(loaded.get_cn(), Some("actor-1"));
        assert_eq!(loaded.get_zpr_addr(), Some(&zpr_addr));
        assert!(loaded.provides("svc:one"));
        assert!(loaded.provides("svc%two"));
        assert_eq!(loaded.get_identity(), Some(vec!["id-1".to_string()]));
    }

    #[tokio::test]
    async fn test_list_services_decodes_names() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            "fd5a:5052::2",
            &["svc:one", "svc%two"],
            "actor-1",
        );
        let zpr_addr: IpAddr = "fd5a:5052::2".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();
        let mut services = repo.list_services().await.unwrap();
        services.sort_by(|a, b| a.name.cmp(&b.name));

        let names: Vec<String> = services.iter().map(|s| s.name.clone()).collect();
        assert_eq!(names, vec!["svc%two".to_string(), "svc:one".to_string()]);
        for entry in services {
            assert_eq!(entry.zpr_addr, zpr_addr);
        }
    }

    #[tokio::test]
    async fn test_service_registry_keeps_other_provider_when_one_disconnects() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let first_addr: IpAddr = "fd5a:5052::21".parse().unwrap();
        let second_addr: IpAddr = "fd5a:5052::22".parse().unwrap();
        let first = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            &first_addr.to_string(),
            &["shared.svc.zpr"],
            "first-provider",
        );
        let second = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            &second_addr.to_string(),
            &["shared.svc.zpr"],
            "second-provider",
        );

        repo.add_actor(&first).await.unwrap();
        repo.add_actor(&second).await.unwrap();

        let mut providers = repo.list_services().await.unwrap();
        providers.sort_by_key(|entry| entry.zpr_addr);
        assert_eq!(providers.len(), 2);
        assert_eq!(providers[0].zpr_addr, first_addr);
        assert_eq!(providers[1].zpr_addr, second_addr);

        let updated_first = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            &first_addr.to_string(),
            &["replacement.svc.zpr"],
            "first-provider",
        );
        repo.update_actor(&updated_first).await.unwrap();
        let shared_providers = repo
            .get_zpr_addrs_for_service("shared.svc.zpr")
            .await
            .unwrap();
        assert_eq!(shared_providers, vec![second_addr]);
        let replacement_providers = repo
            .get_zpr_addrs_for_service("replacement.svc.zpr")
            .await
            .unwrap();
        assert_eq!(replacement_providers, vec![first_addr]);

        repo.rm_actor_by_zpr_addr(&first_addr).await.unwrap();
        let providers = repo.list_services().await.unwrap();
        assert_eq!(providers.len(), 1);
        assert_eq!(providers[0].name, "shared.svc.zpr");
        assert_eq!(providers[0].zpr_addr, second_addr);
    }

    #[tokio::test]
    async fn test_rm_actor_cleans_up_keys() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor =
            make_actor_with_services_defexp(ROLE_NODE, "fd5a:5052::3", &["svc:one"], "actor-1");
        let zpr_addr: IpAddr = "fd5a:5052::3".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();
        repo.rm_actor_by_zpr_addr(&zpr_addr).await.unwrap();

        let base_key = actor_key_for(&zpr_addr);
        let attrs_key = attrs_key_for(&zpr_addr);
        let services_key = actor_services_key_for(&zpr_addr);
        let svc_key = service_key_for("svc:one");

        assert!(!db.exists(&base_key).await.unwrap());
        assert!(!db.exists(&attrs_key).await.unwrap());
        assert!(!db.exists(&services_key).await.unwrap());
        assert!(!db.exists(&svc_key).await.unwrap());

        let nodes = db.smembers(KEY_NODES).await.unwrap();
        assert!(!nodes.contains(&zpr_addr.to_string()));
    }

    #[tokio::test]
    async fn test_list_actor_cns_with_role_filter() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);

        let node_actor =
            make_actor_with_services_defexp(ROLE_NODE, "fd5a:5052::10", &["svc:one"], "node-cn");
        let adapter_actor = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            "fd5a:5052::20",
            &["svc:two"],
            "adapter-cn",
        );

        repo.add_actor(&node_actor).await.unwrap();
        repo.add_actor(&adapter_actor).await.unwrap();

        let mut all_cns = repo.list_actor_cns(None).await.unwrap();
        all_cns.sort();
        assert_eq!(
            all_cns,
            vec!["adapter-cn".to_string(), "node-cn".to_string()]
        );

        let node_cns = repo.list_actor_cns(Some(Role::Node)).await.unwrap();
        assert_eq!(node_cns, vec!["node-cn".to_string()]);

        let adapter_cns = repo.list_actor_cns(Some(Role::Adapter)).await.unwrap();
        assert_eq!(adapter_cns, vec!["adapter-cn".to_string()]);
    }

    #[tokio::test]
    async fn test_get_zpr_addr_for_service_returns_addr_and_none() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor =
            make_actor_with_services_defexp(ROLE_NODE, "fd5a:5052::30", &["svc:one"], "actor-1");
        let zpr_addr: IpAddr = "fd5a:5052::30".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();

        let found = repo.get_zpr_addr_for_service("svc:one").await.unwrap();
        assert_eq!(found, Some(zpr_addr));

        let missing = repo.get_zpr_addr_for_service("svc:missing").await.unwrap();
        assert!(missing.is_none());
    }

    #[tokio::test]
    async fn test_get_zpr_addr_for_service_invalid_addr() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let svc_key = service_key_for("svc:bad");

        db.sadd(&svc_key, "not-an-ip").await.unwrap();

        let err = repo.get_zpr_addr_for_service("svc:bad").await.unwrap_err();
        match err {
            StoreError::InvalidData(_) => {}
            other => panic!("unexpected error: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_get_actor_attrs_returns_only_found() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_defexp(&[
            (key::ROLE, ROLE_ADAPTER),
            (key::CN, "actor-attrs"),
            (key::ZPR_ADDR, "fd5a:5052::40"),
        ]);
        let zpr_addr: IpAddr = "fd5a:5052::40".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();

        let attrs = repo
            .get_actor_attrs(&zpr_addr, &[key::CN, "missing.key", key::ROLE])
            .await
            .unwrap();

        assert_eq!(attrs.len(), 2);
        assert_eq!(attrs[0].get_key(), key::CN);
        assert_eq!(attrs[0].get_single_value().unwrap(), "actor-attrs");
        assert_eq!(attrs[1].get_key(), key::ROLE);
        assert_eq!(attrs[1].get_single_value().unwrap(), ROLE_ADAPTER);
    }

    /// An actor that never sent a key reads back as absent rather than as an error.
    #[tokio::test]
    async fn test_get_a2a_dh_pubkey_none_when_absent() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_for_pubkey_test("fd5a:5052::60", "no-key", None);
        let zpr_addr: IpAddr = "fd5a:5052::60".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();

        let pubkey = repo.get_a2a_dh_pubkey_by_zpr_addr(&zpr_addr).await.unwrap();
        assert!(pubkey.is_none());
    }

    /// A stored FMT:BASE64 value decodes back to the raw 32 key bytes.
    #[tokio::test]
    async fn test_get_a2a_dh_pubkey_decodes_stored_value() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_for_pubkey_test(
            "fd5a:5052::61",
            "keyed",
            Some("ZprKF01:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="),
        );
        let zpr_addr: IpAddr = "fd5a:5052::61".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();

        let pubkey = repo
            .get_a2a_dh_pubkey_by_zpr_addr(&zpr_addr)
            .await
            .unwrap()
            .expect("actor has a stored key");
        assert_eq!(pubkey.public_key, (0..32u8).collect::<Vec<u8>>());
    }

    #[tokio::test]
    async fn test_get_a2a_dh_pubkey_rejects_unknown_format() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_for_pubkey_test("fd5a:5052::62", "bad-key", Some("ZprKF99:AAEC"));
        let zpr_addr: IpAddr = "fd5a:5052::62".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();

        let err = repo
            .get_a2a_dh_pubkey_by_zpr_addr(&zpr_addr)
            .await
            .unwrap_err();
        match err {
            StoreError::InvalidData(_) => {}
            other => panic!("unexpected error: {other:?}"),
        }
    }

    fn make_actor_for_pubkey_test(zpr_addr: &str, cn: &str, pubkey_value: Option<&str>) -> Actor {
        let mut attrs = vec![
            (key::ROLE, ROLE_ADAPTER),
            (key::CN, cn),
            (key::ZPR_ADDR, zpr_addr),
        ];
        if let Some(value) = pubkey_value {
            attrs.push((key::A2A_DH_PUBKEY, value));
        }
        make_actor_defexp(&attrs)
    }

    #[tokio::test]
    async fn test_get_actor_by_cn_not_found() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);

        let err = repo.get_actor_by_cn("missing-cn").await.unwrap_err();
        match err {
            StoreError::NotFound(_) => {}
            other => panic!("unexpected error: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_get_actor_by_cn_after_cache_loss() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db.clone());
        let actor =
            make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::51", &[], "machine-01");
        repo.add_actor(&actor).await.unwrap();

        let recovered = ActorRepo::new(db);
        assert!(
            recovered
                .list_actor_cns(None)
                .await
                .unwrap()
                .contains(&"machine-01".to_string())
        );
        let loaded = recovered.get_actor_by_cn("machine-01").await.unwrap();
        assert_eq!(loaded.get_zpr_addr(), actor.get_zpr_addr());
    }

    #[tokio::test]
    async fn test_get_actor_by_cn_repairs_stale_mapping() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let machine =
            make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::51", &[], "machine-01");
        let other = make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::52", &[], "other");
        repo.add_actor(&machine).await.unwrap();
        repo.add_actor(&other).await.unwrap();
        repo.cn_idx
            .insert("machine-01".into(), *other.get_zpr_addr().unwrap());

        let loaded = repo.get_actor_by_cn("machine-01").await.unwrap();
        assert_eq!(loaded.get_zpr_addr(), machine.get_zpr_addr());
        assert_eq!(
            repo.cn_idx.get("machine-01").map(|entry| *entry),
            machine.get_zpr_addr().copied()
        );
    }

    #[tokio::test]
    async fn test_get_actor_by_cn_and_rm_clears_cache() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_with_services_defexp(
            ROLE_NODE,
            "fd5a:5052::50",
            &["svc:one"],
            "actor-cache",
        );
        let zpr_addr: IpAddr = "fd5a:5052::50".parse().unwrap();

        repo.add_actor(&actor).await.unwrap();
        let loaded = repo.get_actor_by_cn("actor-cache").await.unwrap();
        assert_eq!(loaded.get_zpr_addr(), Some(&zpr_addr));

        repo.rm_actor_by_zpr_addr(&zpr_addr).await.unwrap();
        let err = repo.get_actor_by_cn("actor-cache").await.unwrap_err();
        match err {
            StoreError::NotFound(_) => {}
            other => panic!("unexpected error: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_list_services_for_actor_only_returns_actor_services() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);

        let node_actor = make_actor_with_services_defexp(
            ROLE_NODE,
            "fd5a:5052::30",
            &["svc:one", "svc%two"],
            "cn.1",
        );
        let adapter_actor =
            make_actor_with_services_defexp(ROLE_ADAPTER, "fd5a:5052::40", &["svc:three"], "cn.2");

        repo.add_actor(&node_actor).await.unwrap();
        repo.add_actor(&adapter_actor).await.unwrap();

        let node_addr: IpAddr = "fd5a:5052::30".parse().unwrap();
        let mut node_services = repo.list_services_for_actor(&node_addr).await.unwrap();
        node_services.sort();

        assert_eq!(
            node_services,
            vec!["svc%two".to_string(), "svc:one".to_string()]
        );

        let adapter_addr: IpAddr = "fd5a:5052::40".parse().unwrap();
        let mut adapter_services = repo.list_services_for_actor(&adapter_addr).await.unwrap();
        adapter_services.sort();

        assert_eq!(adapter_services, vec!["svc:three".to_string()]);
    }

    #[tokio::test]
    async fn test_get_cn_by_zpr_addr_returns_cn() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_defexp(&[
            (key::ROLE, ROLE_NODE),
            (key::CN, "my-node"),
            (key::ZPR_ADDR, "fd5a:5052::10"),
        ]);
        repo.add_actor(&actor).await.unwrap();

        let addr: IpAddr = "fd5a:5052::10".parse().unwrap();
        let cn = repo.get_cn_by_zpr_addr(&addr).await.unwrap();
        assert_eq!(cn, "my-node");
    }

    #[tokio::test]
    async fn test_get_cn_by_zpr_addr_not_found() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);

        let addr: IpAddr = "fd5a:5052::10".parse().unwrap();
        let err = repo.get_cn_by_zpr_addr(&addr).await.unwrap_err();
        assert!(matches!(err, StoreError::NotFound(_)));
    }

    /// update_actor is authoritative: an attribute dropped from the actor is dropped from
    /// the store, so a later load does not resurrect it. (An attribute refresh can remove
    /// attributes -- e.g. a trusted service no longer vends one.)
    #[tokio::test]
    async fn test_update_actor_drops_removed_attributes() {
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let mut actor = make_actor_defexp(&[
            (key::ROLE, ROLE_NODE),
            (key::CN, "drop-node"),
            (key::ZPR_ADDR, "fd5a:5052::10"),
            ("user.dept", "sales"),
        ]);
        repo.add_actor(&actor).await.unwrap();

        actor.remove_attribute("user.dept");
        repo.update_actor(&actor).await.unwrap();

        let addr: IpAddr = "fd5a:5052::10".parse().unwrap();
        let loaded = repo.get_actor_by_zpr_addr(&addr).await.unwrap();
        assert!(loaded.attrs_iter().all(|a| a.get_key() != "user.dept"));
        // The rest of the actor survived the rewrite.
        assert_eq!(loaded.get_cn(), Some("drop-node"));
    }

    #[tokio::test]
    async fn test_get_cn_by_zpr_addr_matches_get_actor_get_cn() {
        //  Both get_cn_by_zpr_addr and get_actor_by_zpr_addr().get_cn() must return the same CN.
        let db = Arc::new(FakeDb::new());
        let repo = ActorRepo::new(db);
        let actor = make_actor_defexp(&[
            (key::ROLE, ROLE_NODE),
            (key::CN, "roundtrip-node"),
            (key::ZPR_ADDR, "fd5a:5052::10"),
        ]);
        repo.add_actor(&actor).await.unwrap();

        let addr: IpAddr = "fd5a:5052::10".parse().unwrap();
        let cn_direct = repo.get_cn_by_zpr_addr(&addr).await.unwrap();
        let loaded = repo.get_actor_by_zpr_addr(&addr).await.unwrap();
        let cn_via_actor = loaded.get_cn().unwrap();
        assert_eq!(cn_direct, cn_via_actor);
    }
}
