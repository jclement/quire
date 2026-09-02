// A stand-in OpenAI /v1/embeddings endpoint for the Playwright suite — the
// same bag-of-words fake as internal/semantic/semantictest, so texts that
// share words come out similar and ranking is deterministic. Started by
// playwright.config.ts as a third webServer; the app instance points
// QUIRE_OPENAI_BASE_URL at it.
const port = Number(process.env.FAKE_OPENAI_PORT ?? 8353);

function fnv1a(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

function vector(text: string, dimensions: number): number[] {
  const v = new Array<number>(dimensions).fill(0);
  for (const w of text.toLowerCase().match(/[a-z0-9]+/g) ?? []) {
    v[fnv1a(w) % dimensions]! += 1;
  }
  return v;
}

Bun.serve({
  port,
  hostname: "127.0.0.1",
  async fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === "/health") return new Response("ok");
    if (url.pathname !== "/v1/embeddings" || req.method !== "POST") {
      return new Response("not found", { status: 404 });
    }
    if (req.headers.get("authorization") !== "Bearer test-key") {
      return Response.json({ error: { message: "bad key" } }, { status: 401 });
    }
    const body = (await req.json()) as { input: string[]; dimensions?: number };
    const dims = body.dimensions ?? 512;
    return Response.json({
      data: body.input.map((text, index) => ({ index, embedding: vector(text, dims) })),
    });
  },
});
console.log(`fake openai on 127.0.0.1:${port}`);
