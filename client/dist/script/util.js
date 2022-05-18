"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.sortBy = exports.ResponseError = exports.doFetch = exports.trimUrl = void 0;
const result_js_1 = require("./result.js");
function trimUrl(orig) {
    let url = orig.toString();
    if (url.endsWith("/")) {
        url = url.slice(0, url.length - 1);
    }
    return url;
}
exports.trimUrl = trimUrl;
async function doFetch(url, authToken, body) {
    try {
        const response = await fetch(url, {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${authToken}`,
            },
            body: JSON.stringify(body),
        });
        if (response.ok) {
            return result_js_1.Result.Ok(response);
        }
        else {
            return result_js_1.Result.Err(response);
        }
    }
    catch (response) {
        return result_js_1.Result.Err(response);
    }
}
exports.doFetch = doFetch;
/// Transforms Unary and Stream errors into a similar structure.
class ResponseError {
    constructor(body, response, status) {
        Object.defineProperty(this, "body", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        Object.defineProperty(this, "response", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        Object.defineProperty(this, "status", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        this.body = body;
        this.response = response;
        this.status = status;
    }
    static async fromResponse(response) {
        const body = await response.json();
        const status = body.code;
        return new ResponseError(body, response, status);
    }
    static async fromStreamResponse(response) {
        const body = await response.json();
        const status = body.error.httpStatus;
        return new ResponseError(body.error, response, status);
    }
}
exports.ResponseError = ResponseError;
function sortBy(...fields) {
    return (a, b) => {
        for (let i = 0; i < fields.length; i++) {
            const field = fields[i];
            if (a[field] > b[field]) {
                return 1;
            }
            else if (a[field] < b[field]) {
                return -1;
            }
            else {
                // a and b have the same field values. Try the next field.
            }
        }
        // a and b are equal across all fields
        return 0;
    };
}
exports.sortBy = sortBy;
