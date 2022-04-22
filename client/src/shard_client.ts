import * as consumer from "./gen/consumer/protocol/consumer.ts";
import { Result } from "./result.ts";
import { ShardSelector } from "./selector.ts";
import { trimUrl } from "./util.ts";

export class ShardClient {
  private baseUrl: URL;
  private client: consumer.Api<null>;

  constructor(baseUrl: URL) {
    this.baseUrl = baseUrl;
    this.client = new consumer.Api({
      baseUrl: trimUrl(baseUrl),
    });
  }

  async list(
    include: ShardSelector = new ShardSelector(),
    exclude: ShardSelector = new ShardSelector(),
  ): Promise<Result<Array<consumer.ConsumerShardSpec>, Response>> {
    const response = await this.client.v1.shardList({
      selector: {
        include: include.toLabelSet(),
        exclude: exclude.toLabelSet(),
      },
    });

    if (response.ok) {
      return Result.Ok(response.data.shards!.map((j) => j.spec!));
    } else {
      return Result.Err(response);
    }
  }

  async stat(
    shard: string,
    readThrough: Record<string, string>,
  ): Promise<Result<consumer.ConsumerStatResponse, Response>> {
    const response = await this.client.v1.shardStat({ shard, readThrough });

    if (response.ok) {
      return Result.Ok(response.data);
    } else {
      return Result.Err(response);
    }
  }
}
