import { assertEquals, assertMatch } from "https://deno.land/std@0.135.0/testing/asserts.ts";
// TODO: Built-in snapshot testing has landed in deno std, but is not yet released.
import { test as snapshotTest } from "https://deno.land/x/snap/mod.ts";

import * as consumer from "../src/gen/consumer/protocol/consumer.ts";
import { BASE_URL, makeJwt } from "./test_support.ts";
import { ShardClient } from "../src/shard_client.ts";
import { ShardSelector } from "../src/selector.ts";


Deno.test("ShardClient.list task selector test", async () => {
  const client = new ShardClient(BASE_URL, await makeJwt({}));
  const taskSelector = new ShardSelector().task("acmeCo/source-hello-world");

  const shards = (await client.list(taskSelector)).unwrap();

  assertEquals(1, shards.length);
  assertEquals(
    "capture/acmeCo/source-hello-world/00ffffffffffffff/00000000-00000000",
    shards[0].spec.id,
  );
  assertEquals("PRIMARY", shards[0].status[0].code);
});

Deno.test("ShardClient.list bare id selector test", async () => {
  const client = new ShardClient(BASE_URL, await makeJwt({}));
  const idSelector = new ShardSelector().id(
    "capture/acmeCo/source-hello-world/00ffffffffffffff/00000000-00000000",
  );

  const error = (await client.list(idSelector)).unwrap_err();

  assertMatch(error.body.message!, new RegExp("No authorizing labels provided"));
});

Deno.test("ShardClient.list compound id selector test", async () => {
  const client = new ShardClient(BASE_URL, await makeJwt({}));
  const idSelector = new ShardSelector()
    .task("acmeCo/yet-another-task")
    .task("acmeCo/source-hello-world")
    .task("acmeCo/verifies-label-sorting")
    .id("capture/acmeCo/source-hello-world/00ffffffffffffff/00000000-00000000");

  const shards = (await client.list(idSelector)).unwrap();

  assertEquals(1, shards.length);
  assertEquals(
    "capture/acmeCo/source-hello-world/00ffffffffffffff/00000000-00000000",
    shards[0].spec.id,
  );
});

snapshotTest("ShardClient.stat test", async ({ assertSnapshot }) => {
  const client = new ShardClient(BASE_URL, await makeJwt({prefixes: ["capture/acmeCo/"]}));

  const stats = (await client.stat(
    "capture/acmeCo/source-hello-world/00ffffffffffffff/00000000-00000000",
    {},
  )).unwrap();

  const pluck = (res: consumer.ConsumerStatResponse) => {
    return {
      status: res.status,
      // The connector may or may not have exited prior to this running, so the eof journal may not
      // be present. We don't care about the state of the connector, since it really has no bearing
      // on the test of the Shards api, so we add this line so the snapshot output is consistent.
      readThrough: {
        ...res.readThrough,
        "acmeCo/source-hello-world/eof": "who cares"
      },
      publishAt: res.publishAt,
    };
  };

  const masks = [
    "/readThrough/acmeCo\/source-hello-world\/txn",
    "/publishAt/acmeCo\/greetings\/00ffffffffffffff\/pivot=00",
    "/publishAt/ops.us-central1.v1\/stats\/00ffffffffffffff\/kind=capture\/name=acmeCo%2Fsource-hello-world\/pivot=00",
  ];
  assertSnapshot(pluck(stats), masks);
});
