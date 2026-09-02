// Attachments and the photo→task capture gesture. Both write files into the
// vault from user-supplied names, which makes them worth testing for what
// they refuse as much as for what they accept.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

// A one-pixel PNG, so the bytes are a real image rather than a placeholder.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  "base64",
);

test("an uploaded attachment lands in the vault and serves back", async ({ request }) => {
  const res = await request.post("/api/v1/attachments", {
    multipart: {
      file: { name: "screen shot.png", mimeType: "image/png", buffer: PNG },
    },
  });
  expect(res.status()).toBe(201);
  const { data } = await res.json();

  // The server picks the path; the client only learns where it went.
  expect(data.path).toMatch(/\.png$/);
  expect(data.markdown).toContain(data.path);

  const fetched = await request.get(`/api/v1/files/${data.path}`);
  expect(fetched.status()).toBe(200);
  expect(fetched.headers()["content-type"]).toContain("image");
  expect(Buffer.from(await fetched.body()).equals(PNG)).toBe(true);
});

test("capture turns a photo into a dated task with the image attached", async ({
  request,
}) => {
  const res = await request.post("/api/v1/capture", {
    multipart: {
      text: "Permission slip",
      due: "today",
      file: { name: "slip.png", mimeType: "image/png", buffer: PNG },
    },
  });
  expect(res.status()).toBe(201);
  const { data } = await res.json();

  expect(data.text).toContain("Permission slip");
  // "today" is resolved server-side, like every other date entry point.
  expect(data.due).toMatch(/^\d{4}-\d{2}-\d{2}$/);

  // The task line carries the image inline, so the daily note shows it.
  const doc = await request.get(`/api/v1/documents/${data.doc_path}`);
  const body = await doc.text();
  expect(body).toContain("Permission slip");
  expect(body).toMatch(/!\[[^\]]*\]\(attachments/);
});

test("capture works with no text when a photo carries the meaning", async ({
  request,
}) => {
  // The mobile gesture is one tap: snap and go, no typing.
  const res = await request.post("/api/v1/capture", {
    multipart: {
      file: { name: "just-a-photo.png", mimeType: "image/png", buffer: PNG },
    },
  });
  expect(res.status()).toBe(201);
  expect((await res.json()).data.text).toContain("just-a-photo");
});

test("an upload with no file is refused", async ({ request }) => {
  const res = await request.post("/api/v1/attachments", {
    multipart: { notafile: "hello" },
  });
  expect(res.status()).toBeGreaterThanOrEqual(400);
});
