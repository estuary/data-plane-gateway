"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.trimUrl = void 0;
function trimUrl(orig) {
    let url = orig.toString();
    if (url.endsWith("/")) {
        url = url.slice(0, url.length - 1);
    }
    return url;
}
exports.trimUrl = trimUrl;
