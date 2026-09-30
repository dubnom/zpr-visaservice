//! Authenticated RFC 2136 updates for the policy-gated ZPR DNS service.

use std::net::IpAddr;
use std::path::Path;
use std::process::Stdio;

use tokio::io::AsyncWriteExt;
use tokio::process::Command;

use crate::config::DnsUpdateConfig;
use crate::error::ServiceError;

/// Publishes service-provider address records through BIND's TSIG-authenticated nsupdate client.
#[derive(Clone)]
pub struct DnsPublisher {
    config: DnsUpdateConfig,
    zone: String,
}

impl DnsPublisher {
    /// Validate and construct a publisher from the optional Visa Service configuration.
    pub fn new(config: DnsUpdateConfig) -> Result<Self, ServiceError> {
        if config.port == 0 {
            return Err(ServiceError::DnsUpdate(
                "DNS server port must be non-zero".into(),
            ));
        }
        if config.ttl_seconds == 0 {
            return Err(ServiceError::DnsUpdate(
                "DNS record TTL must be non-zero".into(),
            ));
        }
        let zone = canonical_name(&config.zone)?;
        if !config.tsig_key_file.is_file() {
            return Err(ServiceError::DnsUpdate(format!(
                "TSIG key file does not exist: {}",
                config.tsig_key_file.display()
            )));
        }
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode = std::fs::metadata(&config.tsig_key_file)?
                .permissions()
                .mode();
            if mode & 0o077 != 0 {
                return Err(ServiceError::DnsUpdate(format!(
                    "TSIG key file must not be accessible to group or other users: {}",
                    config.tsig_key_file.display()
                )));
            }
        }
        Ok(Self { config, zone })
    }

    /// Return whether a service ID is a valid owner in the configured DNS zone.
    pub fn accepts_service_id(&self, service_name: &str) -> bool {
        self.owner_name(service_name).is_ok()
    }

    /// Publish one authenticated provider address as an A or AAAA record.
    pub async fn add_provider(
        &self,
        service_name: &str,
        address: IpAddr,
    ) -> Result<(), ServiceError> {
        self.update_provider(service_name, address, true).await
    }

    /// Withdraw one provider address without affecting other providers of the same service.
    pub async fn remove_provider(
        &self,
        service_name: &str,
        address: IpAddr,
    ) -> Result<(), ServiceError> {
        self.update_provider(service_name, address, false).await
    }

    /// Replace both address RRsets for a service with its currently authorized providers.
    pub async fn replace_providers(
        &self,
        service_name: &str,
        addresses: &[IpAddr],
    ) -> Result<(), ServiceError> {
        let owner = self.owner_name(service_name)?;
        let mut update = format!(
            "server {} {}\nzone {}\nupdate delete {owner} A\nupdate delete {owner} AAAA\n",
            self.config.server, self.config.port, self.zone
        );
        for address in addresses {
            let record_type = if address.is_ipv4() { "A" } else { "AAAA" };
            update.push_str(&format!(
                "update add {owner} {} {record_type} {address}\n",
                self.config.ttl_seconds
            ));
        }
        update.push_str("send\n");
        self.send_update(&owner, &update).await
    }

    async fn update_provider(
        &self,
        service_name: &str,
        address: IpAddr,
        add: bool,
    ) -> Result<(), ServiceError> {
        let owner = self.owner_name(service_name)?;
        let record_type = if address.is_ipv4() { "A" } else { "AAAA" };
        let action = if add { "add" } else { "delete" };
        let ttl = if add {
            format!(" {}", self.config.ttl_seconds)
        } else {
            String::new()
        };
        let update = format!(
            "server {} {}\nzone {}\nupdate {action} {owner}{ttl} {record_type} {address}\nsend\n",
            self.config.server, self.config.port, self.zone
        );

        self.send_update(&owner, &update).await
    }

    async fn send_update(&self, owner: &str, update: &str) -> Result<(), ServiceError> {
        let mut child = Command::new(&self.config.nsupdate_bin)
            .arg("-k")
            .arg(&self.config.tsig_key_file)
            .arg("-v")
            .stdin(Stdio::piped())
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .kill_on_drop(true)
            .spawn()
            .map_err(|error| {
                ServiceError::DnsUpdate(format!(
                    "could not start {}: {error}",
                    self.config.nsupdate_bin.display()
                ))
            })?;

        let mut stdin = child
            .stdin
            .take()
            .ok_or_else(|| ServiceError::DnsUpdate("nsupdate stdin was unavailable".into()))?;
        stdin.write_all(update.as_bytes()).await.map_err(|error| {
            ServiceError::DnsUpdate(format!("could not send update for {owner}: {error}"))
        })?;
        drop(stdin);

        let output =
            tokio::time::timeout(std::time::Duration::from_secs(5), child.wait_with_output())
                .await
                .map_err(|_| ServiceError::DnsUpdate(format!("nsupdate timed out for {owner}")))?
                .map_err(|error| {
                    ServiceError::DnsUpdate(format!("nsupdate failed for {owner}: {error}"))
                })?;
        if !output.status.success() {
            let diagnostic = String::from_utf8_lossy(&output.stderr);
            let diagnostic = diagnostic.trim();
            return Err(ServiceError::DnsUpdate(if diagnostic.is_empty() {
                format!("nsupdate exited with {} for {owner}", output.status)
            } else {
                format!(
                    "nsupdate exited with {} for {owner}: {diagnostic}",
                    output.status
                )
            }));
        }
        Ok(())
    }

    fn owner_name(&self, service_name: &str) -> Result<String, ServiceError> {
        let owner = canonical_name(service_name)?;
        let zone_suffix = format!(".{}", self.zone);
        if !owner.ends_with(&zone_suffix) {
            return Err(ServiceError::DnsUpdate(format!(
                "service name {owner} is outside configured DNS zone {}",
                self.zone
            )));
        }
        Ok(owner)
    }
}

