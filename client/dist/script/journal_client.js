"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.parseJournalDocuments = exports.JournalClient = void 0;
const broker = __importStar(require("./gen/broker/protocol/broker.js"));
const selector_js_1 = require("./selector.js");
const util_js_1 = require("./util.js");
const result_js_1 = require("./result.js");
const streams_js_1 = require("./streams.js");
class JournalClient {
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
            baseUrl: (0, util_js_1.trimUrl)(baseUrl),
        });
    }
    async list(include = new selector_js_1.JournalSelector(), exclude = new selector_js_1.JournalSelector()) {
        const response = await this.client.v1.journalList({
            selector: {
                include: include.toLabelSet(),
                exclude: exclude.toLabelSet(),
            },
        });
        if (response.ok) {
            return result_js_1.Result.Ok(response.data.journals.map((j) => j.spec));
        }
        else {
            return result_js_1.Result.Err(response);
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
                .pipeThrough((0, streams_js_1.splitStream)("\n"))
                .pipeThrough((0, streams_js_1.parseJSONStream)())
                .pipeThrough((0, streams_js_1.unwrapResult)());
            return result_js_1.Result.Ok(reader);
        }
        else {
            return result_js_1.Result.Err(response);
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
