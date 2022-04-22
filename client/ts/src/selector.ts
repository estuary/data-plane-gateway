import { broker } from "./protocols.ts";

export class Selector {
  protected labels: Array<broker.ProtocolLabel>;

  constructor() {
    this.labels = [];
  }

  toLabelSet(): broker.ProtocolLabelSet {
    return { labels: this.labels };
  }
}

export class JournalSelector extends Selector {
  collection(v: string): this {
    this.labels.push({ name: "estuary.dev/collection", value: v });
    return this;
  }

  name(v: string): this {
    this.labels.push({ name: "name", value: v });
    return this;
  }

  static prefix(v: string): JournalSelector {
    if (!v.endsWith("/")) {
      v = v + "/";
    }
    const sel = new JournalSelector();
    sel.labels.push({ name: "prefix", value: v });
    return sel;
  }
}

export class ShardSelector extends Selector {
  task(v: string): this {
    this.labels.push({ name: "estuary.dev/task-name", value: v });
    return this;
  }

  id(v: string): this {
    this.labels.push({ name: "id", value: v });
    return this;
  }
}
