import * as consumer from "./gen/consumer/protocol/consumer.js";
import { Result } from "./result.js";
import { ShardSelector } from "./selector.js";
export declare class ShardClient {
    private baseUrl;
    private client;
    constructor(baseUrl: URL);
    list(include?: ShardSelector, exclude?: ShardSelector): Promise<Result<Array<consumer.ConsumerShardSpec>, Response>>;
    stat(shard: string, readThrough: Record<string, string>): Promise<Result<consumer.ConsumerStatResponse, Response>>;
}
