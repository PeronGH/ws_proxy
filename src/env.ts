import "@std/dotenv/load";

export const HOSTNAME = Deno.env.get("HOSTNAME") ?? "localhost";
export const PORT = Deno.env.get("PORT") ?? "7769";
export const PASSWORD = Deno.env.get("PASSWORD");
