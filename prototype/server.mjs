// Zero-dependency static dev server for SingTail desktop prototype
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = fileURLToPath(new URL(".", import.meta.url));
const PORT = Number(process.env.PORT || 4188);

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml"
};

const server = createServer(async (req, res) => {
  try {
    const urlObj = new URL(req.url || "/", `http://127.0.0.1:${PORT}`);
    let reqPath = decodeURIComponent(urlObj.pathname);
    if (reqPath === "/") reqPath = "/index.html";

    const safeRel = normalize(reqPath).replace(/^(\.\.(\/|\\|$))+/, "");
    const absPath = join(ROOT, safeRel);

    if (!absPath.startsWith(ROOT)) {
      res.writeHead(403, { "Content-Type": "text/plain; charset=utf-8" });
      res.end("Forbidden");
      return;
    }

    const body = await readFile(absPath);
    const ext = extname(absPath).toLowerCase();
    res.writeHead(200, {
      "Content-Type": MIME[ext] || "application/octet-stream",
      "Cache-Control": "no-store"
    });
    res.end(body);
  } catch {
    res.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("Not Found");
  }
});

server.listen(PORT, "127.0.0.1", () => {
  console.log(`SingTail prototype running at http://127.0.0.1:${PORT}`);
});
