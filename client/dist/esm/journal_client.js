import { JournalSelector } from "./selector.js";
import { doFetch, ResponseError } from "./util.js";
import { Result } from "./result.js";
import { decodeContent, parseJSONStream, readStreamToEnd, splitStream, unwrapResult, } from "./streams.js";
export class JournalClient {
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
    async list(include = new JournalSelector(), exclude = new JournalSelector()) {
        const url = `${this.baseUrl.toString()}v1/journals/list`;
        const body = {
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet(),
            },
        };
        const result = await doFetch(url, this.authToken, body);
        if (result.ok()) {
            // The List RPC is server-streaming: a large listing arrives as multiple
            // newline-delimited `{ result: ListResponse }` frames, because the broker
            // caps the number of journals per frame. Drain the stream and merge every
            // frame's journals into one response, mirroring how `read()` consumes a
            // stream. `unwrapResult()` strips the `{ result: ... }` envelope, so each
            // chunk is a bare ListResponse.
            const stream = result.unwrap().body
                .pipeThrough(new TextDecoderStream())
                .pipeThrough(splitStream("\n"))
                .pipeThrough(parseJSONStream())
                .pipeThrough(unwrapResult());
            const frames = await readStreamToEnd(stream);
            const merged = {
                status: frames[0]?.status,
                header: frames[0]?.header,
                journals: frames.flatMap((frame) => frame?.journals ?? []),
            };
            return Result.Ok(merged);
        }
        else {
            return Result.Err(await ResponseError.fromStreamResponse(result.unwrap_err()));
        }
    }
    async read(req) {
        const url = `${this.baseUrl.toString()}v1/journals/read`;
        const result = await doFetch(url, this.authToken, req);
        if (result.ok()) {
            const reader = result.unwrap().body
                .pipeThrough(new TextDecoderStream())
                .pipeThrough(splitStream("\n"))
                .pipeThrough(parseJSONStream())
                .pipeThrough(unwrapResult());
            return Result.Ok(reader);
        }
        else {
            return Result.Err(await ResponseError.fromStreamResponse(result.unwrap_err()));
        }
    }
}
export function parseJournalDocuments(stream) {
    return stream
        .pipeThrough(decodeContent())
        .pipeThrough(splitStream("\n"))
        .pipeThrough(parseJSONStream());
}
