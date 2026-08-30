import { createServer } from 'node:http';

// A tiny, dependency-free stand-in for the Go/Huma API. It serves canned data
// shaped like the sample OpenAPI document so both frontends can build and serve
// end-to-end before the real API is complete. It is NOT a spec
// validator and deliberately holds only a handful of presets.

const PORT = Number(process.env.SAMPLE_API_PORT ?? 8787);
const REVISION = 'sample-0001';

/** @type {Array<Record<string, unknown>>} */
const PRESETS = [
  {
    id: 'prusa/mk4-input-shaper',
    type: 'printer',
    name: 'Prusa MK4 (Input Shaper)',
    vendor: 'prusa',
    model: 'MK4',
    material: null,
    spec: '0.4 mm nozzle · 250×210×220 mm',
  },
  {
    id: 'prusa/petg-generic',
    type: 'filament',
    name: 'Prusament PETG',
    vendor: 'prusa',
    model: null,
    material: 'PETG',
    spec: '240 °C · bed 85 °C',
  },
  {
    id: 'bambu/x1c-0.4-standard',
    type: 'process',
    name: 'X1C 0.4 Standard',
    vendor: 'bambu',
    model: 'X1 Carbon',
    material: null,
    spec: '0.2 mm layer · 15% infill',
  },
];

const VENDORS = [
  {
    slug: 'prusa',
    display_name: 'Prusa Research',
    brands: ['Prusa', 'Prusament'],
    website: 'https://www.prusa3d.com',
  },
  {
    slug: 'bambu',
    display_name: 'Bambu Lab',
    brands: ['Bambu Lab'],
    website: 'https://bambulab.com',
  },
];

/**
 * Compute a naive case-insensitive match range (UTF-16 offsets) of `q` within
 * the preset name, mirroring the shape the real API returns for highlighting.
 * @param {string} name
 * @param {string} q
 */
function nameMatch(name, q) {
  if (!q) return undefined;
  const idx = name.toLowerCase().indexOf(q.toLowerCase());
  if (idx < 0) return undefined;
  return { field: 'name', ranges: [[idx, idx + q.length]] };
}

function summarize(preset, q) {
  const { id, type, name, vendor, model, material, spec } = preset;
  const match = nameMatch(name, q);
  return { id, type, name, vendor, model, material, spec, ...(match ? { match } : {}) };
}

function send(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    'content-type': 'application/json',
    'access-control-allow-origin': '*',
    'x-catalog-revision': REVISION,
  });
  res.end(payload);
}

const server = createServer((req, res) => {
  const url = new URL(req.url ?? '/', `http://localhost:${PORT}`);
  const path = url.pathname;

  if (path === '/v1/health') {
    return send(res, 200, {
      ready: true,
      revision: REVISION,
      last_ingest_at: new Date().toISOString(),
    });
  }

  if (path === '/v1/presets') {
    const q = url.searchParams.get('q') ?? '';
    const type = url.searchParams.get('type');
    const vendor = url.searchParams.get('vendor');
    let results = PRESETS;
    if (type) results = results.filter((p) => p.type === type);
    if (vendor) results = results.filter((p) => p.vendor === vendor);
    if (q) {
      const needle = q.toLowerCase();
      results = results.filter((p) =>
        [p.name, p.vendor, p.model, p.material].some(
          (v) => typeof v === 'string' && v.toLowerCase().includes(needle),
        ),
      );
    }
    return send(res, 200, { results: results.map((p) => summarize(p, q)), revision: REVISION });
  }

  const detail = path.match(/^\/v1\/presets\/(.+)$/);
  if (detail) {
    const id = decodeURIComponent(detail[1]);
    const preset = PRESETS.find((p) => p.id === id);
    if (!preset) {
      return send(res, 404, {
        type: 'about:blank',
        title: 'Not Found',
        status: 404,
        detail: `Unknown preset: ${id}`,
      });
    }
    return send(res, 200, {
      id: preset.id,
      type: preset.type,
      name: preset.name,
      vendor: preset.vendor,
      source: 'catalog',
      import_url: `https://cloud-presets.coldcrabby.dev/v1/presets/${encodeURIComponent(preset.id)}`,
      params: { nozzle_diameter: 0.4, layer_height: 0.2 },
    });
  }

  if (path === '/v1/vendors') {
    return send(res, 200, VENDORS);
  }

  if (path === '/v1/vendor/presets') {
    // The sample API does not verify JWTs; it just returns Prusa's presets as a demo scope.
    return send(
      res,
      200,
      PRESETS.filter((p) => p.vendor === 'prusa').map((p) => summarize(p, '')),
    );
  }

  return send(res, 404, {
    type: 'about:blank',
    title: 'Not Found',
    status: 404,
    detail: `No route for ${path}`,
  });
});

server.listen(PORT, () => {
  console.log(`[sample-api] listening on http://localhost:${PORT} (revision ${REVISION})`);
});
