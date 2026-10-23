use std::time::Duration;

use colored::Colorize;
use reqwest;
use reqwest::tls::Certificate;

use zpr::policy_types::PolicyBundle;

use admin_api_types::{
    ActorDescriptor, AuthRevokeDescriptor, CnEntry, DenyRecord, ListEntry, NamedListEntry,
    NetworkDetails, Revokes, ServiceDescriptor, Stats, VisaDescriptor,
};

use crate::error::VsaError;

const HTTP_TIMEOUT: Duration = Duration::from_secs(10);
const API_KEY_HEADER: &str = "X-API-Key";

#[derive(Debug)]
pub struct VsClient {
    cert: Certificate,
    api_url: String,
    api_key: String,
    quiet: bool,
}

#[allow(dead_code)]
pub enum RoleFilter {
    All,
    NodesOnly,
    AdaptersOnly,
}

impl VsClient {
    pub fn new(svc_url: String, cert: Certificate, api_key: String, quiet: bool) -> Self {
        VsClient {
            cert,
            api_url: svc_url,
            api_key,
            quiet,
        }
    }

    fn build_client(&self) -> Result<reqwest::blocking::Client, VsaError> {
        let cb = reqwest::blocking::ClientBuilder::new()
            .add_root_certificate(self.cert.clone())
            .https_only(true)
            .timeout(HTTP_TIMEOUT);
        let client = cb.build()?;
        Ok(client)
    }

    fn ht_get(&self, url: &str) -> Result<reqwest::blocking::Response, VsaError> {
        let client = self.build_client()?;
        if !self.quiet {
            eprint!("{}", format!(">> get {url}").dimmed());
        }
        let resp = client
            .get(url)
            .header(API_KEY_HEADER, &self.api_key)
            .send()?;

        let stat = resp.status();
        if !self.quiet {
            eprintln!("  {}", stat);
        }

        if stat.is_success() {
            return Ok(resp);
        }
        self.display_hterror(resp);
        Err(VsaError::HttpError(stat))
    }

    fn ht_post(&self, url: &str) -> Result<reqwest::blocking::Response, VsaError> {
        let client = self.build_client()?;
        if !self.quiet {
            eprint!("{}", format!(">> post {url}").dimmed());
        }
        let resp = client
            .post(url)
            .header(API_KEY_HEADER, &self.api_key)
            .send()?;

        let stat = resp.status();
        if !self.quiet {
            eprintln!("  {}", stat);
        }

        if stat.is_success() {
            return Ok(resp);
        }
        self.display_hterror(resp);
        Err(VsaError::HttpError(stat))
    }

    fn ht_post_json<T: serde::Serialize>(
        &self,
        url: &str,
        body: &T,
    ) -> Result<reqwest::blocking::Response, VsaError> {
        let client = self.build_client()?;
        if !self.quiet {
            eprint!("{}", format!(">> post {url}").dimmed());
        }
        let resp = client
            .post(url)
            .header(API_KEY_HEADER, &self.api_key)
            .json(body)
            .send()?;

        let stat = resp.status();
        if !self.quiet {
            eprintln!("  {}", stat);
        }

        if stat.is_success() {
            return Ok(resp);
        }
        self.display_hterror(resp);
        Err(VsaError::HttpError(stat))
    }

    fn ht_delete(&self, url: &str) -> Result<reqwest::blocking::Response, VsaError> {
        let client = self.build_client()?;
        if !self.quiet {
            eprint!("{}", format!(">> delete {url}").dimmed());
        }
        let resp = client
            .delete(url)
            .header(API_KEY_HEADER, &self.api_key)
            .send()?;

        let stat = resp.status();
        if !self.quiet {
            eprintln!("  {}", stat);
        }

        if stat.is_success() {
            return Ok(resp);
        }
        self.display_hterror(resp);
        Err(VsaError::HttpError(stat))
    }

