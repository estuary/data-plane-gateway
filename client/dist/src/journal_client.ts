import * as broker from "./gen/broker/protocol/broker.js";
import { JournalSelector } from "./selector.js";
import { JsonObject, trimUrl } from "./util.js";
import { Result } from "./result.js";
import {
  decodeContent,
  parseJSONStream,
  splitStream,
  unwrapResult,
} from "./streams.js";

export class JournalClient {
  private baseUrl: URL;
  private client: broker.Api<null>;

  constructor(baseUrl: URL) {
    this.baseUrl = baseUrl;
    this.client = new broker.Api({
      baseUrl: trimUrl(baseUrl),
    });
  }

  async list(
    include: JournalSelector = new JournalSelector(),
    exclude: JournalSelector = new JournalSelector(),
  ): Promise<Result<Array<broker.ProtocolJournalSpec>, Response>> {
    const response = await this.client.v1.journalList({
      selector: {
        include: include.toLabelSet(),
        exclude: exclude.toLabelSet(),
      },
    });

    if (response.ok) {
      return Result.Ok(response.data.journals!.map((j) => j.spec!));
    } else {
      return Result.Err(response);
    }
  }

  async read(
    req: broker.ProtocolReadRequest,
  ): Promise<Result<ReadableStream<broker.ProtocolReadResponse>, Response>> {
    const url = `${this.baseUrl.toString()}v1/journals/read`;
    const response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": broker.ContentType.Json },
      body: JSON.stringify(req),
    });

    if (response.ok) {
      const reader = response.body!
        .pipeThrough(new TextDecoderStream())
        .pipeThrough(splitStream("\n"))
        .pipeThrough(parseJSONStream())
        .pipeThrough(unwrapResult());

      return Result.Ok(reader);
    } else {
      return Result.Err(response);
    }
  }
}

export function parseJournalDocuments(
  stream: ReadableStream<broker.ProtocolReadResponse>,
): ReadableStream<JsonObject> {
  return stream!
    .pipeThrough(decodeContent())
    .pipeThrough(splitStream("\n"))
    .pipeThrough(parseJSONStream());
}
