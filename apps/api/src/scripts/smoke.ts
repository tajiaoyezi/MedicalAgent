/**
 * c01 foundation smoke test — 需 docker-compose up 后运行
 */
import { config } from "../config.js";
import {
  computeFileHash,
  createObjectStorage,
  objectKeyForVersion,
} from "../services/object-storage.js";

async function smoke() {
  const storage = createObjectStorage();
  const tenantId = "smoke-tenant";
  const docId = "smoke-doc";
  const verId = "smoke-ver";
  const key = objectKeyForVersion(tenantId, docId, verId);
  const body = Buffer.from("medoffice smoke test", "utf8");
  const hash = computeFileHash(body);

  console.log("Object storage smoke...");
  await storage.put(key, body, "text/plain");
  const head = await storage.headObject(key);
  console.log("  headObject size:", head.size);

  const got = await storage.get(key);
  if (computeFileHash(got) !== hash) throw new Error("hash mismatch on get");
  console.log("  get OK");

  const url = await storage.presignedUrl(key, 60);
  console.log("  presignedUrl:", url.slice(0, 60) + "...");

  await storage.delete(key);
  console.log("  delete OK");

  const res = await fetch(`${config.webOrigin.replace("5173", "3001")}/api/health`).catch(
    () => fetch(`http://localhost:${config.port}/api/health`),
  );
  const health = await res.json();
  console.log("API health:", health);

  console.log("Smoke tests passed");
}

smoke().catch((e) => {
  console.error(e);
  process.exit(1);
});