    fn display_hterror(&self, error_resp: reqwest::blocking::Response) {
        if self.quiet {
            return;
        }
        eprintln!(
            "{} {}: {}",
            "HTTP Error".red(),
            error_resp.status(),
            error_resp.status().canonical_reason().unwrap_or("unknown")
        );
        if let Ok(txt) = error_resp.text() {
            eprintln!("       {}", txt);
        }
    }

    fn request_get_list_entries<T>(&self, req_uri: &str) -> Result<Vec<T>, VsaError>
    where
        T: serde::de::DeserializeOwned,
    {
        let resp = self.ht_get(req_uri)?;
        let entries: Vec<T> = resp.json()?;
        Ok(entries)
    }

    /// `GET <api_url>/admin/actors[?role=node|adapter]`
    ///
    /// Returns a list of CN values.
    pub fn get_actors(&self, filter: RoleFilter) -> Result<Vec<CnEntry>, VsaError> {
        let query = match filter {
            RoleFilter::NodesOnly => "?role=node",
            RoleFilter::AdaptersOnly => "?role=adapter",
            RoleFilter::All => "",
        };
        let entry_vec = self.request_get_list_entries::<CnEntry>(&format!(
            "{}/admin/actors{}",
            self.api_url, query
        ))?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/actors/<cn>`
    pub fn get_actor(&self, cn: &str) -> Result<ActorDescriptor, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/actors", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(cn);
        let resp = self.ht_get(requrl.as_str())?;
        let entry: ActorDescriptor = resp.json()?;
        Ok(entry)
    }

    /// `GET <api_url>/admin/services`
    ///
    /// Returns a list of service IDs (whihch are names)
    pub fn get_services(&self) -> Result<Vec<NamedListEntry>, VsaError> {
        let entry_vec = self.request_get_list_entries::<NamedListEntry>(&format!(
            "{}/admin/services",
            self.api_url
        ))?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/services/<id>`
    pub fn get_service(&self, id: &str) -> Result<ServiceDescriptor, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/services", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(id);
        let resp = self.ht_get(requrl.as_str())?;
        let entry: ServiceDescriptor = resp.json()?;
        Ok(entry)
    }

