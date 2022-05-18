"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ShardSelector = exports.JournalSelector = exports.Selector = void 0;
class Selector {
    constructor() {
        Object.defineProperty(this, "labels", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        this.labels = [];
    }
    toLabelSet() {
        return { labels: this.labels };
    }
}
exports.Selector = Selector;
class JournalSelector extends Selector {
    collection(v) {
        this.labels.push({ name: "estuary.dev/collection", value: v });
        return this;
    }
    name(v) {
        this.labels.push({ name: "name", value: v });
        return this;
    }
    static prefix(v) {
        if (!v.endsWith("/")) {
            v = v + "/";
        }
        const sel = new JournalSelector();
        sel.labels.push({ name: "prefix", value: v });
        return sel;
    }
}
exports.JournalSelector = JournalSelector;
class ShardSelector extends Selector {
    task(v) {
        this.labels.push({ name: "estuary.dev/task-name", value: v });
        return this;
    }
    id(v) {
        this.labels.push({ name: "id", value: v });
        return this;
    }
}
exports.ShardSelector = ShardSelector;