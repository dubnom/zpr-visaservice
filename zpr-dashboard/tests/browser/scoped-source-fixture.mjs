export function scopedSourceFixture(page, api, expect, catalogs) {
  if (catalogs) {
    api.assertions.default_source = catalogs[0].name;
    api.assertions.sources = catalogs.map(source => ({
      source_id: source.name, name: source.name, display_name: source.name,
      organization_id: api.assertions.organization_id, kind: "ldap",
    }));
  }
  const requests = [];
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/")) requests.push({ url, method: request.method() });
  });
  api.handlers.set("/api/assertions/source", route => {
    const sourceID = new URL(route.request().url()).searchParams.get("source");
    expect(api.assertions.sources.map(source => source.source_id)).toContain(sourceID);
    const source = catalogs ? catalogs.find(source => source.name === sourceID) : api.assertionSource;
    return route.fulfill({ json: {
      ...source, source_id: sourceID, organization_id: api.assertions.organization_id,
    } });
  });
  return () => {
    const reads = requests.filter(({ url }) => url.pathname === "/api/assertions/source");
    expect(reads.length).toBeGreaterThan(0);
    expect([...new Set(reads.map(({ url }) => url.searchParams.get("source")))].sort())
      .toEqual(api.assertions.sources.map(source => source.source_id).sort());
    for (const source of api.assertions.sources) {
      expect(source.organization_id).toBe(api.assertions.organization_id);
      expect(source.source_id).toBeTruthy();
    }
    for (const { url, method } of reads) {
      expect(method).toBe("GET");
      expect(api.assertions.sources.map(source => source.source_id)).toContain(url.searchParams.get("source"));
      expect([...url.searchParams.keys()]).toEqual(["source"]);
    }
    expect(requests.some(({ url }) => url.pathname.startsWith("/api/simulator"))).toBe(false);
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator"))).toBe(false);
    expect(api.counts.get("/api/assertions")).toBeGreaterThan(0);
  };
}
