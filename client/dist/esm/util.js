export function trimUrl(orig) {
    let url = orig.toString();
    if (url.endsWith("/")) {
        url = url.slice(0, url.length - 1);
    }
    return url;
}
