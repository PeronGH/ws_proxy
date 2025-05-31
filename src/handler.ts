import { PASSWORD } from "./env.ts";
import { ProxyManager } from "./proxy.ts";

const PROXY_UPGRADE_PATH = "/__proxy_ws";

export const handler: Deno.ServeHandler = async (
  req: Request,
): Promise<Response> => {
  const url = new URL(req.url);

  if (url.pathname === PROXY_UPGRADE_PATH) {
    if (PASSWORD && url.searchParams.get("password") !== PASSWORD) {
      return new Response("Unauthorized", { status: 401 });
    }
    return ProxyManager.handler(req);
  }

  console.log(`Proxying request: ${req.method} ${url.pathname}${url.search}`);

  const path = `${url.pathname}${url.search}`;
  const body = req.body ? await req.text() : undefined;

  return await ProxyManager.request(req.method, path, body);
};
