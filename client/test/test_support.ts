import * as jose from "https://deno.land/x/jose@v4.8.1/index.ts";

export const BASE_URL = new URL("https://localhost:28318");

const SIGNING_KEY = new TextEncoder().encode("supersecret");

export function makeJwt(
  {
    prefixes = ["acmeCo/", "ops.us-central1.v1/", "recovery/capture/acmeCo/"],
    expiresAt = "1h",
    key = SIGNING_KEY,
  }: { prefixes?: string[]; expiresAt?: string | number; key?: Uint8Array },
): Promise<string> {
  return new jose.SignJWT({ "prefixes": prefixes })
    .setProtectedHeader({ alg: "HS256" })
    .setIssuedAt()
    .setExpirationTime(expiresAt)
    .sign(key);
}
