"use strict";
/* eslint-disable */
/* tslint:disable */
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Api = exports.HttpClient = exports.ContentType = void 0;
var ContentType;
(function (ContentType) {
    ContentType["Json"] = "application/json";
    ContentType["FormData"] = "multipart/form-data";
    ContentType["UrlEncoded"] = "application/x-www-form-urlencoded";
})(ContentType = exports.ContentType || (exports.ContentType = {}));
class HttpClient {
    constructor(apiConfig = {}) {
        Object.defineProperty(this, "baseUrl", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: ""
        });
        Object.defineProperty(this, "securityData", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: null
        });
        Object.defineProperty(this, "securityWorker", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        Object.defineProperty(this, "abortControllers", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: new Map()
        });
        Object.defineProperty(this, "customFetch", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: (...fetchParams) => fetch(...fetchParams)
        });
        Object.defineProperty(this, "baseApiParams", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: {
                credentials: "same-origin",
                headers: {},
                redirect: "follow",
                referrerPolicy: "no-referrer",
            }
        });
        Object.defineProperty(this, "setSecurityData", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: (data) => {
                this.securityData = data;
            }
        });
        Object.defineProperty(this, "contentFormatters", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: {
                [ContentType.Json]: (input) => input !== null && (typeof input === "object" || typeof input === "string") ? JSON.stringify(input) : input,
                [ContentType.FormData]: (input) => Object.keys(input || {}).reduce((formData, key) => {
                    const property = input[key];
                    formData.append(key, property instanceof Blob
                        ? property
                        : typeof property === "object" && property !== null
                            ? JSON.stringify(property)
                            : `${property}`);
                    return formData;
                }, new FormData()),
                [ContentType.UrlEncoded]: (input) => this.toQueryString(input),
            }
        });
        Object.defineProperty(this, "createAbortSignal", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: (cancelToken) => {
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
            }
        });
        Object.defineProperty(this, "abortRequest", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: (cancelToken) => {
                const abortController = this.abortControllers.get(cancelToken);
                if (abortController) {
                    abortController.abort();
                    this.abortControllers.delete(cancelToken);
                }
            }
        });
        Object.defineProperty(this, "request", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: async ({ body, secure, path, type, query, format, baseUrl, cancelToken, ...params }) => {
                const secureParams = ((typeof secure === "boolean" ? secure : this.baseApiParams.secure) &&
                    this.securityWorker &&
                    (await this.securityWorker(this.securityData))) ||
                    {};
                const requestParams = this.mergeRequestParams(params, secureParams);
                const queryString = query && this.toQueryString(query);
                const payloadFormatter = this.contentFormatters[type || ContentType.Json];
                const responseFormat = format || requestParams.format;
                return this.customFetch(`${baseUrl || this.baseUrl || ""}${path}${queryString ? `?${queryString}` : ""}`, {
                    ...requestParams,
                    headers: {
                        ...(type && type !== ContentType.FormData ? { "Content-Type": type } : {}),
                        ...(requestParams.headers || {}),
                    },
                    signal: cancelToken ? this.createAbortSignal(cancelToken) : void 0,
                    body: typeof body === "undefined" || body === null ? null : payloadFormatter(body),
                }).then(async (response) => {
                    const r = response;
                    r.data = null;
                    r.error = null;
                    const data = !responseFormat
                        ? r
                        : await response[responseFormat]()
                            .then((data) => {
                            if (r.ok) {
                                r.data = data;
                            }
                            else {
                                r.error = data;
                            }
                            return r;
                        })
                            .catch((e) => {
                            r.error = e;
                            return r;
                        });
                    if (cancelToken) {
                        this.abortControllers.delete(cancelToken);
                    }
                    if (!response.ok)
                        throw data;
                    return data;
                });
            }
        });
        Object.assign(this, apiConfig);
    }
    encodeQueryParam(key, value) {
        const encodedKey = encodeURIComponent(key);
        return `${encodedKey}=${encodeURIComponent(typeof value === "number" ? value : `${value}`)}`;
    }
    addQueryParam(query, key) {
        return this.encodeQueryParam(key, query[key]);
    }
    addArrayQueryParam(query, key) {
        const value = query[key];
        return value.map((v) => this.encodeQueryParam(key, v)).join("&");
    }
    toQueryString(rawQuery) {
        const query = rawQuery || {};
        const keys = Object.keys(query).filter((key) => "undefined" !== typeof query[key]);
        return keys
            .map((key) => (Array.isArray(query[key]) ? this.addArrayQueryParam(query, key) : this.addQueryParam(query, key)))
            .join("&");
    }
    addQueryParams(rawQuery) {
        const queryString = this.toQueryString(rawQuery);
        return queryString ? `?${queryString}` : "";
    }
    mergeRequestParams(params1, params2) {
        return {
            ...this.baseApiParams,
            ...params1,
            ...(params2 || {}),
            headers: {
                ...(this.baseApiParams.headers || {}),
                ...(params1.headers || {}),
                ...((params2 && params2.headers) || {}),
            },
        };
    }
}
exports.HttpClient = HttpClient;
/**
 * @title broker/protocol/protocol.proto
 * @version version not set
 */
class Api extends HttpClient {
    constructor() {
        super(...arguments);
        Object.defineProperty(this, "v1", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: {
                /**
                 * No description
                 *
                 * @tags Journal
                 * @name JournalList
                 * @summary List Journals, their JournalSpecs and current Routes.
                 * @request POST:/v1/journals/list
                 * @response `200` `ProtocolListResponse` A successful response.
                 * @response `default` `RuntimeError` An unexpected error response.
                 */
                journalList: (body, params = {}) => this.request({
                    path: `/v1/journals/list`,
                    method: "POST",
                    body: body,
                    type: ContentType.Json,
                    format: "json",
                    ...params,
                }),
                /**
                 * No description
                 *
                 * @tags Journal
                 * @name JournalRead
                 * @summary Read from a specific Journal.
                 * @request POST:/v1/journals/read
                 * @response `200` `{ result?: ProtocolReadResponse, error?: RuntimeStreamError }` A successful response.(streaming responses)
                 * @response `default` `RuntimeError` An unexpected error response.
                 */
                journalRead: (body, params = {}) => this.request({
                    path: `/v1/journals/read`,
                    method: "POST",
                    body: body,
                    type: ContentType.Json,
                    format: "json",
                    ...params,
                }),
            }
        });
    }
}
exports.Api = Api;
