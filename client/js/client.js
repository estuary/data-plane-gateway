// deno-fmt-ignore-file
// deno-lint-ignore-file
// This code was bundled using `deno bundle` and it's not recommended to edit it manually

var ContentType;
(function(ContentType2) {
    ContentType2["Json"] = "application/json";
    ContentType2["FormData"] = "multipart/form-data";
    ContentType2["UrlEncoded"] = "application/x-www-form-urlencoded";
})(ContentType || (ContentType = {}));
class HttpClient {
    baseUrl = "";
    securityData = null;
    securityWorker;
    abortControllers = new Map();
    customFetch = (...fetchParams)=>fetch(...fetchParams)
    ;
    baseApiParams = {
        credentials: "same-origin",
        headers: {},
        redirect: "follow",
        referrerPolicy: "no-referrer"
    };
    constructor(apiConfig = {}){
        Object.assign(this, apiConfig);
    }
    setSecurityData = (data)=>{
        this.securityData = data;
    };
    encodeQueryParam(key, value) {
        const encodedKey = encodeURIComponent(key);
        return `${encodedKey}=${encodeURIComponent(typeof value === "number" ? value : `${value}`)}`;
    }
    addQueryParam(query, key) {
        return this.encodeQueryParam(key, query[key]);
    }
    addArrayQueryParam(query, key) {
        const value = query[key];
        return value.map((v)=>this.encodeQueryParam(key, v)
        ).join("&");
    }
    toQueryString(rawQuery) {
        const query = rawQuery || {};
        const keys = Object.keys(query).filter((key)=>"undefined" !== typeof query[key]
        );
        return keys.map((key)=>Array.isArray(query[key]) ? this.addArrayQueryParam(query, key) : this.addQueryParam(query, key)
        ).join("&");
    }
    addQueryParams(rawQuery) {
        const queryString = this.toQueryString(rawQuery);
        return queryString ? `?${queryString}` : "";
    }
    contentFormatters = {
        [ContentType.Json]: (input)=>input !== null && (typeof input === "object" || typeof input === "string") ? JSON.stringify(input) : input
        ,
        [ContentType.FormData]: (input)=>Object.keys(input || {}).reduce((formData, key)=>{
                const property = input[key];
                formData.append(key, property instanceof Blob ? property : typeof property === "object" && property !== null ? JSON.stringify(property) : `${property}`);
                return formData;
            }, new FormData())
        ,
        [ContentType.UrlEncoded]: (input)=>this.toQueryString(input)
    };
    mergeRequestParams(params1, params2) {
        return {
            ...this.baseApiParams,
            ...params1,
            ...params2 || {},
            headers: {
                ...this.baseApiParams.headers || {},
                ...params1.headers || {},
                ...params2 && params2.headers || {}
            }
        };
    }
    createAbortSignal = (cancelToken)=>{
        if (this.abortControllers.has(cancelToken)) {
            const abortController = this.abortControllers.get(cancelToken);
            if (abortController) {
                return abortController.signal;
            }
            return void 0;
        }
        const abortController = new AbortController();
        this.abortControllers.set(cancelToken, abortController);
        return abortController.signal;
    };
    abortRequest = (cancelToken)=>{
        const abortController = this.abortControllers.get(cancelToken);
        if (abortController) {
            abortController.abort();
            this.abortControllers.delete(cancelToken);
        }
    };
    request = async ({ body , secure , path , type , query , format , baseUrl , cancelToken , ...params })=>{
        const secureParams = (typeof secure === "boolean" ? secure : this.baseApiParams.secure) && this.securityWorker && await this.securityWorker(this.securityData) || {};
        const requestParams = this.mergeRequestParams(params, secureParams);
        const queryString = query && this.toQueryString(query);
        const payloadFormatter = this.contentFormatters[type || ContentType.Json];
        const responseFormat = format || requestParams.format;
        return this.customFetch(`${baseUrl || this.baseUrl || ""}${path}${queryString ? `?${queryString}` : ""}`, {
            ...requestParams,
            headers: {
                ...type && type !== ContentType.FormData ? {
                    "Content-Type": type
                } : {},
                ...requestParams.headers || {}
            },
            signal: cancelToken ? this.createAbortSignal(cancelToken) : void 0,
            body: typeof body === "undefined" || body === null ? null : payloadFormatter(body)
        }).then(async (response)=>{
            const r = response;
            r.data = null;
            r.error = null;
            const data1 = !responseFormat ? r : await response[responseFormat]().then((data)=>{
                if (r.ok) {
                    r.data = data;
                } else {
                    r.error = data;
                }
                return r;
            }).catch((e)=>{
                r.error = e;
                return r;
            });
            if (cancelToken) {
                this.abortControllers.delete(cancelToken);
            }
            if (!response.ok) throw data1;
            return data1;
        });
    };
}
class Api extends HttpClient {
    v1 = {
        journalList: (body, params = {})=>this.request({
                path: `/v1/journals/list`,
                method: "POST",
                body: body,
                type: ContentType.Json,
                format: "json",
                ...params
            })
        ,
        journalRead: (body, params = {})=>this.request({
                path: `/v1/journals/read`,
                method: "POST",
                body: body,
                type: ContentType.Json,
                format: "json",
                ...params
            })
    };
}
const mod = {
    ContentType: ContentType,
    HttpClient: HttpClient,
    Api: Api
};
var ContentType1;
(function(ContentType3) {
    ContentType3["Json"] = "application/json";
    ContentType3["FormData"] = "multipart/form-data";
    ContentType3["UrlEncoded"] = "application/x-www-form-urlencoded";
})(ContentType1 || (ContentType1 = {}));
class HttpClient1 {
    baseUrl = "";
    securityData = null;
    securityWorker;
    abortControllers = new Map();
    customFetch = (...fetchParams)=>fetch(...fetchParams)
    ;
    baseApiParams = {
        credentials: "same-origin",
        headers: {},
        redirect: "follow",
        referrerPolicy: "no-referrer"
    };
    constructor(apiConfig = {}){
        Object.assign(this, apiConfig);
    }
    setSecurityData = (data)=>{
        this.securityData = data;
    };
    encodeQueryParam(key, value) {
        const encodedKey = encodeURIComponent(key);
        return `${encodedKey}=${encodeURIComponent(typeof value === "number" ? value : `${value}`)}`;
    }
    addQueryParam(query, key) {
        return this.encodeQueryParam(key, query[key]);
    }
    addArrayQueryParam(query, key) {
        const value = query[key];
        return value.map((v)=>this.encodeQueryParam(key, v)
        ).join("&");
    }
    toQueryString(rawQuery) {
        const query = rawQuery || {};
        const keys = Object.keys(query).filter((key)=>"undefined" !== typeof query[key]
        );
        return keys.map((key)=>Array.isArray(query[key]) ? this.addArrayQueryParam(query, key) : this.addQueryParam(query, key)
        ).join("&");
    }
    addQueryParams(rawQuery) {
        const queryString = this.toQueryString(rawQuery);
        return queryString ? `?${queryString}` : "";
    }
    contentFormatters = {
        [ContentType1.Json]: (input)=>input !== null && (typeof input === "object" || typeof input === "string") ? JSON.stringify(input) : input
        ,
        [ContentType1.FormData]: (input)=>Object.keys(input || {}).reduce((formData, key)=>{
                const property = input[key];
                formData.append(key, property instanceof Blob ? property : typeof property === "object" && property !== null ? JSON.stringify(property) : `${property}`);
                return formData;
            }, new FormData())
        ,
        [ContentType1.UrlEncoded]: (input)=>this.toQueryString(input)
    };
    mergeRequestParams(params1, params2) {
        return {
            ...this.baseApiParams,
            ...params1,
            ...params2 || {},
            headers: {
                ...this.baseApiParams.headers || {},
                ...params1.headers || {},
                ...params2 && params2.headers || {}
            }
        };
    }
    createAbortSignal = (cancelToken)=>{
        if (this.abortControllers.has(cancelToken)) {
            const abortController = this.abortControllers.get(cancelToken);
            if (abortController) {
                return abortController.signal;
            }
            return void 0;
        }
        const abortController = new AbortController();
        this.abortControllers.set(cancelToken, abortController);
        return abortController.signal;
    };
    abortRequest = (cancelToken)=>{
        const abortController = this.abortControllers.get(cancelToken);
        if (abortController) {
            abortController.abort();
            this.abortControllers.delete(cancelToken);
        }
    };
    request = async ({ body , secure , path , type , query , format , baseUrl , cancelToken , ...params })=>{
        const secureParams = (typeof secure === "boolean" ? secure : this.baseApiParams.secure) && this.securityWorker && await this.securityWorker(this.securityData) || {};
        const requestParams = this.mergeRequestParams(params, secureParams);
        const queryString = query && this.toQueryString(query);
        const payloadFormatter = this.contentFormatters[type || ContentType1.Json];
        const responseFormat = format || requestParams.format;
        return this.customFetch(`${baseUrl || this.baseUrl || ""}${path}${queryString ? `?${queryString}` : ""}`, {
            ...requestParams,
            headers: {
                ...type && type !== ContentType1.FormData ? {
                    "Content-Type": type
                } : {},
                ...requestParams.headers || {}
            },
            signal: cancelToken ? this.createAbortSignal(cancelToken) : void 0,
            body: typeof body === "undefined" || body === null ? null : payloadFormatter(body)
        }).then(async (response)=>{
            const r = response;
            r.data = null;
            r.error = null;
            const data1 = !responseFormat ? r : await response[responseFormat]().then((data)=>{
                if (r.ok) {
                    r.data = data;
                } else {
                    r.error = data;
                }
                return r;
            }).catch((e)=>{
                r.error = e;
                return r;
            });
            if (cancelToken) {
                this.abortControllers.delete(cancelToken);
            }
            if (!response.ok) throw data1;
            return data1;
        });
    };
}
class Api1 extends HttpClient1 {
    v1 = {
        shardList: (body, params = {})=>this.request({
                path: `/v1/shards/list`,
                method: "POST",
                body: body,
                type: ContentType1.Json,
                format: "json",
                ...params
            })
        ,
        shardStat: (body, params = {})=>this.request({
                path: `/v1/shards/stat`,
                method: "POST",
                body: body,
                type: ContentType1.Json,
                format: "json",
                ...params
            })
    };
}
const mod1 = {
    ContentType: ContentType1,
    HttpClient: HttpClient1,
    Api: Api1
};
const mod2 = {
    broker: mod,
    consumer: mod1
};
function splitStream(splitOn) {
    let buffer = "";
    function queueLines(controller) {
        const parts = buffer.split(splitOn);
        parts.slice(0, -1).forEach((part)=>controller.enqueue(part)
        );
        buffer = parts[parts.length - 1];
    }
    return new TransformStream({
        transform (chunk, controller) {
            buffer += chunk;
            queueLines(controller);
        },
        flush (controller) {
            if (buffer.length) {
                queueLines(controller);
            }
        }
    });
}
function parseJSONStream() {
    return new TransformStream({
        transform (chunk, controller) {
            try {
                controller.enqueue(JSON.parse(chunk));
            } catch (_e) {}
        }
    });
}
function unwrapResult() {
    return new TransformStream({
        transform (chunk, controller) {
            controller.enqueue(chunk.result);
        }
    });
}
function decodeContent() {
    return new TransformStream({
        transform (value, controller) {
            if (value.content?.length) {
                controller.enqueue(atob(value.content));
            }
        }
    });
}
async function readStreamToEnd(stream) {
    const results = [];
    const reader = stream.getReader();
    await reader.read().then(async function pump({ done , value  }) {
        if (!done) {
            results.push(value);
            return pump(await reader.read());
        }
    });
    return results;
}
const mod3 = {
    splitStream: splitStream,
    parseJSONStream: parseJSONStream,
    unwrapResult: unwrapResult,
    decodeContent: decodeContent,
    readStreamToEnd: readStreamToEnd
};
class Selector {
    labels;
    constructor(){
        this.labels = [];
    }
    toLabelSet() {
        return {
            labels: this.labels
        };
    }
}
class JournalSelector extends Selector {
    collection(v) {
        this.labels.push({
            name: "estuary.dev/collection",
            value: v
        });
        return this;
    }
    name(v) {
        this.labels.push({
            name: "name",
            value: v
        });
        return this;
    }
    static prefix(v) {
        if (!v.endsWith("/")) {
            v = v + "/";
        }
        const sel = new JournalSelector();
        sel.labels.push({
            name: "prefix",
            value: v
        });
        return sel;
    }
}
class ShardSelector extends Selector {
    task(v) {
        this.labels.push({
            name: "estuary.dev/task-name",
            value: v
        });
        return this;
    }
    id(v) {
        this.labels.push({
            name: "id",
            value: v
        });
        return this;
    }
}
function trimUrl(orig) {
    let url = orig.toString();
    if (url.endsWith("/")) {
        url = url.slice(0, url.length - 1);
    }
    return url;
}
class Result {
    value;
    error;
    constructor(value, error){
        this.value = value;
        this.error = error;
    }
    static Ok(value) {
        const self = new Result(value, undefined);
        return Object.freeze(self);
    }
    static Err(err) {
        const self = new Result(undefined, err);
        return Object.freeze(self);
    }
    ok() {
        return !!this.value;
    }
    err() {
        return !!this.error;
    }
    unwrap() {
        if (this.value) {
            return this.value;
        } else {
            throw "Attempted to unwrap an Result error";
        }
    }
    unwrap_err() {
        if (this.error) {
            return this.error;
        } else {
            throw "Attempted to unwrap an Result error";
        }
    }
    map(f) {
        if (this.value) {
            return new Result(f(this.value), undefined);
        } else {
            return this;
        }
    }
    map_err(f) {
        if (this.error) {
            return new Result(undefined, f(this.error));
        } else {
            return this;
        }
    }
}
class JournalClient {
    baseUrl;
    client;
    constructor(baseUrl){
        this.baseUrl = baseUrl;
        this.client = new mod.Api({
            baseUrl: trimUrl(baseUrl)
        });
    }
    async list(include = new JournalSelector(), exclude = new JournalSelector()) {
        const response = await this.client.v1.journalList({
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet()
            }
        });
        if (response.ok) {
            return Result.Ok(response.data.journals.map((j)=>j.spec
            ));
        } else {
            return Result.Err(response);
        }
    }
    async read(req) {
        const url = `${this.baseUrl.toString()}v1/journals/read`;
        const response = await fetch(url, {
            method: "POST",
            headers: {
                "Content-Type": mod.ContentType.Json
            },
            body: JSON.stringify(req)
        });
        if (response.ok) {
            const reader = response.body.pipeThrough(new TextDecoderStream()).pipeThrough(splitStream("\n")).pipeThrough(parseJSONStream()).pipeThrough(unwrapResult());
            return Result.Ok(reader);
        } else {
            return Result.Err(response);
        }
    }
}
class ShardClient {
    baseUrl;
    client;
    constructor(baseUrl){
        this.baseUrl = baseUrl;
        this.client = new mod1.Api({
            baseUrl: trimUrl(baseUrl)
        });
    }
    async list(include = new ShardSelector(), exclude = new ShardSelector()) {
        const response = await this.client.v1.shardList({
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet()
            }
        });
        if (response.ok) {
            return Result.Ok(response.data.shards.map((j)=>j.spec
            ));
        } else {
            return Result.Err(response);
        }
    }
    async stat(shard, readThrough) {
        const response = await this.client.v1.shardStat({
            shard,
            readThrough
        });
        if (response.ok) {
            return Result.Ok(response.data);
        } else {
            return Result.Err(response);
        }
    }
}
export { JournalClient as JournalClient };
export { JournalSelector as JournalSelector, ShardSelector as ShardSelector };
export { Result as Result };
export { ShardClient as ShardClient };
export { mod2 as protocols };
export { mod3 as streams };