/// Lowercase and validate an absolute ASCII DNS name, adding its final root dot.
fn canonical_name(name: &str) -> Result<String, ServiceError> {
    let name = name.trim_end_matches('.').to_ascii_lowercase();
    let labels = name.split('.').collect::<Vec<_>>();
    let valid = !name.is_empty()
        && name.len() <= 253
        && labels.iter().all(|label| {
            !label.is_empty()
                && label.len() <= 63
                && label.as_bytes()[0].is_ascii_alphanumeric()
                && label.as_bytes()[label.len() - 1].is_ascii_alphanumeric()
                && label
                    .bytes()
                    .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
        });
    if !valid {
        return Err(ServiceError::DnsUpdate(format!(
            "invalid DNS name {name:?}; expected lowercase-compatible ASCII labels"
        )));
    }
    Ok(format!("{name}."))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::os::unix::fs::PermissionsExt;

    fn fake_nsupdate(directory: &Path) -> (DnsUpdateConfig, String) {
        let command_file = directory.join("nsupdate-input");
        let executable = directory.join("nsupdate-fake");
        fs::write(
            &executable,
            format!("#!/bin/sh\ncat > '{}'\n", command_file.display()),
        )
        .expect("write fake nsupdate executable");
        let mut permissions = fs::metadata(&executable)
            .expect("read fake executable metadata")
            .permissions();
        permissions.set_mode(0o700);
        fs::set_permissions(&executable, permissions).expect("set executable mode");
        let key_file = directory.join("publisher.key");
        fs::write(&key_file, "test-only").expect("write test TSIG key marker");
        let mut key_permissions = fs::metadata(&key_file)
            .expect("read TSIG key metadata")
            .permissions();
        key_permissions.set_mode(0o600);
        fs::set_permissions(&key_file, key_permissions).expect("protect test TSIG key");
        (
            DnsUpdateConfig {
                server: "fd5a:5052::53".parse().expect("parse fixture ZPR address"),
                port: 53,
                zone: "svc.zpr.".into(),
                tsig_key_file: key_file,
                ttl_seconds: 30,
                nsupdate_bin: executable,
            },
            command_file.to_string_lossy().into_owned(),
        )
    }

    #[tokio::test]
    async fn provider_add_and_remove_are_zone_scoped_and_address_specific() {
        let directory = tempfile::tempdir().expect("create temp dir");
        let (config, command_file) = fake_nsupdate(directory.path());
        let publisher = DnsPublisher::new(config).expect("construct publisher");

        publisher
            .add_provider("Payroll.Finance.svc.zpr", "fd5a:5052::22".parse().unwrap())
            .await
            .expect("publish IPv6 provider");
        let add = fs::read_to_string(&command_file).expect("read add update");
        assert!(add.contains("zone svc.zpr.\n"));
        assert!(add.contains("update add payroll.finance.svc.zpr. 30 AAAA fd5a:5052::22"));

        publisher
            .remove_provider("payroll.finance.svc.zpr.", "10.1.2.3".parse().unwrap())
            .await
            .expect("remove IPv4 provider");
        let remove = fs::read_to_string(&command_file).expect("read remove update");
        assert!(remove.contains("update delete payroll.finance.svc.zpr. A 10.1.2.3"));
    }

    #[test]
    fn publisher_rejects_names_outside_its_zone_and_zero_ttl() {
        let directory = tempfile::tempdir().expect("create temp dir");
        let (mut config, _) = fake_nsupdate(directory.path());
        let publisher = DnsPublisher::new(config.clone()).expect("construct publisher");
        assert!(publisher.owner_name("attacker.example").is_err());
        config.ttl_seconds = 0;
        assert!(DnsPublisher::new(config).is_err());
    }

    #[tokio::test]
    async fn provider_replacement_deletes_stale_addresses_and_adds_current_set() {
        let directory = tempfile::tempdir().expect("create temp dir");
        let (config, command_file) = fake_nsupdate(directory.path());
        let publisher = DnsPublisher::new(config).expect("construct publisher");
        publisher
            .replace_providers(
                "payroll.finance.svc.zpr",
                &[
                    "fd5a:5052::22".parse().unwrap(),
                    "10.0.0.22".parse().unwrap(),
                ],
            )
            .await
            .expect("replace provider RRsets");
        let update = fs::read_to_string(command_file).expect("read RRset replacement");
        assert!(update.contains("update delete payroll.finance.svc.zpr. A\n"));
        assert!(update.contains("update delete payroll.finance.svc.zpr. AAAA\n"));
        assert!(update.contains("update add payroll.finance.svc.zpr. 30 AAAA fd5a:5052::22\n"));
        assert!(update.contains("update add payroll.finance.svc.zpr. 30 A 10.0.0.22\n"));
    }
}
