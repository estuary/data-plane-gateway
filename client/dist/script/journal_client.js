"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.parseJournalDocuments = exports.JournalClient = void 0;
const selector_js_1 = require("./selector.js");
const util_js_1 = require("./util.js");
const result_js_1 = require("./result.js");
const streams_js_1 = require("./streams.js");
class JournalClient {
    constructor(baseUrl, authToken) {
        Object.defineProperty(this, "authToken", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        Object.defineProperty(this, "baseUrl", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        this.authToken = authToken;
        this.baseUrl = baseUrl;
    }
    async list(include = new selector_js_1.JournalSelector(), exclude = new selector_js_1.JournalSelector()) {
        const url = `${this.baseUrl.toString()}v1/journals/list`;
        const body = {
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet(),
            },
        };
        const result = await (0, util_js_1.doFetch)(url, this.authToken, body);
        if (result.ok()) {
            const data = await result.unwrap().json();
            return result_js_1.Result.Ok(data.journals.map((j) => j.spec));
        }
        else {
            return result_js_1.Result.Err(await util_js_1.ResponseError.fromResponse(result.unwrap_err()));
        }
    }
    async read(req) {
        const url = `${this.baseUrl.toString()}v1/journals/read`;
        const result = await (0, util_js_1.doFetch)(url, this.authToken, req);
        if (result.ok()) {
            const reader = result.unwrap().body
                .pipeThrough(new TextDecoderStream())
                .pipeThrough((0, streams_js_1.splitStream)("\n"))
                .pipeThrough((0, streams_js_1.parseJSONStream)())
                .pipeThrough((0, streams_js_1.unwrapResult)());
            return result_js_1.Result.Ok(reader);
        }
        else {
            return result_js_1.Result.Err(await util_js_1.ResponseError.fromStreamResponse(result.unwrap_err()));
        }
    }
}
exports.JournalClient = JournalClient;
function parseJournalDocuments(stream) {
    return stream
        .pipeThrough((0, streams_js_1.decodeContent)())
        .pipeThrough((0, streams_js_1.splitStream)("\n"))
        .pipeThrough((0, streams_js_1.parseJSONStream)());
}
exports.parseJournalDocuments = parseJournalDocuments;
