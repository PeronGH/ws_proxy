import { HOSTNAME, PASSWORD, PORT } from "./src/env.ts";
import { handler } from "./src/handler.ts";

Deno.serve(
  { hostname: HOSTNAME, port: Number.parseInt(PORT) },
  handler,
);
if (PASSWORD) console.log(`Password: ${PASSWORD}`);
