import * as broker from "./gen/broker/protocol/broker.js";
import { JournalSelector } from "./selector.js";
import { JsonObject } from "./util.js";
import { Result } from "./result.js";
export declare class JournalClient {
    private baseUrl;
    private client;
    constructor(baseUrl: URL);
    list(include?: JournalSelector, exclude?: JournalSelector): Promise<Result<Array<broker.ProtocolJournalSpec>, Response>>;
    read(req: broker.ProtocolReadRequest): Promise<Result<ReadableStream<broker.ProtocolReadResponse>, Response>>;
}
export declare function parseJournalDocuments(stream: ReadableStream<broker.ProtocolReadResponse>): ReadableStream<JsonObject>;
