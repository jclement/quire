// A stand-in OpenAI endpoint for the Playwright suite, serving both
// /v1/embeddings and /v1/chat/completions (vision, for screenshot alt text).
// The embeddings half is the same bag-of-words fake as
// internal/semantic/semantictest, so texts that share words come out similar
// and ranking is deterministic. Started by playwright.config.ts as a third
// webServer; the app instance points QUIRE_OPENAI_BASE_URL at it.
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
    if (req.method !== "POST")
      return new Response("not found", { status: 404 });
    if (req.headers.get("authorization") !== "Bearer test-key") {
      return Response.json({ error: { message: "bad key" } }, { status: 401 });
    }

    // Vision: a fixed sentence, so a spec can assert the alt text the app
    // wrote without depending on a model's phrasing. It echoes the image's
    // media type to prove the data URL actually arrived.
    if (url.pathname === "/v1/chat/completions") {
      const chat = (await req.json()) as {
        messages: {
          content: { type: string; image_url?: { url: string } }[];
        }[];
      };
      const part = chat.messages?.[0]?.content?.find(
        (c) => c.type === "image_url",
      );
      const url_ = part?.image_url?.url ?? "";
      if (!url_.startsWith("data:image/")) {
        return Response.json(
          { error: { message: "no image" } },
          { status: 400 },
        );
      }
      const kind = url_.slice("data:".length, url_.indexOf(";"));
      return Response.json({
        choices: [
          { message: { content: `A test screenshot in ${kind} format.` } },
        ],
      });
    }

    if (url.pathname !== "/v1/embeddings") {
      return new Response("not found", { status: 404 });
    }
    const body = (await req.json()) as { input: string[]; dimensions?: number };
    const dims = body.dimensions ?? 512;
    return Response.json({
      data: body.input.map((text, index) => ({
        index,
        embedding: vector(text, dims),
      })),
    });
  },
});
console.log(`fake openai on 127.0.0.1:${port}`);
