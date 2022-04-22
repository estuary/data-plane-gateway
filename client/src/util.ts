export type JsonObject = Record<string, unknown>;

export function trimUrl(orig: URL): string {
  let url = orig.toString();
  if (url.endsWith("/")) {
    url = url.slice(0, url.length - 1);
  }
  return url;
}
