import * as broker from "./gen/broker/protocol/broker.js";
import { JournalSelector } from "./selector.js";
import { trimUrl } from "./util.js";
import { Result } from "./result.js";
import { decodeContent, parseJSONStream, splitStream, unwrapResult, } from "./streams.js";
export class JournalClient {
    constructor(baseUrl) {
        Object.defineProperty(this, "baseUrl", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        Object.defineProperty(this, "client", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        this.baseUrl = baseUrl;
        this.client = new broker.Api({
            baseUrl: trimUrl(baseUrl),
        });
    }
    async list(include = new JournalSelector(), exclude = new JournalSelector()) {
        const response = await this.client.v1.journalList({
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet(),
            },
        });
        if (response.ok) {
            return Result.Ok(response.data.journals.map((j) => j.spec));
        }
        else {
            return Result.Err(response);
        }
    }
    async read(req) {
        const url = `${this.baseUrl.toString()}v1/journals/read`;
        const response = await fetch(url, {
            method: "POST",
            headers: { "Content-Type": broker.ContentType.Json },
            body: JSON.stringify(req),
        });
        if (response.ok) {
            const reader = response.body
                .pipeThrough(new TextDecoderStream())
                .pipeThrough(splitStream("\n"))
                .pipeThrough(parseJSONStream())
                .pipeThrough(unwrapResult());
            return Result.Ok(reader);
        }
        else {
            return Result.Err(response);
        }
    }
}
export function parseJournalDocuments(stream) {
    return stream
        .pipeThrough(decodeContent())
        .pipeThrough(splitStream("\n"))
        .pipeThrough(parseJSONStream());
}
