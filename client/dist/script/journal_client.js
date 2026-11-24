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
            // The List RPC is server-streaming: a large listing arrives as multiple
            // newline-delimited `{ result: ListResponse }` frames, because the broker
            // caps the number of journals per frame. Drain the stream and merge every
            // frame's journals into one response. Once the first frame is sent, the
            // HTTP status is already 200, so a later failure arrives as an
            // `{ error: ... }` frame and must fail the whole listing.
            const response = result.unwrap();
            const stream = response.body
                .pipeThrough(new TextDecoderStream())
                .pipeThrough((0, streams_js_1.splitStream)("\n"))
                .pipeThrough((0, streams_js_1.parseJSONStream)());
            const frames = await (0, streams_js_1.readStreamToEnd)(stream);
            const failed = frames.find((frame) => frame.error);
            if (failed) {
                return result_js_1.Result.Err(util_js_1.ResponseError.fromStreamError(failed.error, response));
            }
            const merged = {
                status: frames[0]?.result?.status,
                header: frames[0]?.result?.header,
                journals: frames.flatMap((frame) => frame.result?.journals ?? []),
            };
            return result_js_1.Result.Ok(merged);
        }
        else {
            return result_js_1.Result.Err(await util_js_1.ResponseError.fromStreamResponse(result.unwrap_err()));
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
