"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ShardClient = void 0;
const result_js_1 = require("./result.js");
const selector_js_1 = require("./selector.js");
const util_js_1 = require("./util.js");
class ShardClient {
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
    async list(include = new selector_js_1.ShardSelector(), exclude = new selector_js_1.ShardSelector()) {
        const url = `${this.baseUrl.toString()}v1/shards/list`;
        const body = {
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet(),
            },
        };
        const result = await (0, util_js_1.doFetch)(url, this.authToken, body);
        if (result.ok()) {
            const data = await result.unwrap().json();
            return result_js_1.Result.Ok(data.shards.map((j) => j.spec));
        }
        else {
            return result_js_1.Result.Err(await util_js_1.ResponseError.fromResponse(result.unwrap_err()));
        }
    }
    async stat(shard, readThrough) {
        const url = `${this.baseUrl.toString()}v1/shards/stat`;
        const body = { shard, readThrough };
        const result = await (0, util_js_1.doFetch)(url, this.authToken, body);
        if (result.ok()) {
            const data = await result.unwrap().json();
            return result_js_1.Result.Ok(data);
        }
        else {
            return result_js_1.Result.Err(await util_js_1.ResponseError.fromResponse(result.unwrap_err()));
        }
    }
}
exports.ShardClient = ShardClient;
