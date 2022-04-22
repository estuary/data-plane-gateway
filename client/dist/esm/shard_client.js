import * as consumer from "./gen/consumer/protocol/consumer.js";
import { Result } from "./result.js";
import { ShardSelector } from "./selector.js";
import { trimUrl } from "./util.js";
export class ShardClient {
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
        this.client = new consumer.Api({
            baseUrl: trimUrl(baseUrl),
        });
    }
    async list(include = new ShardSelector(), exclude = new ShardSelector()) {
        const response = await this.client.v1.shardList({
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet(),
            },
        });
        if (response.ok) {
            return Result.Ok(response.data.shards.map((j) => j.spec));
        }
        else {
            return Result.Err(response);
        }
    }
    async stat(shard, readThrough) {
        const response = await this.client.v1.shardStat({ shard, readThrough });
        if (response.ok) {
            return Result.Ok(response.data);
        }
        else {
            return Result.Err(response);
        }
    }
}