    /// `DELETE <api_url>/admin/services/<id>/cache`
    ///
    /// Reloads the trusted service's attribute data. The visa service then refreshes the
    /// actors behind live visas and revalidates those visas asynchronously (202 Accepted).
    pub fn flush_service_cache(&self, id: &str) -> Result<(), VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/services", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(id).push("cache");
        self.ht_delete(requrl.as_str())?;
        Ok(())
    }

    /// `GET <api_url>/admin/visas`
    pub fn get_visas(&self) -> Result<Vec<ListEntry>, VsaError> {
        let entry_vec =
            self.request_get_list_entries::<ListEntry>(&format!("{}/admin/visas", self.api_url))?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/visas/<id>`
    pub fn get_visa(&self, id: u64) -> Result<VisaDescriptor, VsaError> {
        let req = format!("{}/admin/visas/{}", self.api_url, id);
        let resp = self.ht_get(&req)?;
        let entry: VisaDescriptor = resp.json()?;
        Ok(entry)
    }

    /// `GET <api_url>/admin/policies`
    pub fn get_policies(&self) -> Result<Vec<ListEntry>, VsaError> {
        let entry_vec = self
            .request_get_list_entries::<ListEntry>(&format!("{}/admin/policies", self.api_url))?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/policies/<id>`
    pub fn get_policy(&self, id: u64) -> Result<PolicyBundle, VsaError> {
        let req = format!("{}/admin/policies/{}", self.api_url, id);
        let resp = self.ht_get(&req)?;
        let entry: PolicyBundle = resp.json()?;
        Ok(entry)
    }

    /// `GET <api_url>/admin/policies/curr`
    pub fn get_curr_policy(&self) -> Result<PolicyBundle, VsaError> {
        let req = format!("{}/admin/policies/curr", self.api_url);
        let resp = self.ht_get(&req)?;
        let entry: PolicyBundle = resp.json()?;
        Ok(entry)
    }

    /// `POST <api_url>/admin/policies`
    pub fn install_policy(&self, bundle: &PolicyBundle) -> Result<ListEntry, VsaError> {
        let req = format!("{}/admin/policies", self.api_url);
        let resp = self.ht_post_json(&req, bundle)?;
        let entry: ListEntry = resp.json()?;
        Ok(entry)
    }

    /// `DELETE <api_url>/admin/visas/<id>`
    pub fn revoke_visa(&self, id: u64) -> Result<Revokes, VsaError> {
        let req = format!("{}/admin/visas/{}", self.api_url, id);
        let resp = self.ht_delete(&req)?;
        let revoke: Revokes = resp.json()?;
        Ok(revoke)
    }

    /// `DELETE <api_url>/admin/actors/<cn>`
    pub fn revoke_actor(&self, cn: &str) -> Result<Revokes, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/actors", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(cn);
        let resp = self.ht_delete(requrl.as_str())?;
        let revoke: Revokes = resp.json()?;
        Ok(revoke)
    }

    /// `GET <api_url>/admin/actors/<cn>/visas`
    pub fn get_related_visas(&self, cn: &str) -> Result<Vec<ListEntry>, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/actors", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(cn).push("visas");
        let entry_vec = self.request_get_list_entries::<ListEntry>(requrl.as_str())?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/authrevoke`
    pub fn get_revokes(&self) -> Result<Vec<ListEntry>, VsaError> {
        let entry_vec = self
            .request_get_list_entries::<ListEntry>(&format!("{}/admin/authrevoke", self.api_url))?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/authrevoke/<id>`
    pub fn get_revoke(&self, id: &str) -> Result<AuthRevokeDescriptor, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/authrevoke", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(id);
        let resp = self.ht_get(requrl.as_str())?;
        let entry: AuthRevokeDescriptor = resp.json()?;
        Ok(entry)
    }

    /// `POST <api_url>/admin/authrevoke/clear`
    pub fn clear_revokes(&self) -> Result<Vec<ListEntry>, VsaError> {
        let req = format!("{}/admin/authrevoke/clear", self.api_url);
        let resp = self.ht_post(&req)?;
        let entries: Vec<ListEntry> = resp.json()?;
        Ok(entries)
    }

    /// `DELETE <api_url>/admin/authrevoke/<id>`
    pub fn remove_revoke(&self, id: &str) -> Result<ListEntry, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/authrevoke", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(id);
        let resp = self.ht_delete(requrl.as_str())?;
        let entry: ListEntry = resp.json()?;
        Ok(entry)
    }

    /// `POST <api_url>/admin/authrevoke/<id>`
    pub fn add_revoke(&self, id: &str) -> Result<ListEntry, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/authrevoke", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(id);
        let resp = self.ht_post(requrl.as_str())?;
        let entry: ListEntry = resp.json()?;
        Ok(entry)
    }

    /// `GET <api_url>/admin/network`
    pub fn get_network(&self) -> Result<NetworkDetails, VsaError> {
        let req = reqwest::Url::parse(&format!("{}/admin/network", self.api_url))?;
        let resp = self.ht_get(req.as_str())?;
        let entry: NetworkDetails = resp.json()?;
        Ok(entry)
    }

    /// `GET <api_url>/admin/nodes/<cn>/visas`
    ///
    /// Returns the IDs of the visas currently installed on the given node.
    pub fn get_visas_on_node(&self, cn: &str) -> Result<Vec<ListEntry>, VsaError> {
        let mut requrl = reqwest::Url::parse(&format!("{}/admin/nodes", self.api_url))?;
        requrl.path_segments_mut().unwrap().push(cn).push("visas");
        let entry_vec = self.request_get_list_entries::<ListEntry>(requrl.as_str())?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/visas/denies[?since=<EPOCH_MS>][&limit=<COUNT>]`
    ///
    /// Returns the recent-denies window, newest request first.
    pub fn get_denies(
        &self,
        since: Option<u64>,
        limit: Option<usize>,
    ) -> Result<Vec<DenyRecord>, VsaError> {
        let requrl = denies_url(&self.api_url, since, limit)?;
        let entry_vec = self.request_get_list_entries::<DenyRecord>(requrl.as_str())?;
        Ok(entry_vec)
    }

    /// `GET <api_url>/admin/stats`
    pub fn get_stats(&self) -> Result<Stats, VsaError> {
        let req = format!("{}/admin/stats", self.api_url);
        let resp = self.ht_get(&req)?;
        let entry: Stats = resp.json()?;
        Ok(entry)
    }
}

/// Builds the `/admin/visas/denies` URL, omitting each query parameter that is
/// `None` so the service applies its own default.
fn denies_url(
    api_url: &str,
    since: Option<u64>,
    limit: Option<usize>,
) -> Result<reqwest::Url, VsaError> {
    let mut requrl = reqwest::Url::parse(&format!("{api_url}/admin/visas/denies"))?;
    if since.is_none() && limit.is_none() {
        // query_pairs_mut() would leave a bare trailing "?" with nothing to append.
        return Ok(requrl);
    }
    {
        let mut qp = requrl.query_pairs_mut();
        if let Some(since) = since {
            qp.append_pair("since", &since.to_string());
        }
        if let Some(limit) = limit {
            qp.append_pair("limit", &limit.to_string());
        }
    }
    Ok(requrl)
}

#[cfg(test)]
mod tests {
    use super::*;
    use openssl::asn1::Asn1Time;
    use openssl::bn::BigNum;
    use openssl::hash::MessageDigest;
    use openssl::pkey::{PKey, Private};
    use openssl::rsa::Rsa;
    use openssl::ssl::{SslAcceptor, SslMethod};
    use openssl::x509::extension::{BasicConstraints, KeyUsage, SubjectAlternativeName};
    use openssl::x509::{X509, X509NameBuilder};
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::thread;
    use std::time::Instant;

    fn test_certificate(
        host: Option<&str>,
        authority: Option<(&X509, &PKey<Private>)>,
        expired: bool,
    ) -> (X509, PKey<Private>) {
        let key = PKey::from_rsa(Rsa::generate(2048).unwrap()).unwrap();
        let mut name = X509NameBuilder::new().unwrap();
        name.append_entry_by_text("CN", host.unwrap_or("test authority"))
            .unwrap();
        let name = name.build();
        let mut builder = X509::builder().unwrap();
        builder.set_version(2).unwrap();
        builder
            .set_serial_number(&BigNum::from_u32(1).unwrap().to_asn1_integer().unwrap())
            .unwrap();
        builder.set_subject_name(&name).unwrap();
        builder
            .set_issuer_name(authority.map_or(&*name, |(ca, _)| ca.subject_name()))
            .unwrap();
        builder.set_pubkey(&key).unwrap();
        builder
            .set_not_before(&Asn1Time::from_unix(0).unwrap())
            .unwrap();
        let not_after = if expired {
            Asn1Time::from_unix(1).unwrap()
        } else {
            Asn1Time::days_from_now(1).unwrap()
        };
        builder.set_not_after(&not_after).unwrap();
        if let Some(host) = host {
            let san = SubjectAlternativeName::new()
                .ip(host)
                .build(&builder.x509v3_context(authority.map(|(ca, _)| ca.as_ref()), None))
                .unwrap();
            builder.append_extension(san).unwrap();
        } else {
            builder
                .append_extension(BasicConstraints::new().critical().ca().build().unwrap())
                .unwrap();
            builder
                .append_extension(KeyUsage::new().key_cert_sign().crl_sign().build().unwrap())
                .unwrap();
        }
        builder
            .sign(
                authority.map_or(&key, |(_, key)| key),
                MessageDigest::sha256(),
            )
            .unwrap();
        (builder.build(), key)
    }

    fn tls_request(method: &str, trusted: bool, matching_host: bool, expired: bool) {
        let (ca, ca_key) = test_certificate(None, None, false);
        let (leaf, key) = test_certificate(
            Some(if matching_host {
                "127.0.0.1"
            } else {
                "127.0.0.2"
            }),
            Some((&ca, &ca_key)),
            expired,
        );
        let mut acceptor = SslAcceptor::mozilla_intermediate(SslMethod::tls()).unwrap();
        acceptor.set_private_key(&key).unwrap();
        acceptor.set_certificate(&leaf).unwrap();
        let acceptor = acceptor.build();
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        listener.set_nonblocking(true).unwrap();
        let address = listener.local_addr().unwrap();
        let server = thread::spawn(move || {
            let deadline = Instant::now() + Duration::from_secs(5);
            let stream = loop {
                match listener.accept() {
                    Ok((stream, _)) => break stream,
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        assert!(Instant::now() < deadline, "TLS client never connected");
                        thread::sleep(Duration::from_millis(10));
                    }
                    Err(error) => panic!("TLS test listener failed: {error}"),
                }
            };
            stream.set_nonblocking(false).unwrap();
            stream
                .set_read_timeout(Some(Duration::from_secs(5)))
                .unwrap();
            stream
                .set_write_timeout(Some(Duration::from_secs(5)))
                .unwrap();
            let Ok(mut stream) = acceptor.accept(stream) else {
                return false;
            };
            let mut bytes = [0; 4096];
            let count = stream.read(&mut bytes).unwrap_or(0);
            if count == 0 {
                return false;
            }
            let request = String::from_utf8_lossy(&bytes[..count]).to_lowercase();
            assert!(request.contains("x-api-key: test-only-api-key"));
            stream
                .write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}")
                .unwrap();
            true
        });
        let trusted_ca = if trusted {
            ca
        } else {
            test_certificate(None, None, false).0
        };
        let url = format!("https://{address}/admin/test");
        let client = VsClient::new(
            url.clone(),
            Certificate::from_pem(&trusted_ca.to_pem().unwrap()).unwrap(),
            "test-only-api-key".into(),
            true,
        );
        let result = match method {
            "GET" => client.ht_get(&url),
            "POST" => client.ht_post(&url),
            "JSON" => client.ht_post_json(&url, &serde_json::json!({})),
            "DELETE" => client.ht_delete(&url),
            _ => unreachable!(),
        };
        let expected = trusted && matching_host && !expired;
        assert_eq!(result.is_ok(), expected, "{method}: {result:?}");
        assert_eq!(
            server.join().unwrap(),
            expected,
            "credentials sent before TLS validation"
        );
    }

    #[test]
    fn tls_checks_every_request_method_before_sending_credentials() {
        for method in ["GET", "POST", "JSON", "DELETE"] {
            tls_request(method, true, true, false);
            tls_request(method, false, true, false);
            tls_request(method, true, false, false);
            tls_request(method, true, true, true);
        }
    }

    #[test]
    fn plaintext_admin_requests_are_rejected() {
        let ca = test_certificate(None, None, false).0;
        let client = VsClient::new(
            "http://127.0.0.1:1".into(),
            Certificate::from_pem(&ca.to_pem().unwrap()).unwrap(),
            "test-only-api-key".into(),
            true,
        );
        let error = client
            .ht_get("http://127.0.0.1:1/admin/network")
            .unwrap_err();
        assert!(matches!(error, VsaError::HttpRequest(error) if error.is_builder()));
    }

    /// `since`/`limit` are passed through verbatim, and omitted entirely when None.
    #[test]
    fn denies_url_includes_only_supplied_query_params() {
        let base = "https://[fd5a:5052::1]:8182";
        let url = |since, limit| denies_url(base, since, limit).unwrap().to_string();

        assert_eq!(url(None, None), format!("{base}/admin/visas/denies"));
        assert_eq!(
            url(Some(1_700_000_000_000), None),
            format!("{base}/admin/visas/denies?since=1700000000000")
        );
        assert_eq!(
            url(None, Some(25)),
            format!("{base}/admin/visas/denies?limit=25")
        );
        assert_eq!(
            url(Some(5), Some(25)),
            format!("{base}/admin/visas/denies?since=5&limit=25")
        );
    }
}
